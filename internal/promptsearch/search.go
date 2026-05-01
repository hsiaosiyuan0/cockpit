package promptsearch

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"cockpit/internal/app"

	"github.com/yanyiwu/gojieba"
	_ "modernc.org/sqlite"
)

const Retention = 15 * 24 * time.Hour

type Index struct {
	db   *sql.DB
	path string
}

type promptRecord struct {
	Result app.PromptSearchResult
	Hidden bool
}

var (
	codeTokenRe = regexp.MustCompile(`[a-z0-9][a-z0-9_./:@-]*`)
	jiebaOnce   sync.Once
	jiebaMu     sync.Mutex
	jiebaSeg    *gojieba.Jieba
)

func DBPath(stateDir string) string {
	return filepath.Join(stateDir, "prompts.sqlite")
}

func Open(path string) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	index := &Index{db: db, path: path}
	if err := index.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	db.SetMaxOpenConns(2)
	return index, nil
}

func (i *Index) Close() error {
	if i == nil || i.db == nil {
		return nil
	}
	return i.db.Close()
}

func (i *Index) Sync(snapshot app.Snapshot, retention time.Duration) (int, error) {
	if retention <= 0 {
		retention = Retention
	}
	cutoff := time.Now().Add(-retention).Unix()
	records := recordsFromSnapshot(snapshot, cutoff)

	tx, err := i.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`delete from prompts where at_unix < ?`, cutoff); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`delete from prompts where prompt like '# AGENTS.md instructions for %' or prompt like 'Base directory for this skill:%' or prompt like '[Request interrupted%' or prompt like '[Image #%'`); err != nil {
		return 0, err
	}

	promptStmt, err := tx.Prepare(`
insert into prompts (
  id, agent, session_id, task_id, mission_id, mission_name, cwd, prompt, search_text, at, at_unix, summary, hidden, updated_at
) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
on conflict(id) do update set
  agent=excluded.agent,
  session_id=excluded.session_id,
  task_id=excluded.task_id,
  mission_id=excluded.mission_id,
  mission_name=excluded.mission_name,
  cwd=excluded.cwd,
  prompt=excluded.prompt,
  search_text=excluded.search_text,
  at=excluded.at,
  at_unix=excluded.at_unix,
  summary=excluded.summary,
  hidden=excluded.hidden,
  updated_at=excluded.updated_at`)
	if err != nil {
		return 0, err
	}
	defer promptStmt.Close()

	updatedAt := time.Now().Format(time.RFC3339Nano)
	for _, record := range records {
		result := record.Result
		searchText := searchablePromptText(result)
		atUnix := result.At.Unix()
		atText := ""
		if !result.At.IsZero() {
			atText = result.At.Format(time.RFC3339Nano)
		}
		hidden := 0
		if record.Hidden {
			hidden = 1
		}
		if _, err := promptStmt.Exec(
			result.ID,
			string(result.Agent),
			result.SessionID,
			result.TaskID,
			result.MissionID,
			result.MissionName,
			result.CWD,
			result.Prompt,
			searchText,
			atText,
			atUnix,
			result.Summary,
			hidden,
			updatedAt,
		); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(records), nil
}

func (i *Index) Search(query string, limit int) ([]app.PromptSearchResult, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	return i.searchKeyword(query, limit)
}

func (i *Index) init() error {
	if err := i.dropEmbeddingArtifacts(); err != nil {
		return err
	}
	statements := []string{
		`pragma busy_timeout = 5000`,
		`pragma journal_mode = wal`,
		`pragma foreign_keys = on`,
		`create table if not exists prompts (
			id text primary key,
			agent text not null,
			session_id text not null,
			task_id text not null,
			mission_id text,
			mission_name text,
			cwd text,
			prompt text not null,
			search_text text not null default '',
			at text,
			at_unix integer not null,
			summary text,
			hidden integer not null default 0,
			updated_at text not null
		)`,
		`create index if not exists prompts_at_idx on prompts(at_unix desc)`,
		`create index if not exists prompts_hidden_idx on prompts(hidden, at_unix desc)`,
		`create index if not exists prompts_task_idx on prompts(task_id)`,
	}
	for _, statement := range statements {
		if _, err := i.db.Exec(statement); err != nil {
			return err
		}
	}
	if err := i.ensureSearchTextColumn(); err != nil {
		return err
	}
	if err := i.ensurePromptFTSSchema(); err != nil {
		return err
	}
	ftsStatements := []string{
		`create virtual table if not exists prompt_fts using fts5(
			search_text,
			prompt,
			summary,
			mission_name,
			cwd,
			content='prompts',
			content_rowid='rowid',
			tokenize='unicode61'
		)`,
		`create trigger if not exists prompts_ai after insert on prompts begin
			insert into prompt_fts(rowid, search_text, prompt, summary, mission_name, cwd)
			values (new.rowid, new.search_text, new.prompt, new.summary, new.mission_name, new.cwd);
		end`,
		`create trigger if not exists prompts_ad after delete on prompts begin
			insert into prompt_fts(prompt_fts, rowid, search_text, prompt, summary, mission_name, cwd)
			values ('delete', old.rowid, old.search_text, old.prompt, old.summary, old.mission_name, old.cwd);
		end`,
		`create trigger if not exists prompts_au after update on prompts begin
			insert into prompt_fts(prompt_fts, rowid, search_text, prompt, summary, mission_name, cwd)
			values ('delete', old.rowid, old.search_text, old.prompt, old.summary, old.mission_name, old.cwd);
			insert into prompt_fts(rowid, search_text, prompt, summary, mission_name, cwd)
			values (new.rowid, new.search_text, new.prompt, new.summary, new.mission_name, new.cwd);
		end`,
	}
	for _, statement := range ftsStatements {
		if _, err := i.db.Exec(statement); err != nil {
			return err
		}
	}
	_, err := i.db.Exec(`insert into prompt_fts(prompt_fts) values ('rebuild')`)
	return err
}

