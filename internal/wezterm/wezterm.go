package wezterm

import (
	"context"
	"encoding/json"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"cockpit/internal/app"
	"cockpit/internal/macosfocus"
)

const weztermBundleID = "com.github.wez.wezterm"

type paneJSON struct {
	WindowID    int    `json:"window_id"`
	TabID       int    `json:"tab_id"`
	PaneID      int    `json:"pane_id"`
	Workspace   string `json:"workspace"`
	Title       string `json:"title"`
	CWD         string `json:"cwd"`
	TabTitle    string `json:"tab_title"`
	WindowTitle string `json:"window_title"`
	IsActive    bool   `json:"is_active"`
	TTYName     string `json:"tty_name"`
}

func ListPanes(ctx context.Context, weztermBin string) ([]app.Pane, error) {
	cmd := exec.CommandContext(ctx, weztermBin, "cli", "list", "--format", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var raw []paneJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	panes := make([]app.Pane, 0, len(raw))
	for _, pane := range raw {
		panes = append(panes, app.Pane{
			WindowID:    pane.WindowID,
			TabID:       pane.TabID,
			PaneID:      pane.PaneID,
			Workspace:   pane.Workspace,
			Title:       pane.Title,
			TabTitle:    pane.TabTitle,
			WindowTitle: pane.WindowTitle,
			CWD:         decodeCWD(pane.CWD),
			TTYName:     normalizeTTY(pane.TTYName),
			IsActive:    pane.IsActive,
		})
	}
	return panes, nil
}

func ActivatePane(ctx context.Context, weztermBin, paneID string) error {
	if _, err := strconv.Atoi(paneID); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, weztermBin, "cli", "activate-pane", "--pane-id", paneID)
	if err := cmd.Run(); err != nil {
		return err
	}
	_ = macosfocus.Bundle(weztermBundleID)
	return nil
}

func decodeCWD(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "file" {
		return raw
	}
	if parsed.Path == "" {
		return raw
	}
	return parsed.Path
}

func normalizeTTY(tty string) string {
	tty = strings.TrimSpace(tty)
	tty = strings.TrimPrefix(tty, "/dev/")
	return tty
}
