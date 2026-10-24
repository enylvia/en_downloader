package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"enylvia-downloader/internal/engine"
	"enylvia-downloader/internal/nativehost"
	"enylvia-downloader/internal/tray"
	"enylvia-downloader/internal/web"
)

type Server struct {
	cfgMgr       *ConfigManager
	dm           *engine.DownloadManager
	store        *TaskStore
	sseClients   map[chan sseMessage]struct{}
	sseMu        sync.Mutex
	onHideWindow func()
	onShowWindow func()
	lastEdgePing time.Time
	pingMu       sync.Mutex
}

type sseMessage struct {
	event string
	data  string
}

func NewServer(cfgMgr *ConfigManager, dm *engine.DownloadManager, store *TaskStore, onHideWindow func(), onShowWindow func()) *Server {
	s := &Server{
		cfgMgr:       cfgMgr,
		dm:           dm,
		store:        store,
		sseClients:   make(map[chan sseMessage]struct{}),
		onHideWindow: onHideWindow,
		onShowWindow: onShowWindow,
	}

	dm.SetSaveCallback(func(tasks []*engine.DownloadTask) {
		_ = store.Save(tasks)
	})

	dm.SetEventBroadcast(func(event string, data any) {
		b, err := json.Marshal(data)
		if err != nil {
			return
		}
		s.broadcastSSE(event, string(b))
	})

	return s
}

func (s *Server) broadcastSSE(event, data string) {
	s.sseMu.Lock()
	defer s.sseMu.Unlock()

	msg := sseMessage{event: event, data: data}
	for ch := range s.sseClients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Static web dashboard
	fileServer := http.FileServer(web.GetFileSystem())
	mux.Handle("/", fileServer)

	// API routes
	mux.HandleFunc("/api/events", s.handleSSE)
	mux.HandleFunc("/api/tasks", s.handleGetTasks)
	mux.HandleFunc("/api/tasks/probe", s.handleProbeURL)
	mux.HandleFunc("/api/tasks/add", s.handleAddTask)
	mux.HandleFunc("/api/tasks/pause", s.handlePauseTask)
	mux.HandleFunc("/api/tasks/resume", s.handleResumeTask)
	mux.HandleFunc("/api/tasks/delete", s.handleDeleteTask)
	mux.HandleFunc("/api/tasks/retry", s.handleRetryTask)
	mux.HandleFunc("/api/tasks/open-file", s.handleOpenFile)
	mux.HandleFunc("/api/tasks/open-folder", s.handleOpenFolder)
	mux.HandleFunc("/api/tasks/resume-all", s.handleResumeAll)
	mux.HandleFunc("/api/tasks/pause-all", s.handlePauseAll)
	mux.HandleFunc("/api/tasks/clear-completed", s.handleClearCompleted)

	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/speed-limit", s.handleSpeedLimit)
	mux.HandleFunc("/api/system/stats", s.handleSystemStats)
	mux.HandleFunc("/api/system/autostart", s.handleAutoStart)
	mux.HandleFunc("/api/window/minimize", s.handleWindowMinimize)
	mux.HandleFunc("/api/window/close", s.handleWindowClose)

	mux.HandleFunc("/api/edge/status", s.handleEdgeStatus)
	mux.HandleFunc("/api/edge/install", s.handleEdgeInstall)
	mux.HandleFunc("/api/edge/ping", s.handleEdgePing)

	return withCORS(mux)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan sseMessage, 32)
	s.sseMu.Lock()
	s.sseClients[ch] = struct{}{}
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.sseClients, ch)
		close(ch)
		s.sseMu.Unlock()
	}()

	// Send initial state immediately
	initialData, _ := json.Marshal(s.dm.GetTasks())
	fmt.Fprintf(w, "event: tasks_update\ndata: %s\n\n", string(initialData))
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.event, msg.data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	tasks := s.dm.GetTasks()
	jsonResponse(w, http.StatusOK, tasks)
}

func (s *Server) handleProbeURL(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "url parameter is required"})
		return
	}

	ctx, cancel := r.Context(), func() {}
	defer cancel()

	filename, size, supportsRange, err := s.dm.ProbeURL(ctx, targetURL, nil)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]any{
			"error":          err.Error(),
			"filename":       filename,
			"size":           size,
			"supports_range": supportsRange,
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"filename":       filename,
		"size":           size,
		"supports_range": supportsRange,
	})
}

