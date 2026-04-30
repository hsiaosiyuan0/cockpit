package tui

import (
	"cockpit/internal/app"
)

func demoTasks() []app.Task {
	return app.DemoTasks()
}

func DemoView(width int) string {
	model := NewDemoModel()
	model.width = width
	model.height = 42
	return model.View()
}
