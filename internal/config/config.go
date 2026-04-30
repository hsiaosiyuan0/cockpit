package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Home             string
	CodexHome        string
	ClaudeHome       string
	StateDir         string
	SettingsPath     string
	InternalRunDir   string
	CockpitOwner     string
	CodexBin         string
	WeztermBin       string
	RecentWindow     time.Duration
	IdleAfter        time.Duration
	StuckAfter       time.Duration
	SummaryInterval  time.Duration
	MaxSessions      int
	ShowUnbound      bool
	EnableLLMSummary bool
	BindingMode      string
	NotifyAttention  bool
	NotifyCompleted  bool
	NotifyStuck      bool
	NotificationMode string
	QuietHours       QuietHours
	AttentionRules   []AttentionRule
}

type QuietHours struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

type AttentionRule struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Pattern  string `json:"pattern"`
	Severity string `json:"severity"`
	Scope    string `json:"scope,omitempty"`
	Message  string `json:"message,omitempty"`
}

type Settings struct {
	CockpitOwner           string          `json:"cockpit_owner,omitempty"`
	CodexHome              string          `json:"codex_home"`
	ClaudeHome             string          `json:"claude_home"`
	CodexBin               string          `json:"codex_bin"`
	WeztermBin             string          `json:"wezterm_bin"`
	RecentWindowHours      int             `json:"recent_window_hours"`
	IdleAfterSeconds       int             `json:"idle_after_seconds"`
	StuckAfterSeconds      int             `json:"stuck_after_seconds"`
	SummaryIntervalSeconds int             `json:"summary_interval_seconds"`
	MaxSessions            int             `json:"max_sessions"`
	ShowUnbound            bool            `json:"show_unbound"`
	EnableLLMSummary       bool            `json:"enable_llm_summary"`
	BindingMode            string          `json:"binding_mode"`
	NotifyAttention        bool            `json:"notify_attention"`
	NotifyCompleted        bool            `json:"notify_completed"`
	NotifyStuck            bool            `json:"notify_stuck"`
	NotificationMode       string          `json:"notification_mode"`
	QuietHoursEnabled      bool            `json:"quiet_hours_enabled"`
	QuietHoursStart        string          `json:"quiet_hours_start"`
	QuietHoursEnd          string          `json:"quiet_hours_end"`
	AttentionRules         []AttentionRule `json:"attention_rules,omitempty"`
}

func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Home:             home,
		CodexHome:        envOr("CODEX_HOME", filepath.Join(home, ".codex")),
		ClaudeHome:       envOr("CLAUDE_HOME", filepath.Join(home, ".claude")),
		StateDir:         envOr("COCKPIT_STATE_DIR", filepath.Join(home, ".local", "state", "cockpit")),
		CockpitOwner:     envOr("COCKPIT_OWNER", ""),
		CodexBin:         envOr("COCKPIT_CODEX_BIN", "codex"),
		WeztermBin:       envOr("COCKPIT_WEZTERM_BIN", "wezterm"),
		RecentWindow:     durationEnv("COCKPIT_RECENT_WINDOW", 48*time.Hour),
		IdleAfter:        durationEnv("COCKPIT_IDLE_AFTER", 10*time.Minute),
		StuckAfter:       durationEnv("COCKPIT_STUCK_AFTER", 20*time.Minute),
		SummaryInterval:  durationEnv("COCKPIT_SUMMARY_INTERVAL", 60*time.Second),
		MaxSessions:      intEnv("COCKPIT_MAX_SESSIONS", 40),
		ShowUnbound:      boolEnv("COCKPIT_SHOW_UNBOUND", true),
		EnableLLMSummary: boolEnv("COCKPIT_ENABLE_LLM_SUMMARY", true),
		BindingMode:      envOr("COCKPIT_BINDING_MODE", "auto"),
		NotifyAttention:  boolEnv("COCKPIT_NOTIFY_ATTENTION", true),
		NotifyCompleted:  boolEnv("COCKPIT_NOTIFY_COMPLETED", true),
		NotifyStuck:      boolEnv("COCKPIT_NOTIFY_STUCK", true),
		NotificationMode: envOr("COCKPIT_NOTIFICATION_MODE", "normal"),
		QuietHours: QuietHours{
			Enabled: boolEnv("COCKPIT_QUIET_HOURS", false),
			Start:   envOr("COCKPIT_QUIET_START", "22:00"),
			End:     envOr("COCKPIT_QUIET_END", "08:00"),
		},
		AttentionRules: DefaultAttentionRules(),
	}
	cfg.SettingsPath = envOr("COCKPIT_SETTINGS_PATH", filepath.Join(cfg.StateDir, "settings.json"))
	cfg = ApplySettings(cfg, LoadSettings(cfg.SettingsPath, cfg.Settings()))
	cfg.InternalRunDir = filepath.Join(cfg.StateDir, "internal-runs")
	return cfg, nil
}