func (i *Index) dropEmbeddingArtifacts() error {
	for _, statement := range []string{
		`drop table if exists _vec_prompt_vectors`,
		`drop table if exists vector_storage`,
	} {
		if _, err := i.db.Exec(statement); err != nil {
			return err
		}
	}
	if _, err := i.db.Exec(`drop table if exists prompt_vectors`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no such module") {
			return err
		}
		// Older prototype databases may contain a vector virtual table, but current
		// builds no longer register that module. Remove the orphan schema entry.
		if _, pragmaErr := i.db.Exec(`pragma writable_schema = on`); pragmaErr != nil {
			return pragmaErr
		}
		if _, deleteErr := i.db.Exec(`delete from sqlite_master where name = 'prompt_vectors'`); deleteErr != nil {
			_, _ = i.db.Exec(`pragma writable_schema = off`)
			return deleteErr
		}
		if _, pragmaErr := i.db.Exec(`pragma writable_schema = off`); pragmaErr != nil {
			return pragmaErr
		}
	}
	return nil
}

func (i *Index) ensureSearchTextColumn() error {
	var count int
	err := i.db.QueryRow(`select count(*) from pragma_table_info('prompts') where name = 'search_text'`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = i.db.Exec(`alter table prompts add column search_text text not null default ''`)
	return err
}

func (i *Index) ensurePromptFTSSchema() error {
	var createSQL sql.NullString
	err := i.db.QueryRow(`select sql from sqlite_master where name = 'prompt_fts'`).Scan(&createSQL)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.Contains(strings.ToLower(createSQL.String), "search_text") {
		return nil
	}
	for _, statement := range []string{
		`drop trigger if exists prompts_ai`,
		`drop trigger if exists prompts_ad`,
		`drop trigger if exists prompts_au`,
		`drop table if exists prompt_fts`,
	} {
		if _, err := i.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (i *Index) searchKeyword(query string, limit int) ([]app.PromptSearchResult, error) {
	if query == "" {
		return i.recent(limit)
	}
	seen := map[string]bool{}
	results := make([]app.PromptSearchResult, 0)
	addFTSResults := func(match string, boost float64) error {
		if match == "" || len(results) >= limit {
			return nil
		}
		rows, err := i.db.Query(`
select p.id, p.agent, p.session_id, p.task_id, p.mission_id, p.mission_name, p.cwd, p.prompt, p.at, p.at_unix, p.summary, bm25(prompt_fts) rank
from prompt_fts
join prompts p on p.rowid = prompt_fts.rowid
where prompt_fts match ? and p.hidden = 0
order by rank
limit ?`, match, limit*2)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			result, rank, err := scanPromptResult(rows, "keyword")
			if err != nil {
				return err
			}
			if seen[result.ID] {
				continue
			}
			result.Score = boost + 12/(1+math.Abs(rank)) + recencyBoost(result.At)
			seen[result.ID] = true
			results = append(results, result)
			if len(results) >= limit {
				break
			}
		}
		return rows.Err()
	}
	if err := addFTSResults(ftsQuery(query, "AND"), 6); err != nil {
		return nil, err
	}
	if err := addFTSResults(ftsQuery(query, "OR"), 0); err != nil {
		return nil, err
	}

	if len(results) < limit {
		rows, err := i.db.Query(`
select id, agent, session_id, task_id, mission_id, mission_name, cwd, prompt, at, at_unix, summary
from prompts
where hidden = 0
order by at_unix desc
limit ?`, 2000)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			result, err := scanPromptResultNoRank(rows, "keyword")
			if err != nil {
				return nil, err
			}
			if seen[result.ID] {
				continue
			}
			score := keywordScore(query, searchablePromptText(result))
			if score <= 0.15 {
				continue
			}
			result.Score = score + recencyBoost(result.At)
			results = append(results, result)
			seen[result.ID] = true
			if len(results) >= limit {
				break
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	sortPromptResults(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (i *Index) recent(limit int) ([]app.PromptSearchResult, error) {
	rows, err := i.db.Query(`
select id, agent, session_id, task_id, mission_id, mission_name, cwd, prompt, at, at_unix, summary
from prompts
where hidden = 0
order by at_unix desc
limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]app.PromptSearchResult, 0)
	for rows.Next() {
		result, err := scanPromptResultNoRank(rows, "keyword")
		if err != nil {
			return nil, err
		}
		result.Score = 1 + recencyBoost(result.At)
		results = append(results, result)
	}
	return results, rows.Err()
}

func recordsFromSnapshot(snapshot app.Snapshot, cutoffUnix int64) []promptRecord {
	taskMissions := missionIndex(snapshot.Missions)
	var records []promptRecord
	for _, task := range snapshot.Tasks {
		if task.Session.Internal {
			continue
		}
		mission := taskMissions[task.ID]
		hidden := task.Archived || task.Ignored
		hasUserEvent := false
		for _, event := range task.Session.Events {
			event.Text = strings.TrimSpace(event.Text)
			if event.Type != app.EventUser || event.Text == "" || isPromptSearchNoise(event.Text) {
				continue
			}
			if !event.At.IsZero() && event.At.Unix() < cutoffUnix {
				continue
			}
			hasUserEvent = true
			records = append(records, promptRecord{Result: resultFromEvent(task, mission, event, "keyword"), Hidden: hidden})
		}
		task.Session.LastPrompt = strings.TrimSpace(task.Session.LastPrompt)
		if hasUserEvent || task.Session.LastPrompt == "" || isPromptSearchNoise(task.Session.LastPrompt) {
			continue
		}
		if !task.Session.LastEventAt.IsZero() && task.Session.LastEventAt.Unix() < cutoffUnix {
			continue
		}
		records = append(records, promptRecord{
			Result: resultFromEvent(task, mission, app.Event{
				At:   task.Session.LastEventAt,
				Type: app.EventUser,
				Text: task.Session.LastPrompt,
			}, "keyword"),
			Hidden: hidden,
		})
	}
	return records
}

func isPromptSearchNoise(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "# AGENTS.md instructions for ") ||
		strings.HasPrefix(text, "Base directory for this skill:") ||
		strings.HasPrefix(text, "[Request interrupted") ||
		strings.HasPrefix(text, "[Image #")
}

func resultFromEvent(task app.Task, mission app.Mission, event app.Event, mode string) app.PromptSearchResult {
	at := event.At
	if at.IsZero() {
		at = task.Session.LastEventAt
	}
	result := app.PromptSearchResult{
		ID:          promptID(task.ID, at, event.Text),
		Agent:       task.Session.Agent,
		SessionID:   task.Session.ID,
		TaskID:      task.ID,
		MissionID:   mission.ID,
		MissionName: mission.Name,
		CWD:         task.Session.CWD,
		Prompt:      strings.TrimSpace(event.Text),
		At:          at,
		Mode:        mode,
		Summary:     compactLine(task.Summary, 140),
	}
	if result.MissionName == "" {
		result.MissionName = repoName(task.Session.CWD)
	}
	return result
}

func scanPromptResult(rows scanner, mode string) (app.PromptSearchResult, float64, error) {
	var rank float64
	result, err := scanPromptResultNoRank(rows, mode, &rank)
	return result, rank, err
}

func scanPromptResultNoRank(rows scanner, mode string, extra ...*float64) (app.PromptSearchResult, error) {
	var result app.PromptSearchResult
	var agent string
	var atText string
	var atUnix int64
	var missionID, missionName, cwd, summary sql.NullString
	dest := []any{
		&result.ID,
		&agent,
		&result.SessionID,
		&result.TaskID,
		&missionID,
		&missionName,
		&cwd,
		&result.Prompt,
		&atText,
		&atUnix,
		&summary,
	}
	if len(extra) > 0 {
		dest = append(dest, extra[0])
	}
	if err := rows.Scan(dest...); err != nil {
		return result, err
	}
	result.Agent = app.Agent(agent)
	result.MissionID = missionID.String
	result.MissionName = missionName.String
	result.CWD = cwd.String
	result.Summary = summary.String
	result.Mode = mode
	result.At = parseDBTime(atText, atUnix)
	return result, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func parseDBTime(raw string, unix int64) time.Time {
	if raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err == nil {
			return parsed
		}
	}
	if unix > 0 {
		return time.Unix(unix, 0)
	}
	return time.Time{}
}

func missionIndex(missions []app.Mission) map[string]app.Mission {
	index := map[string]app.Mission{}
	for _, mission := range missions {
		for _, taskID := range mission.TaskIDs {
			index[taskID] = mission
		}
	}
	return index
}

func sortPromptResults(results []app.PromptSearchResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].At.After(results[j].At)
	})
}

func ftsQuery(query string, op string) string {
	terms := splitTerms(query)
	if len(terms) == 0 {
		return ""
	}
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		if len([]rune(term)) > 48 {
			continue
		}
		parts = append(parts, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	if strings.EqualFold(op, "AND") {
		return strings.Join(parts, " AND ")
	}
	return strings.Join(parts, " OR ")
}

func promptID(taskID string, at time.Time, text string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(taskID))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(at.Format(time.RFC3339Nano)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(text))
	return fmt.Sprintf("%s:%x", taskID, hash.Sum64())
}

func keywordScore(query string, text string) float64 {
	q := strings.ToLower(strings.TrimSpace(query))
	t := strings.ToLower(text)
	if q == "" || t == "" {
		return 0
	}
	score := 0.0
	if strings.Contains(t, q) {
		score += 6
	}
	queryTerms := termSet(q)
	textTerms := termSet(t)
	if len(queryTerms) > 0 {
		matched := 0
		for term := range queryTerms {
			if textTerms[term] || strings.Contains(t, term) {
				matched++
			}
		}
		score += 4 * float64(matched) / float64(len(queryTerms))
		if matched == len(queryTerms) {
			score += 2
		}
	}
	if score == 0 {
		score = 1.5 * jaccard(charNgrams(q, 2), charNgrams(t, 2))
	}
	return score
}

func searchablePromptText(result app.PromptSearchResult) string {
	base := strings.Join([]string{
		result.Prompt,
		result.Summary,
		result.MissionName,
		result.CWD,
		repoName(result.CWD),
	}, " ")
	terms := splitTerms(base)
	if len(terms) == 0 {
		return base
	}
	return base + " " + strings.Join(terms, " ")
}

func termSet(text string) map[string]bool {
	terms := map[string]bool{}
	for _, term := range splitTerms(text) {
		terms[term] = true
	}
	return terms
}

func splitTerms(text string) []string {
	text = strings.ToLower(text)
	out := make([]string, 0, 16)
	seen := map[string]bool{}
	add := func(term string) {
		term = normalizeSearchTerm(term)
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		out = append(out, term)
	}

	jiebaMu.Lock()
	seg := jiebaSegmenter()
	words := seg.CutForSearch(text, true)
	jiebaMu.Unlock()
	for _, word := range words {
		add(word)
	}
	for _, token := range codeTokenRe.FindAllString(text, -1) {
		add(token)
		for _, part := range strings.FieldsFunc(token, func(r rune) bool {
			return r == '/' || r == '.' || r == '_' || r == '-' || r == ':' || r == '@'
		}) {
			add(part)
		}
	}
	return out
}

func jiebaSegmenter() *gojieba.Jieba {
	jiebaOnce.Do(func() {
		jiebaSeg = gojieba.NewJieba()
	})
	return jiebaSeg
}

func normalizeSearchTerm(term string) string {
	term = strings.TrimSpace(strings.ToLower(term))
	term = strings.TrimFunc(term, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if term == "" {
		return ""
	}
	runes := []rune(term)
	if len(runes) > 64 {
		return ""
	}
	if len(runes) == 1 && isASCIILetterOrDigit(runes[0]) {
		return ""
	}
	return term
}

func isASCIILetterOrDigit(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func charNgrams(text string, size int) map[string]bool {
	runes := []rune(strings.TrimSpace(text))
	grams := map[string]bool{}
	if len(runes) == 0 {
		return grams
	}
	if len(runes) <= size {
		grams[string(runes)] = true
		return grams
	}
	for index := 0; index+size <= len(runes); index++ {
		gram := strings.TrimSpace(string(runes[index : index+size]))
		if gram != "" {
			grams[gram] = true
		}
	}
	return grams
}

func jaccard(a map[string]bool, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	for key := range a {
		if b[key] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func recencyBoost(at time.Time) float64 {
	if at.IsZero() {
		return 0
	}
	age := time.Since(at)
	if age < 0 {
		return 0.1
	}
	if age > Retention {
		return 0
	}
	return 0.25 * (1 - age.Hours()/Retention.Hours())
}

func compactLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit-1]) + "..."
}

func repoName(path string) string {
	path = strings.TrimRight(path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
