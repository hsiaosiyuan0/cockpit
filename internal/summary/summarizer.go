package summary

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cockpit/internal/app"
	"cockpit/internal/config"
	"cockpit/internal/redact"
)

type Store interface {
	Config() config.Config
	SaveSummary(app.SummaryCache) error
	LoadSummary(string) (app.SummaryCache, bool)
	SaveDebrief(app.Debrief) error
	LoadDebrief(string) (app.Debrief, bool)
}

type CodexSummarizer struct {
	cfg   config.Config
	store Store
}

type response struct {
	OneLine         string  `json:"one_line"`
	Attention       bool    `json:"attention"`
	AttentionReason string  `json:"attention_reason"`
	Progress        any     `json:"progress"`
	NextLikelyStep  string  `json:"next_likely_step"`
	Confidence      float64 `json:"confidence"`
}

func NewCodexSummarizer(cfg config.Config, store Store) *CodexSummarizer {
	return &CodexSummarizer{cfg: cfg, store: store}
}

func (s *CodexSummarizer) NeedsRefresh(task app.Task) bool {
	inputHash := InputHash(task)
	cache, ok := s.store.LoadSummary(task.ID)
	if !ok {
		return true
	}
	if cache.InputHash != inputHash {
		return true
	}
	return time.Since(cache.GeneratedAt) > s.cfg.SummaryInterval
}