func (cfg Config) Settings() Settings {
	return Settings{
		CockpitOwner:           cfg.CockpitOwner,
		CodexHome:              cfg.CodexHome,
		ClaudeHome:             cfg.ClaudeHome,
		CodexBin:               cfg.CodexBin,
		WeztermBin:             cfg.WeztermBin,
		RecentWindowHours:      durationHours(cfg.RecentWindow),
		IdleAfterSeconds:       durationSeconds(cfg.IdleAfter),
		StuckAfterSeconds:      durationSeconds(cfg.StuckAfter),
		SummaryIntervalSeconds: durationSeconds(cfg.SummaryInterval),
		MaxSessions:            cfg.MaxSessions,
		ShowUnbound:            cfg.ShowUnbound,
		EnableLLMSummary:       cfg.EnableLLMSummary,
		BindingMode:            cfg.BindingMode,
		NotifyAttention:        cfg.NotifyAttention,
		NotifyCompleted:        cfg.NotifyCompleted,
		NotifyStuck:            cfg.NotifyStuck,
		NotificationMode:       cfg.NotificationMode,
		QuietHoursEnabled:      cfg.QuietHours.Enabled,
		QuietHoursStart:        cfg.QuietHours.Start,
		QuietHoursEnd:          cfg.QuietHours.End,
		AttentionRules:         cloneAttentionRules(cfg.AttentionRules),
	}
}

func LoadSettings(path string, fallback Settings) Settings {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	settings := fallback
	_ = json.Unmarshal(data, &settings)
	return normalizeSettings(settings, fallback)
}

