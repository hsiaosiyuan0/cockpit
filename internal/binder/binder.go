package binder

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"cockpit/internal/app"
	"cockpit/internal/process"
)

func Bind(sessions []app.Session, processes []app.Process, panes []app.Pane) map[string]app.Binding {
	byPID := map[int]*app.Process{}
	agentProcessesByTTY := map[string][]app.Process{}
	for i := range processes {
		proc := &processes[i]
		byPID[proc.PID] = proc
		for _, agent := range []app.Agent{app.AgentCodex, app.AgentClaude} {
			if process.IsAgentProcess(*proc, agent) {
				agentProcessesByTTY[proc.TTY] = append(agentProcessesByTTY[proc.TTY], *proc)
			}
		}
	}

	bindings := map[string]app.Binding{}
	for _, session := range sessions {
		taskID := app.TaskID(session)
		if session.Internal {
			bindings[taskID] = app.Binding{Confidence: -100, Reasons: []string{session.InternalReason}}
			continue
		}
		var proc *app.Process
		if session.PID != 0 {
			proc = byPID[session.PID]
		}
		bindings[taskID] = bestBinding(session, proc, panes, agentProcessesByTTY)
	}
	return bindings
}

func bestBinding(session app.Session, proc *app.Process, panes []app.Pane, agentProcessesByTTY map[string][]app.Process) app.Binding {
	var candidates []app.Binding
	for i := range panes {
		pane := panes[i]
		candidateProc := proc
		score := 0
		var reasons []string
		if candidateProc != nil && candidateProc.TTY != "" && candidateProc.TTY == pane.TTYName {
			score += 120
			reasons = append(reasons, fmt.Sprintf("pid %d tty match", candidateProc.PID))
		}
		if session.CWD != "" && pane.CWD != "" {
			switch pathRelationship(session.CWD, pane.CWD) {
			case "equal":
				score += 60
				reasons = append(reasons, "cwd match")
			case "related":
				score += 35
				reasons = append(reasons, "cwd related")
			}
		}
		for _, agentProc := range agentProcessesByTTY[pane.TTYName] {
			if process.IsAgentProcess(agentProc, session.Agent) {
				if candidateProc != nil || time.Since(session.LastEventAt) < 30*time.Minute {
					score += 40
					reasons = append(reasons, "agent process on pane tty")
				} else {
					score += 15
					reasons = append(reasons, "agent process on pane tty, idle evidence")
				}
				if candidateProc == nil {
					p := agentProc
					candidateProc = &p
				}
				break
			}
		}
		title := strings.ToLower(pane.Title + " " + pane.TabTitle + " " + pane.WindowTitle)
		if strings.Contains(title, string(session.Agent)) {
			score += 15
			reasons = append(reasons, "title contains agent")
		}
		if repo := strings.ToLower(filepath.Base(session.CWD)); repo != "" && repo != "." && strings.Contains(title, repo) {
			score += 10
			reasons = append(reasons, "title contains repo")
		}
		candidates = append(candidates, app.Binding{
			Pane:       &pane,
			Process:    candidateProc,
			Confidence: score,
			Reasons:    reasons,
		})
	}
	if len(candidates) == 0 {
		return app.Binding{Confidence: 0, Reasons: []string{"no panes"}}
	}
	best := candidates[0]
	tie := false
	for _, candidate := range candidates[1:] {
		if candidate.Confidence > best.Confidence {
			best = candidate
			tie = false
			continue
		}
		if candidate.Confidence == best.Confidence && candidate.Confidence > 0 {
			tie = true
		}
	}
	if best.Confidence < 80 {
		return app.Binding{Confidence: best.Confidence, Reasons: append(best.Reasons, "below auto-bind threshold")}
	}
	if tie {
		return app.Binding{Confidence: best.Confidence, Reasons: append(best.Reasons, "multiple equal candidates")}
	}
	return best
}

func pathRelationship(a, b string) string {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if a == b {
		return "equal"
	}
	if strings.HasPrefix(a, b+string(filepath.Separator)) || strings.HasPrefix(b, a+string(filepath.Separator)) {
		return "related"
	}
	return ""
}
