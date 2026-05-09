package discovery

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"

	"cockpit/internal/app"
	"cockpit/internal/redact"
)

func scanJSONL(path string, fn func(map[string]any)) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 20*1024*1024)
	for scanner.Scan() {
		var obj map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &obj); err == nil {
			fn(obj)
		}
	}
	return scanner.Err()
}

func stringField(obj map[string]any, key string) string {
	value, ok := obj[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func mapField(obj map[string]any, key string) map[string]any {
	value, ok := obj[key].(map[string]any)
	if !ok {
		return nil
	}
	return value
}

func sliceField(obj map[string]any, key string) []any {
	value, ok := obj[key].([]any)
	if !ok {
		return nil
	}
	return value
}

func timeField(obj map[string]any, key string) time.Time {
	raw := stringField(obj, key)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func addEvent(session *app.Session, event app.Event) {
	if event.At.IsZero() {
		event.At = time.Now()
	}
	event.Text = compact(event.Text, 900)
	event.Detail = compact(event.Detail, 900)
	if session.CreatedAt.IsZero() || event.At.Before(session.CreatedAt) {
		session.CreatedAt = event.At
	}
	if session.LastEventAt.IsZero() || event.At.After(session.LastEventAt) {
		session.LastEventAt = event.At
	}
	session.Events = append(session.Events, event)
}

func compact(text string, limit int) string {
	text = app.CleanString(text)
	text = redact.Text(text)
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	if limit < 4 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func lastEvents(events []app.Event, max int) []app.Event {
	if len(events) <= max {
		return events
	}
	return events[len(events)-max:]
}
