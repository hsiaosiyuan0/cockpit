package promptsearch

import (
	"path/filepath"
	"testing"
	"time"

	"cockpit/internal/app"

	_ "modernc.org/sqlite"
)

func TestIndexKeywordSearchFindsUserPrompts(t *testing.T) {
	index := openTestIndex(t)
	now := time.Now()
	snapshot := app.Snapshot{
		Tasks: []app.Task{
			{
				ID: "codex:one",
				Session: app.Session{
					ID:          "one",
					Agent:       app.AgentCodex,
					CWD:         "/tmp/release-web",
					LastEventAt: now,
					Events: []app.Event{
						{At: now.Add(-2 * time.Minute), Type: app.EventUser, Text: "verify release-web build and auth redirect"},
						{At: now.Add(-1 * time.Minute), Type: app.EventAssistant, Text: "build passed"},
					},
				},
			},
			{
				ID: "claude:two",
				Session: app.Session{
					ID:          "two",
					Agent:       app.AgentClaude,
					CWD:         "/tmp/payments-api",
					LastEventAt: now.Add(-10 * time.Minute),
					Events: []app.Event{
						{At: now.Add(-12 * time.Minute), Type: app.EventUser, Text: "refresh payment webhook auth flow"},
					},
				},
				Archived: true,
			},
		},
		Missions: []app.Mission{
			{ID: "repo:/tmp/release-web", Name: "release-web", TaskIDs: []string{"codex:one"}},
		},
	}

	if synced, err := index.Sync(snapshot, Retention); err != nil {
		t.Fatalf("sync: %v", err)
	} else if synced != 2 {
		t.Fatalf("expected 2 synced records including hidden prompt, got %d", synced)
	}

	results, err := index.Search("auth redirect", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one visible result, got %d", len(results))
	}
	if results[0].TaskID != "codex:one" {
		t.Fatalf("expected codex:one, got %s", results[0].TaskID)
	}
	if results[0].MissionName != "release-web" {
		t.Fatalf("expected mission name release-web, got %q", results[0].MissionName)
	}
}

func TestIndexKeywordSearchUsesChineseSegmentation(t *testing.T) {
	index := openTestIndex(t)
	now := time.Now()
	snapshot := app.Snapshot{
		Tasks: []app.Task{
			{
				ID: "codex:music",
				Session: app.Session{
					ID:          "music",
					Agent:       app.AgentCodex,
					CWD:         "/tmp/music-know-vault",
					LastEventAt: now,
					Events: []app.Event{
						{At: now, Type: app.EventUser, Text: "帮我梳理云音乐生产环境的 ZK 中间件升级方案"},
					},
				},
			},
		},
	}

	if _, err := index.Sync(snapshot, Retention); err != nil {
		t.Fatalf("sync: %v", err)
	}
	results, err := index.Search("音乐中间件", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one segmented Chinese result, got %d", len(results))
	}
	if results[0].TaskID != "codex:music" {
		t.Fatalf("expected codex:music, got %s", results[0].TaskID)
	}
}

func TestIndexKeywordSearchIncludesCWDTokens(t *testing.T) {
	index := openTestIndex(t)
	now := time.Now()
	snapshot := app.Snapshot{
		Tasks: []app.Task{
			{
				ID: "codex:repo",
				Session: app.Session{
					ID:          "repo",
					Agent:       app.AgentCodex,
					CWD:         "/Users/hsy/work/kb-workspace/music-know-vault",
					LastEventAt: now,
					Events: []app.Event{
						{At: now, Type: app.EventUser, Text: "补充 release skill 的单测"},
					},
				},
			},
		},
	}

	if _, err := index.Sync(snapshot, Retention); err != nil {
		t.Fatalf("sync: %v", err)
	}
	results, err := index.Search("music vault", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected repo token result, got %d", len(results))
	}
}

func TestIndexExpiresOldPrompts(t *testing.T) {
	index := openTestIndex(t)
	now := time.Now()
	old := now.Add(-16 * 24 * time.Hour)
	snapshot := app.Snapshot{
		Tasks: []app.Task{
			{
				ID: "codex:old",
				Session: app.Session{
					ID:          "old",
					Agent:       app.AgentCodex,
					CWD:         "/tmp/cockpit",
					LastEventAt: old,
					Events: []app.Event{
						{At: old, Type: app.EventUser, Text: "ancient prompt"},
					},
				},
			},
			{
				ID: "codex:new",
				Session: app.Session{
					ID:          "new",
					Agent:       app.AgentCodex,
					CWD:         "/tmp/cockpit",
					LastEventAt: now,
					Events: []app.Event{
						{At: now, Type: app.EventUser, Text: "fresh prompt"},
					},
				},
			},
		},
	}
	if _, err := index.Sync(snapshot, Retention); err != nil {
		t.Fatalf("sync: %v", err)
	}
	results, err := index.Search("prompt", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Prompt != "fresh prompt" {
		t.Fatalf("expected only fresh prompt, got %#v", results)
	}
}

func TestIndexReturnsEmptySlicesForNoResults(t *testing.T) {
	index := openTestIndex(t)
	now := time.Now()
	snapshot := app.Snapshot{
		Tasks: []app.Task{
			{
				ID: "codex:one",
				Session: app.Session{
					ID:          "one",
					Agent:       app.AgentCodex,
					CWD:         "/tmp/cockpit",
					LastEventAt: now,
					Events: []app.Event{
						{At: now, Type: app.EventUser, Text: "ship prompt archive search"},
					},
				},
			},
		},
	}
	if _, err := index.Sync(snapshot, Retention); err != nil {
		t.Fatalf("sync: %v", err)
	}

	keyword, err := index.Search("definitely missing phrase", 10)
	if err != nil {
		t.Fatalf("keyword search: %v", err)
	}
	if keyword == nil {
		t.Fatalf("expected keyword search to return an empty slice, got nil")
	}
	if len(keyword) != 0 {
		t.Fatalf("expected no keyword results, got %d", len(keyword))
	}
}

func TestSchemaUsesFTS5SearchText(t *testing.T) {
	index := openTestIndex(t)
	var ftsName string
	if err := index.db.QueryRow(`select name from sqlite_master where type = 'table' and name = 'prompt_fts'`).Scan(&ftsName); err != nil {
		t.Fatalf("prompt_fts missing: %v", err)
	}
	var count int
	if err := index.db.QueryRow(`select count(*) from pragma_table_info('prompts') where name = 'search_text'`).Scan(&count); err != nil {
		t.Fatalf("inspect prompts: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected prompts.search_text column")
	}
	if err := index.db.QueryRow(`select count(*) from sqlite_master where name in ('prompt_vectors', '_vec_prompt_vectors', 'vector_storage')`).Scan(&count); err != nil {
		t.Fatalf("inspect vector artifacts: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no embedding/vector tables, got %d", count)
	}
}

func openTestIndex(t *testing.T) *Index {
	t.Helper()
	index, err := Open(filepath.Join(t.TempDir(), "prompts.sqlite"))
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	t.Cleanup(func() {
		_ = index.Close()
	})
	return index
}
