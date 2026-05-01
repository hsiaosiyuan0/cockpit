package execpath

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Resolve finds command-line tools from GUI-launched apps, where PATH usually
// does not include Homebrew, fnm, nvm, pnpm, or other shell-managed locations.
func Resolve(bin string) string {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return bin
	}
	if strings.ContainsRune(bin, filepath.Separator) {
		return bin
	}
	if path, err := exec.LookPath(bin); err == nil {
		return path
	}
	for _, candidate := range Candidates(bin) {
		if isExecutable(candidate) {
			return candidate
		}
	}
	return bin
}

func Candidates(bin string) []string {
	bin = strings.TrimSpace(bin)
	if bin == "" || strings.ContainsRune(bin, filepath.Separator) {
		return nil
	}
	home, _ := os.UserHomeDir()
	var dirs []string
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "bin"),
			filepath.Join(home, "go", "bin"),
			filepath.Join(home, ".cargo", "bin"),
			filepath.Join(home, "Library", "pnpm"),
			filepath.Join(home, ".bun", "bin"),
			filepath.Join(home, ".npm-global", "bin"),
			filepath.Join(home, ".yarn", "bin"),
		)
		dirs = append(dirs, globDirs(filepath.Join(home, ".local", "share", "fnm", "node-versions", "*", "installation", "bin"))...)
		dirs = append(dirs, globDirs(filepath.Join(home, ".nvm", "versions", "node", "*", "bin"))...)
	}
	dirs = append(dirs,
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
		"/usr/local/sbin",
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
	)
	if runtime.GOOS == "darwin" {
		dirs = append(dirs,
			"/Applications/WezTerm.app/Contents/MacOS",
			filepath.Join(home, "Applications", "WezTerm.app", "Contents", "MacOS"),
		)
	}
	out := make([]string, 0, len(dirs))
	seen := map[string]bool{}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, bin)
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	return out
}

func globDirs(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err == nil && info.IsDir() {
			out = append(out, match)
		}
	}
	return out
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
