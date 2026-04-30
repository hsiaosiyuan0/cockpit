package discovery

import (
	"context"
	"errors"
	"sort"
	"time"

	"cockpit/internal/app"
	"cockpit/internal/config"
)

func DiscoverSessions(ctx context.Context, cfg config.Config) ([]app.Session, error) {
	var errs []error
	var sessions []app.Session

	codexSessions, err := DiscoverCodex(ctx, cfg)
	if err != nil {
		errs = append(errs, err)
	}
	sessions = append(sessions, codexSessions...)

	claudeSessions, err := DiscoverClaude(ctx, cfg)
	if err != nil {
		errs = append(errs, err)
	}
	sessions = append(sessions, claudeSessions...)
	sessions = dedupeSessions(sessions)

	cutoff := time.Now().Add(-cfg.RecentWindow)
	filtered := sessions[:0]
	for _, session := range sessions {
		if session.LastEventAt.IsZero() || session.LastEventAt.After(cutoff) || session.PID != 0 {
			filtered = append(filtered, session)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].LastEventAt.After(filtered[j].LastEventAt)
	})
	if len(filtered) > cfg.MaxSessions {
		filtered = filtered[:cfg.MaxSessions]
	}
	return filtered, errors.Join(errs...)
}

func dedupeSessions(sessions []app.Session) []app.Session {
	byID := map[string]app.Session{}
	for _, session := range sessions {
		key := string(session.Agent) + ":" + session.ID
		existing, ok := byID[key]
		if !ok || session.LastEventAt.After(existing.LastEventAt) {
			byID[key] = session
		}
	}
	out := make([]app.Session, 0, len(byID))
	for _, session := range byID {
		out = append(out, session)
	}
	return out
}
