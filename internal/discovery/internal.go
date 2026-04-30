package discovery

import (
	"strings"

	"cockpit/internal/app"
	"cockpit/internal/config"
)

func markInternal(session *app.Session, cfg config.Config) {
	if session.CWD != "" && strings.HasPrefix(session.CWD, cfg.InternalRunDir) {
		session.Internal = true
		session.InternalReason = "cwd under cockpit internal run dir"
		return
	}
	for _, event := range session.Events {
		if strings.Contains(event.Text, "COCKPIT_INTERNAL_RUN_ID") || strings.Contains(event.Detail, "COCKPIT_INTERNAL_RUN_ID") {
			session.Internal = true
			session.InternalReason = "internal marker in log"
			return
		}
	}
}