func (s *Server) handleAddTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL            string            `json:"url"`
		Filename       string            `json:"filename"`
		SavePath       string            `json:"save_path"`
		Category       string            `json:"category"`
		NumConnections int               `json:"num_connections"`
		Headers        map[string]string `json:"headers"`
		AutoStart      bool              `json:"auto_start"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	cfg := s.cfgMgr.Get()
	conns := req.NumConnections
	if conns <= 0 {
		conns = cfg.DefaultConnections
	}

	task, err := s.dm.AddTask(engine.AddTaskOptions{
		URL:            req.URL,
		Filename:       req.Filename,
		SavePath:       req.SavePath,
		Category:       engine.Category(req.Category),
		NumConnections: conns,
		Headers:        req.Headers,
		AutoStart:      req.AutoStart,
	})

	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if s.onShowWindow != nil {
		go s.onShowWindow()
	}

	jsonResponse(w, http.StatusCreated, map[string]any{"task": task})
}

func (s *Server) handlePauseTask(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "id parameter required"})
		return
	}
	if err := s.dm.PauseTask(id); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleResumeTask(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "id parameter required"})
		return
	}
	if err := s.dm.ResumeTask(id); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	deleteFile, _ := strconv.ParseBool(r.URL.Query().Get("delete_file"))
	if id == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "id parameter required"})
		return
	}
	if err := s.dm.DeleteTask(id, deleteFile); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleRetryTask(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "id parameter required"})
		return
	}
	if err := s.dm.RetryTask(id); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleOpenFile(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "id parameter required"})
		return
	}
	if err := s.dm.OpenFile(id); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "id parameter required"})
		return
	}
	if err := s.dm.OpenFolder(id); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleResumeAll(w http.ResponseWriter, r *http.Request) {
	tasks := s.dm.GetTasks()
	for _, t := range tasks {
		if t.Status == engine.StatusPaused || t.Status == engine.StatusQueued {
			_ = s.dm.ResumeTask(t.ID)
		}
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handlePauseAll(w http.ResponseWriter, r *http.Request) {
	tasks := s.dm.GetTasks()
	for _, t := range tasks {
		if t.Status == engine.StatusDownloading || t.Status == engine.StatusConnecting {
			_ = s.dm.PauseTask(t.ID)
		}
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleClearCompleted(w http.ResponseWriter, r *http.Request) {
	tasks := s.dm.GetTasks()
	for _, t := range tasks {
		if t.Status == engine.StatusCompleted {
			_ = s.dm.DeleteTask(t.ID, false)
		}
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfgMgr.Get()
	stats := GetSystemStats(cfg.DownloadDir)
	jsonResponse(w, http.StatusOK, stats)
}

func (s *Server) handleAutoStart(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		enabled := tray.ToggleAutoStart()
		jsonResponse(w, http.StatusOK, map[string]bool{"auto_start": enabled})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"auto_start": tray.IsAutoStartEnabled()})
}

func (s *Server) handleWindowMinimize(w http.ResponseWriter, r *http.Request) {
	if s.onHideWindow != nil {
		s.onHideWindow()
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleWindowClose(w http.ResponseWriter, r *http.Request) {
	if s.onHideWindow != nil {
		s.onHideWindow()
		tray.ShowBalloonNotification("EnylviaDownloader", "Aplikasi diminimalkan ke System Tray. Klik ikon untuk membukanya kembali.")
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		jsonResponse(w, http.StatusOK, s.cfgMgr.Get())
		return
	}

	if r.Method == http.MethodPost {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "Failed to read request body"})
			return
		}

		var raw map[string]any
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
			return
		}

		if val, exists := raw["global_speed_limit_kb"]; exists {
			var limit int
			switch v := val.(type) {
			case float64:
				limit = int(v)
			case int:
				limit = v
			}
			_ = s.cfgMgr.SetSpeedLimitKB(limit)
			s.dm.SetSpeedLimit(limit)
		}

		var newCfg Config
		_ = json.Unmarshal(bodyBytes, &newCfg)
		if newCfg.DownloadDir != "" || newCfg.MaxConcurrentTasks > 0 || newCfg.DefaultConnections > 0 {
			if err := s.cfgMgr.Update(newCfg); err != nil {
				jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			cur := s.cfgMgr.Get()
			s.dm.SetConfig(cur.MaxConcurrentTasks, cur.DefaultConnections, cur.GlobalSpeedLimitKB, cur.DownloadDir, cur.AutoCategorize)
		}

		cur := s.cfgMgr.Get()
		jsonResponse(w, http.StatusOK, cur)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleSpeedLimit(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		jsonResponse(w, http.StatusOK, map[string]int{"limit_kb": s.dm.GetSpeedLimit()})
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			LimitKB int `json:"limit_kb"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			_ = s.cfgMgr.SetSpeedLimitKB(req.LimitKB)
			s.dm.SetSpeedLimit(req.LimitKB)
		}
		jsonResponse(w, http.StatusOK, map[string]int{"limit_kb": s.dm.GetSpeedLimit()})
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleEdgePing(w http.ResponseWriter, r *http.Request) {
	s.pingMu.Lock()
	s.lastEdgePing = time.Now()
	s.pingMu.Unlock()
	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleEdgeStatus(w http.ResponseWriter, r *http.Request) {
	installed := nativehost.CheckEdgeHostInstalled()
	s.pingMu.Lock()
	isLive := !s.lastEdgePing.IsZero() && time.Since(s.lastEdgePing) < 30*time.Second
	s.pingMu.Unlock()

	jsonResponse(w, http.StatusOK, map[string]any{
		"installed": installed,
		"live":      isLive,
	})
}

func (s *Server) handleEdgeInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := nativehost.InstallEdgeHost(s.cfgMgr.AppDataDir())
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}

	_ = s.cfgMgr.Update(Config{EdgeIntegrated: true})
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
