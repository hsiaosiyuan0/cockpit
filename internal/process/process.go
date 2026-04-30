package process

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	"cockpit/internal/app"
)

func List(ctx context.Context) ([]app.Process, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,tty=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var processes []app.Process
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		processes = append(processes, app.Process{
			PID:  pid,
			PPID: ppid,
			TTY:  normalizeTTY(fields[2]),
			Args: strings.Join(fields[3:], " "),
		})
	}
	return processes, nil
}

func IsAgentProcess(proc app.Process, agent app.Agent) bool {
	args := strings.ToLower(proc.Args)
	switch agent {
	case app.AgentCodex:
		return strings.Contains(args, "codex") && !strings.Contains(args, "cockpit")
	case app.AgentClaude:
		return strings.Contains(args, "claude") && !strings.Contains(args, "cockpit")
	default:
		return strings.Contains(args, "codex") || strings.Contains(args, "claude")
	}
}

func normalizeTTY(tty string) string {
	tty = strings.TrimSpace(tty)
	tty = strings.TrimPrefix(tty, "/dev/")
	return tty
}