func (s *CodexSummarizer) Refresh(ctx context.Context, task *app.Task) error {
	if task == nil {
		return nil
	}
	if task.Session.Internal {
		return nil
	}
	inputHash := InputHash(*task)
	runID := fmt.Sprintf("ckpt_%d", time.Now().UnixNano())
	runDir := filepath.Join(s.cfg.InternalRunDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	outFile := filepath.Join(runDir, "summary.json")
	prompt := buildPrompt(*task, runID)

	cmd := exec.CommandContext(ctx,
		s.cfg.CodexBin,
		"exec",
		"--ephemeral",
		"--skip-git-repo-check",
		"--ignore-rules",
		"--cd", runDir,
		"--sandbox", "read-only",
		"--output-last-message", outFile,
		"-",
	)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = append(os.Environ(),
		"COCKPIT_INTERNAL=1",
		"COCKPIT_ROLE=summarizer",
		"COCKPIT_INTERNAL_RUN_ID="+runID,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex exec failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		return err
	}
	summary := parseSummary(string(data))
	if summary == "" {
		summary = strings.TrimSpace(string(data))
	}
	if summary == "" {
		return fmt.Errorf("empty summary from codex exec")
	}
	task.Summary = summary
	task.SummaryAt = time.Now()
	return s.store.SaveSummary(app.SummaryCache{
		TaskID:      task.ID,
		SessionID:   task.Session.ID,
		Summary:     summary,
		GeneratedAt: task.SummaryAt,
		InputHash:   inputHash,
		Raw:         string(data),
	})
}

func (s *CodexSummarizer) Debrief(ctx context.Context, task *app.Task) error {
	if task == nil {
		return nil
	}
	if task.Session.Internal {
		return nil
	}
	inputHash := DebriefInputHash(*task)
	if cache, ok := s.store.LoadDebrief(task.ID); ok && cache.InputHash == inputHash {
		task.Debrief = &cache
		return nil
	}
	runID := fmt.Sprintf("debrief_%d", time.Now().UnixNano())
	runDir := filepath.Join(s.cfg.InternalRunDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	outFile := filepath.Join(runDir, "debrief.md")
	prompt := buildDebriefPrompt(*task, runID)
	cmd := exec.CommandContext(ctx,
		s.cfg.CodexBin,
		"exec",
		"--ephemeral",
		"--skip-git-repo-check",
		"--ignore-rules",
		"--cd", runDir,
		"--sandbox", "read-only",
		"--output-last-message", outFile,
		"-",
	)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = append(os.Environ(),
		"COCKPIT_INTERNAL=1",
		"COCKPIT_ROLE=debrief",
		"COCKPIT_INTERNAL_RUN_ID="+runID,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex debrief failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return fmt.Errorf("empty debrief from codex exec")
	}
	debrief := app.Debrief{
		TaskID:      task.ID,
		SessionID:   task.Session.ID,
		GeneratedAt: time.Now(),
		Text:        text,
		InputHash:   inputHash,
		Raw:         string(data),
	}
	task.Debrief = &debrief
	return s.store.SaveDebrief(debrief)
}

func InputHash(task app.Task) string {
	h := sha1.New()
	_, _ = h.Write([]byte(task.Session.ID))
	_, _ = h.Write([]byte(task.Status))
	_, _ = h.Write([]byte(task.AttentionReason))
	start := 0
	if len(task.Session.Events) > 30 {
		start = len(task.Session.Events) - 30
	}
	for _, event := range task.Session.Events[start:] {
		_, _ = h.Write([]byte(event.At.Format(time.RFC3339Nano)))
		_, _ = h.Write([]byte(event.Type))
		_, _ = h.Write([]byte(event.Text))
		_, _ = h.Write([]byte(event.Detail))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func DebriefInputHash(task app.Task) string {
	h := sha1.New()
	_, _ = h.Write([]byte(InputHash(task)))
	_, _ = h.Write([]byte(task.Diff.Summary))
	for _, file := range task.Diff.Files {
		_, _ = h.Write([]byte(file.Status))
		_, _ = h.Write([]byte(file.Path))
	}
	for _, event := range task.Flight {
		_, _ = h.Write([]byte(event.Kind))
		_, _ = h.Write([]byte(event.Title))
		_, _ = h.Write([]byte(event.Detail))
		_, _ = h.Write([]byte(event.Status))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func buildPrompt(task app.Task, runID string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "COCKPIT_INTERNAL_RUN_ID=%s\n", runID)
	fmt.Fprintf(&b, "You summarize an observed Claude/Codex CLI task for a local cockpit dashboard.\n")
	fmt.Fprintf(&b, "Do not run tools. Return JSON only, with keys one_line, attention, attention_reason, progress, next_likely_step, confidence.\n")
	fmt.Fprintf(&b, "Keep one_line under 120 Chinese characters or 180 English characters. Avoid mentioning this internal summarization run.\n\n")
	fmt.Fprintf(&b, "Task:\nagent=%s\nsession_id=%s\ncwd=%s\nstatus=%s\nattention_reason=%s\n\n", task.Session.Agent, task.Session.ID, task.Session.CWD, task.Status, task.AttentionReason)
	fmt.Fprintf(&b, "Recent events, oldest to newest:\n")
	start := 0
	if len(task.Session.Events) > 28 {
		start = len(task.Session.Events) - 28
	}
	for _, event := range task.Session.Events[start:] {
		text := event.Text
		if event.Detail != "" {
			text += " | " + event.Detail
		}
		fmt.Fprintf(&b, "- %s [%s] %s\n", event.At.Format(time.RFC3339), event.Type, truncate(text, 500))
	}
	return b.String()
}

func buildDebriefPrompt(task app.Task, runID string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "COCKPIT_INTERNAL_RUN_ID=%s\n", runID)
	fmt.Fprintf(&b, "You write a concise debrief for a locally observed Claude/Codex CLI task.\n")
	fmt.Fprintf(&b, "Do not run tools. Use only the provided log, trace, status, and git radar data.\n")
	fmt.Fprintf(&b, "Return markdown with exactly these sections: Completed, Current State, Changed Files, Tests, Risks, Next Action.\n")
	fmt.Fprintf(&b, "Keep it compact and actionable. Avoid mentioning this internal debrief run.\n\n")
	fmt.Fprintf(&b, "Task:\nagent=%s\nsession_id=%s\ncwd=%s\nstatus=%s\nstatus_reason=%s\nsummary=%s\n\n", task.Session.Agent, task.Session.ID, task.Session.CWD, task.Status, task.StatusExplain.Reason, task.Summary)
	fmt.Fprintf(&b, "Diff Radar:\nrepo=%s\nbranch=%s\ndirty=%v\nsummary=%s\n", task.Diff.RepoRoot, task.Diff.Branch, task.Diff.Dirty, task.Diff.Summary)
	for _, file := range task.Diff.Files {
		fmt.Fprintf(&b, "- %s %s +%d -%d\n", file.Status, file.Path, file.Additions, file.Deletions)
	}
	fmt.Fprintf(&b, "\nFlight events, oldest to newest:\n")
	start := 0
	if len(task.Flight) > 36 {
		start = len(task.Flight) - 36
	}
	for _, event := range task.Flight[start:] {
		fmt.Fprintf(&b, "- %s [%s/%s] %s %s\n", event.At.Format(time.RFC3339), event.Source, event.Kind, truncate(event.Title, 160), truncate(event.Detail, 360))
	}
	return b.String()
}

func parseSummary(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	if oneLine := oneLineFromMap([]byte(raw)); oneLine != "" {
		return oneLine
	}
	var parsed response
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil && parsed.OneLine != "" {
		return strings.TrimSpace(parsed.OneLine)
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err == nil && parsed.OneLine != "" {
			return strings.TrimSpace(parsed.OneLine)
		}
	}
	return ""
}

func oneLineFromMap(data []byte) string {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return ""
	}
	value, ok := obj["one_line"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func truncate(text string, limit int) string {
	text = strings.ToValidUTF8(text, "")
	text = redact.Text(text)
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit < 4 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
