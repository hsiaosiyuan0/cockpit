package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	cockpitapp "cockpit/internal/app"
	"cockpit/internal/engine"
)

const readonlyServerDefaultAddr = "0.0.0.0:17373"

type ShareServerInfo struct {
	URL        string `json:"url"`
	ListenAddr string `json:"listen_addr"`
	Readonly   bool   `json:"readonly"`
}

type readonlyMeta struct {
	Source   string `json:"source"`
	Readonly bool   `json:"readonly"`
}

type readonlyServer struct {
	srv      *http.Server
	listener net.Listener
	info     ShareServerInfo
}

func (a *App) StartReadonlyServer() (ShareServerInfo, error) {
	if err := a.ready(); err != nil {
		return ShareServerInfo{}, err
	}

	a.mu.Lock()
	if a.share != nil {
		info := a.share.info
		a.mu.Unlock()
		return info, nil
	}
	a.mu.Unlock()

	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return ShareServerInfo{}, fmt.Errorf("open embedded frontend: %w", err)
	}

	listener, err := net.Listen("tcp", readonlyServerDefaultAddr)
	if err != nil {
		listener, err = net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			return ShareServerInfo{}, fmt.Errorf("start readonly server: %w", err)
		}
	}

	port := listener.Addr().(*net.TCPAddr).Port
	info := ShareServerInfo{
		URL:        fmt.Sprintf("http://%s:%d/", lanHost(), port),
		ListenAddr: listener.Addr().String(),
		Readonly:   true,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/meta", readonlyMethod("GET", func(w http.ResponseWriter, r *http.Request) {
		writeReadonlyJSON(w, readonlyMeta{Source: "LAN READONLY", Readonly: true})
	}))
	mux.HandleFunc("/api/snapshot", a.readonlySnapshotHandler)
	mux.HandleFunc("/api/settings", a.readonlySettingsHandler)
	mux.Handle("/", readonlyStaticHandler(dist))

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	share := &readonlyServer{srv: server, listener: listener, info: info}

	a.mu.Lock()
	if a.share != nil {
		existing := a.share.info
		a.mu.Unlock()
		_ = listener.Close()
		return existing, nil
	}
	a.share = share
	a.mu.Unlock()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("readonly cockpit server stopped: %v\n", err)
		}
	}()

	return info, nil
}

func (a *App) GetShareServer() (ShareServerInfo, error) {
	if err := a.ready(); err != nil {
		return ShareServerInfo{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.share == nil {
		return ShareServerInfo{}, nil
	}
	return a.share.info, nil
}

func (a *App) readonlySnapshotHandler(w http.ResponseWriter, r *http.Request) {
	if !requireReadonlyMethod(w, r, "GET") {
		return
	}
	if err := a.ready(); err != nil {
		writeReadonlyError(w, http.StatusInternalServerError, err)
		return
	}
	if r.URL.Query().Get("demo") == "true" {
		writeReadonlyJSON(w, cockpitapp.DemoSnapshot())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	snap, err := engine.BuildSnapshot(ctx, a.currentConfig(), a.store)
	if err != nil {
		writeReadonlyError(w, http.StatusInternalServerError, err)
		return
	}
	writeReadonlyJSON(w, snap)
}

func (a *App) readonlySettingsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireReadonlyMethod(w, r, "GET") {
		return
	}
	if err := a.ready(); err != nil {
		writeReadonlyError(w, http.StatusInternalServerError, err)
		return
	}
	writeReadonlyJSON(w, a.currentConfig().Settings())
}

func readonlyStaticHandler(dist fs.FS) http.HandlerFunc {
	files := http.FileServer(http.FS(dist))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeReadonlyError(w, http.StatusMethodNotAllowed, errors.New("readonly dashboard only supports GET"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeReadonlyError(w, http.StatusNotFound, errors.New("readonly API endpoint not found"))
			return
		}
		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		name := strings.TrimPrefix(clean, "/")
		if name != "" {
			file, err := dist.Open(name)
			if err == nil {
				defer file.Close()
				if info, statErr := file.Stat(); statErr == nil && !info.IsDir() {
					files.ServeHTTP(w, r)
					return
				}
			}
		}
		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			writeReadonlyError(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(index)
	}
}

func readonlyMethod(method string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireReadonlyMethod(w, r, method) {
			return
		}
		handler(w, r)
	}
}

func requireReadonlyMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeReadonlyError(w, http.StatusMethodNotAllowed, fmt.Errorf("readonly API only supports %s", method))
	return false
}

func writeReadonlyJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Printf("write readonly response: %v\n", err)
	}
}

func writeReadonlyError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func lanHost() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := interfaceIP(addr)
			if ip == nil || ip.To4() == nil {
				continue
			}
			if isPrivateIPv4(ip) {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}

func interfaceIP(addr net.Addr) net.IP {
	switch typed := addr.(type) {
	case *net.IPNet:
		return typed.IP
	case *net.IPAddr:
		return typed.IP
	default:
		return nil
	}
}

func isPrivateIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 10 ||
		(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
		(ip4[0] == 192 && ip4[1] == 168)
}
