package gitradar

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cockpit/internal/app"
)

func Scan(ctx context.Context, cwd string) app.DiffRadar {
	radar := app.DiffRadar{GeneratedAt: time.Now()}
	if strings.TrimSpace(cwd) == "" {
		radar.LastError = "no cwd"
		return radar
	}
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	root, err := gitOutput(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		radar.LastError = "not a git worktree"
		return radar
	}
	root = strings.TrimSpace(root)
	radar.RepoRoot = root
	if branch, err := gitOutput(ctx, root, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		radar.Branch = strings.TrimSpace(branch)
	}

	status, err := gitOutput(ctx, root, "status", "--porcelain=v1")
	if err != nil {
		radar.LastError = err.Error()
		return radar
	}
	radar.Files = parseStatus(root, status)
	applyNumstat(ctx, root, radar.Files)
	sort.SliceStable(radar.Files, func(i, j int) bool {
		return radar.Files[i].Path < radar.Files[j].Path
	})
	for _, file := range radar.Files {
		radar.Dirty = true
		switch file.Status {
		case "added":
			radar.Added++
		case "deleted":
			radar.Deleted++
		case "renamed":
			radar.Renamed++
		case "untracked":
			radar.Untracked++
		default:
			radar.Modified++
		}
		if isTestPath(file.Path) {
			radar.Tests++
		}
		if isLockfile(file.Path) {
			radar.Lockfiles++
		}
	}
	radar.Summary = summary(radar)
	return radar
}

func gitOutput(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", cwd}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func parseStatus(root, raw string) []app.DiffFile {
	var files []app.DiffFile
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		if len(line) < 4 {
			continue
		}
		code := line[:2]
		path := strings.TrimSpace(line[3:])
		if path == "" {
			continue
		}
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			path = parts[len(parts)-1]
		}
		path = strings.Trim(path, `"`)
		if seen[path] {
			continue
		}
		seen[path] = true
		files = append(files, app.DiffFile{
			Path:   filepath.ToSlash(path),
			Status: statusLabel(code),
		})
	}
	return files
}

func statusLabel(code string) string {
	switch {
	case code == "??":
		return "untracked"
	case strings.Contains(code, "R"):
		return "renamed"
	case strings.Contains(code, "D"):
		return "deleted"
	case strings.Contains(code, "A"):
		return "added"
	default:
		return "modified"
	}
}

func applyNumstat(ctx context.Context, root string, files []app.DiffFile) {
	stats := map[string][2]int{}
	for _, args := range [][]string{
		{"diff", "--numstat"},
		{"diff", "--cached", "--numstat"},
	} {
		out, err := gitOutput(ctx, root, args...)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(out, "\n") {
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			path := parts[2]
			if strings.Contains(line, "=>") && len(parts) > 3 {
				path = parts[len(parts)-1]
			}
			add := atoiDash(parts[0])
			del := atoiDash(parts[1])
			key := filepath.ToSlash(strings.Trim(path, "\"{}"))
			current := stats[key]
			stats[key] = [2]int{current[0] + add, current[1] + del}
		}
	}
	for index := range files {
		if stat, ok := stats[files[index].Path]; ok {
			files[index].Additions = stat[0]
			files[index].Deletions = stat[1]
		}
	}
}

func atoiDash(value string) int {
	if value == "-" {
		return 0
	}
	var parsed int
	_, _ = fmt.Sscanf(value, "%d", &parsed)
	return parsed
}

func summary(radar app.DiffRadar) string {
	if !radar.Dirty {
		return "working tree clean"
	}
	parts := []string{}
	if radar.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d added", radar.Added))
	}
	if radar.Modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", radar.Modified))
	}
	if radar.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", radar.Deleted))
	}
	if radar.Renamed > 0 {
		parts = append(parts, fmt.Sprintf("%d renamed", radar.Renamed))
	}
	if radar.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", radar.Untracked))
	}
	if len(parts) == 0 {
		return "working tree changed"
	}
	return strings.Join(parts, ", ")
}

func isTestPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "test") ||
		strings.Contains(lower, "spec") ||
		strings.HasSuffix(lower, "_test.go")
}

func isLockfile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "go.sum", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "cargo.lock", "poetry.lock", "gemfile.lock":
		return true
	default:
		return strings.HasSuffix(base, ".lock")
	}
}
