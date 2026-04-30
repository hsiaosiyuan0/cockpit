package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestReadonlyStaticHandlerFallsBackToIndex(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":      &fstest.MapFile{Data: []byte("<html>cockpit</html>")},
		"assets/app.js":   &fstest.MapFile{Data: []byte("console.log('cockpit')")},
		"assets/app.css":  &fstest.MapFile{Data: []byte("body{}")},
		"assets/logo.svg": &fstest.MapFile{Data: []byte("<svg />")},
	}
	handler := readonlyStaticHandler(fs.FS(dist))

	req := httptest.NewRequest(http.MethodGet, "/sessions/music-know-vault", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "<html>cockpit</html>" {
		t.Fatalf("body = %q, want index html", body)
	}
}

func TestReadonlyStaticHandlerDoesNotServeUnknownAPIAsIndex(t *testing.T) {
	dist := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>cockpit</html>")}}
	handler := readonlyStaticHandler(fs.FS(dist))

	req := httptest.NewRequest(http.MethodGet, "/api/archive", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
