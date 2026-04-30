package main

import "testing"

func TestNotificationPaneID(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want int
		ok   bool
	}{
		{name: "float from json userInfo", data: map[string]interface{}{"pane_id": float64(7)}, want: 7, ok: true},
		{name: "int", data: map[string]interface{}{"pane_id": 2}, want: 2, ok: true},
		{name: "string", data: map[string]interface{}{"pane_id": "4"}, want: 4, ok: true},
		{name: "missing", data: map[string]interface{}{}, ok: false},
		{name: "negative", data: map[string]interface{}{"pane_id": float64(-1)}, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := notificationPaneID(test.data)
			if ok != test.ok {
				t.Fatalf("ok = %v, want %v", ok, test.ok)
			}
			if ok && got != test.want {
				t.Fatalf("paneID = %d, want %d", got, test.want)
			}
		})
	}
}

func TestNotificationTaskID(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want string
		ok   bool
	}{
		{name: "task id", data: map[string]interface{}{"task_id": "codex:test"}, want: "codex:test", ok: true},
		{name: "missing", data: map[string]interface{}{}, ok: false},
		{name: "empty", data: map[string]interface{}{"task_id": ""}, ok: false},
		{name: "wrong type", data: map[string]interface{}{"task_id": 12}, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := notificationTaskID(test.data)
			if ok != test.ok {
				t.Fatalf("ok = %v, want %v", ok, test.ok)
			}
			if ok && got != test.want {
				t.Fatalf("taskID = %q, want %q", got, test.want)
			}
		})
	}
}