func SaveSettings(path string, settings Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ApplySettings(cfg Config, settings Settings) Config {
	settings = normalizeSettings(settings, cfg.Settings())
	cfg.CockpitOwner = settings.CockpitOwner
	cfg.CodexHome = settings.CodexHome
	cfg.ClaudeHome = settings.ClaudeHome
	cfg.CodexBin = settings.CodexBin
	cfg.WeztermBin = settings.WeztermBin
	cfg.RecentWindow = time.Duration(settings.RecentWindowHours) * time.Hour
	cfg.IdleAfter = time.Duration(settings.IdleAfterSeconds) * time.Second
	cfg.StuckAfter = time.Duration(settings.StuckAfterSeconds) * time.Second
	cfg.SummaryInterval = time.Duration(settings.SummaryIntervalSeconds) * time.Second
	cfg.MaxSessions = settings.MaxSessions
	cfg.ShowUnbound = settings.ShowUnbound
	cfg.EnableLLMSummary = settings.EnableLLMSummary
	cfg.BindingMode = settings.BindingMode
	cfg.NotifyAttention = settings.NotifyAttention
	cfg.NotifyCompleted = settings.NotifyCompleted
	cfg.NotifyStuck = settings.NotifyStuck
	cfg.NotificationMode = settings.NotificationMode
	cfg.QuietHours = QuietHours{
		Enabled: settings.QuietHoursEnabled,
		Start:   settings.QuietHoursStart,
		End:     settings.QuietHoursEnd,
	}
	cfg.AttentionRules = cloneAttentionRules(settings.AttentionRules)
	return cfg
}

func normalizeSettings(settings Settings, fallback Settings) Settings {
	if settings.CodexHome == "" {
		settings.CodexHome = fallback.CodexHome
	}
	if settings.ClaudeHome == "" {
		settings.ClaudeHome = fallback.ClaudeHome
	}
	if settings.CodexBin == "" {
		settings.CodexBin = fallback.CodexBin
	}
	if settings.WeztermBin == "" {
		settings.WeztermBin = fallback.WeztermBin
	}
	if settings.RecentWindowHours <= 0 {
		settings.RecentWindowHours = fallback.RecentWindowHours
	}
	if settings.IdleAfterSeconds <= 0 {
		settings.IdleAfterSeconds = fallback.IdleAfterSeconds
	}
	if settings.StuckAfterSeconds <= 0 {
		settings.StuckAfterSeconds = fallback.StuckAfterSeconds
	}
	if settings.SummaryIntervalSeconds <= 0 {
		settings.SummaryIntervalSeconds = fallback.SummaryIntervalSeconds
	}
	if settings.MaxSessions <= 0 {
		settings.MaxSessions = fallback.MaxSessions
	}
	if settings.BindingMode != "manual" {
		settings.BindingMode = "auto"
	}
	switch settings.NotificationMode {
	case "normal", "focus", "silent":
	default:
		settings.NotificationMode = fallback.NotificationMode
		if settings.NotificationMode == "" {
			settings.NotificationMode = "normal"
		}
	}
	if settings.QuietHoursStart == "" {
		settings.QuietHoursStart = fallback.QuietHoursStart
	}
	if settings.QuietHoursEnd == "" {
		settings.QuietHoursEnd = fallback.QuietHoursEnd
	}
	settings.AttentionRules = normalizeAttentionRules(settings.AttentionRules, fallback.AttentionRules)
	return settings
}

func DefaultAttentionRules() []AttentionRule {
	return []AttentionRule{
		{ID: "permission_denied", Name: "Permission denied", Enabled: true, Pattern: "permission denied|requires approval|operation not permitted", Severity: "blocked", Message: "permission or approval needed"},
		{ID: "auth_required", Name: "Auth required", Enabled: true, Pattern: "authentication failed|unauthorized|forbidden|login required", Severity: "blocked", Message: "authentication required"},
		{ID: "merge_conflict", Name: "Merge conflict", Enabled: true, Pattern: "merge conflict|<<<<<<<|conflict", Severity: "blocked", Message: "merge conflict detected"},
		{ID: "tests_failed", Name: "Tests failed", Enabled: true, Pattern: "test failed|tests failed|failing test|1 failed|failed tests", Severity: "attention", Message: "tests failed"},
		{ID: "build_failed", Name: "Build failed", Enabled: true, Pattern: "build failed|compilation failed|compile error", Severity: "attention", Message: "build failed"},
		{ID: "runtime_error", Name: "Runtime error", Enabled: true, Pattern: "error:|exception|traceback|panic:", Severity: "attention", Message: "error detected"},
		{ID: "tool_blocked", Name: "Tool blocked", Enabled: true, Pattern: "tool_use_error|blocked:", Severity: "blocked", Message: "tool blocked"},
	}
}

func normalizeAttentionRules(rules []AttentionRule, fallback []AttentionRule) []AttentionRule {
	if len(rules) == 0 {
		return cloneAttentionRules(fallback)
	}
	defaultsByID := map[string]AttentionRule{}
	for _, rule := range DefaultAttentionRules() {
		defaultsByID[rule.ID] = rule
	}
	normalized := make([]AttentionRule, 0, len(rules))
	for _, rule := range rules {
		if rule.ID == "" {
			continue
		}
		if base, ok := defaultsByID[rule.ID]; ok {
			if rule.Name == "" {
				rule.Name = base.Name
			}
			if rule.Pattern == "" {
				rule.Pattern = base.Pattern
			}
			if rule.Severity == "" {
				rule.Severity = base.Severity
			}
			if rule.Message == "" {
				rule.Message = base.Message
			}
		}
		if rule.Severity != "blocked" && rule.Severity != "drift" {
			rule.Severity = "attention"
		}
		normalized = append(normalized, rule)
	}
	if len(normalized) == 0 {
		return cloneAttentionRules(fallback)
	}
	return normalized
}

func cloneAttentionRules(rules []AttentionRule) []AttentionRule {
	if len(rules) == 0 {
		return nil
	}
	clone := make([]AttentionRule, len(rules))
	copy(clone, rules)
	return clone
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}

func durationSeconds(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int(d.Round(time.Second) / time.Second)
}

func durationHours(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int(d.Round(time.Hour) / time.Hour)
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
