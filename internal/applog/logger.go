package applog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type Logger struct {
	mu   sync.Mutex
	file *os.File
	log  *log.Logger
	path string
}

func New(stateDir string) (*Logger, error) {
	if stateDir == "" {
		return nil, fmt.Errorf("empty state dir")
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(stateDir, "cockpit.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{
		file: file,
		log:  log.New(file, "", log.LstdFlags|log.Lmicroseconds),
		path: path,
	}, nil
}

func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *Logger) Printf(format string, args ...any) {
	if l == nil || l.log == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.log.Printf(format, args...)
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
