package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type AddTaskOptions struct {
	URL            string            `json:"url"`
	Filename       string            `json:"filename"`
	SavePath       string            `json:"save_path"`
	Category       Category          `json:"category"`
	NumConnections int               `json:"num_connections"`
	Headers        map[string]string `json:"headers"`
	AutoStart      bool              `json:"auto_start"`
}

type DownloadManager struct {
	tasks           map[string]*DownloadTask
	taskOrder       []string
	maxConcurrent   int
	defaultConns    int
	defaultDir      string
	speedLimitKB    int
	rateLimiter     *RateLimiter
	autoCategorize  bool
	client          *http.Client
	mu              sync.RWMutex
	saveCallback    func([]*DownloadTask)
	eventBroadcast  func(event string, data any)
	stopChan        chan struct{}
}

func NewDownloadManager(defaultDir string, maxConcurrent int, defaultConns int, speedLimitKB int, autoCategorize bool) *DownloadManager {
	if defaultConns <= 0 {
		defaultConns = 8
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   0, // No overall timeout for large file downloads
	}

	dm := &DownloadManager{
		tasks:          make(map[string]*DownloadTask),
		taskOrder:      make([]string, 0),
		maxConcurrent:  maxConcurrent,
		defaultConns:   defaultConns,
		defaultDir:     defaultDir,
		speedLimitKB:   speedLimitKB,
		rateLimiter:    NewRateLimiter(speedLimitKB),
		autoCategorize: autoCategorize,
		client:         client,
		stopChan:       make(chan struct{}),
	}

	return dm
}

func (dm *DownloadManager) SetSaveCallback(fn func([]*DownloadTask)) {
	dm.saveCallback = fn
}

func (dm *DownloadManager) SetEventBroadcast(fn func(event string, data any)) {
	dm.eventBroadcast = fn
}

func (dm *DownloadManager) SetConfig(maxConcurrent, defaultConns, speedLimitKB int, defaultDir string, autoCategorize bool) {
	dm.mu.Lock()
	if maxConcurrent > 0 {
		dm.maxConcurrent = maxConcurrent
	}
	if defaultConns > 0 {
		dm.defaultConns = defaultConns
	}
	dm.speedLimitKB = speedLimitKB
	if defaultDir != "" {
		dm.defaultDir = defaultDir
	}
	dm.autoCategorize = autoCategorize
	dm.mu.Unlock()

	if dm.rateLimiter != nil {
		dm.rateLimiter.SetLimit(speedLimitKB)
	}
}

func (dm *DownloadManager) SetSpeedLimit(speedLimitKB int) {
	dm.mu.Lock()
	dm.speedLimitKB = speedLimitKB
	dm.mu.Unlock()

	if dm.rateLimiter != nil {
		dm.rateLimiter.SetLimit(speedLimitKB)
	}
}

func (dm *DownloadManager) GetSpeedLimit() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.speedLimitKB
}

func (dm *DownloadManager) StartLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	saveTicker := time.NewTicker(3 * time.Second)

	go func() {
		for {
			select {
			case <-dm.stopChan:
				ticker.Stop()
				saveTicker.Stop()
				return
			case now := <-ticker.C:
				dm.updateSpeeds(now)
				dm.checkQueue()
				if dm.eventBroadcast != nil {
					dm.eventBroadcast("tasks_update", dm.GetTasks())
				}
			case <-saveTicker.C:
				dm.triggerSave()
			}
		}
	}()
}

func (dm *DownloadManager) Stop() {
	close(dm.stopChan)
	dm.mu.Lock()
	defer dm.mu.Unlock()
	for _, t := range dm.tasks {
		if t.CtxCancel != nil {
			t.CtxCancel()
		}
		if t.OutFile != nil {
			_ = t.OutFile.Close()
		}
	}
}

func (dm *DownloadManager) RestoreTasks(tasks []*DownloadTask) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for _, t := range tasks {
		dm.tasks[t.ID] = t
		dm.taskOrder = append(dm.taskOrder, t.ID)
	}
}

func (dm *DownloadManager) GetTasks() []*DownloadTask {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	res := make([]*DownloadTask, 0, len(dm.taskOrder))
	for _, id := range dm.taskOrder {
		if t, ok := dm.tasks[id]; ok {
			res = append(res, t)
		}
	}
	return res
}

func (dm *DownloadManager) GetTask(id string) (*DownloadTask, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	task, ok := dm.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	return task, nil
}

func sanitizeFilename(name string) string {
	invalidChars := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)
	cleaned := invalidChars.ReplaceAllString(name, "_")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" || cleaned == "." {
		cleaned = "download.bin"
	}
	return cleaned
}

func parseFilenameFromHeader(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err == nil {
		if fn, ok := params["filename*"]; ok {
			// RFC 5987 / 6266: UTF-8''encoded_name
			parts := strings.SplitN(fn, "''", 2)
			if len(parts) == 2 {
				if unescaped, err := url.PathUnescape(parts[1]); err == nil {
					return unescaped
				}
			}
		}
		if fn, ok := params["filename"]; ok {
			return fn
		}
	}
	return ""
}

func (dm *DownloadManager) ProbeURL(ctx context.Context, targetURL string, headers map[string]string) (filename string, size int64, supportsRange bool, err error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", targetURL, nil)
	if err != nil {
		return "", 0, false, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := dm.client.Do(req)
	// Some servers reject HEAD with 405 Method Not Allowed or 403. Fallback to GET range 0-0.
	if err != nil || (resp != nil && resp.StatusCode >= 400) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		getReq, getErr := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
		if getErr != nil {
			return "", 0, false, getErr
		}
		getReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0")
		getReq.Header.Set("Range", "bytes=0-0")
		for k, v := range headers {
			getReq.Header.Set(k, v)
		}

		resp, err = dm.client.Do(getReq)
		if err != nil {
			return "", 0, false, err
		}
	}
	defer resp.Body.Close()

	// Extract filename
	contentDisposition := resp.Header.Get("Content-Disposition")
	filename = parseFilenameFromHeader(contentDisposition)
	if filename == "" {
		parsed, parseErr := url.Parse(targetURL)
		if parseErr == nil {
			pathBase := filepath.Base(parsed.Path)
			if pathBase != "" && pathBase != "/" && pathBase != "." {
				unescaped, _ := url.PathUnescape(pathBase)
				filename = unescaped
			}
		}
	}
	if filename == "" {
		filename = "download.bin"
	}
	filename = sanitizeFilename(filename)

	// Check Range support
	acceptRanges := resp.Header.Get("Accept-Ranges")
	contentRange := resp.Header.Get("Content-Range")
	if strings.EqualFold(acceptRanges, "bytes") || contentRange != "" || resp.StatusCode == http.StatusPartialContent {
		supportsRange = true
	}

	// Extract total size
	if contentRange != "" {
		// Format: bytes 0-0/12345
		parts := strings.Split(contentRange, "/")
		if len(parts) == 2 {
			if s, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				size = s
			}
		}
	}
	if size == 0 {
		contentLength := resp.Header.Get("Content-Length")
		if contentLength != "" {
			if s, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
				size = s
			}
		}
	}

	return filename, size, supportsRange, nil
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func getUniqueFilePath(dir, filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	target := filepath.Join(dir, filename)

	counter := 1
	for {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			return target
		}
		target = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, counter, ext))
		counter++
	}
}

func (dm *DownloadManager) AddTask(opts AddTaskOptions) (*DownloadTask, error) {
	if opts.URL == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	probedName, size, supportsRange, err := dm.ProbeURL(ctx, opts.URL, opts.Headers)
	if err != nil {
		// Even if probe fails, allow adding with fallback
		probedName = "download.bin"
		size = 0
		supportsRange = false
	}

	finalFilename := opts.Filename
	if finalFilename == "" {
		finalFilename = probedName
	}
	finalFilename = sanitizeFilename(finalFilename)

	category := opts.Category
	if category == "" {
		category = DetermineCategory(finalFilename)
	}

	saveDir := opts.SavePath
	if saveDir == "" || filepath.Clean(saveDir) == filepath.Clean(dm.defaultDir) {
		saveDir = dm.defaultDir
		if dm.autoCategorize {
			saveDir = filepath.Join(saveDir, string(category))
		}
	}
	_ = os.MkdirAll(saveDir, 0755)

	finalFilePath := getUniqueFilePath(saveDir, finalFilename)

	numConns := opts.NumConnections
	if numConns <= 0 {
		numConns = dm.defaultConns
	}
	if !supportsRange || size <= 0 {
		numConns = 1
	}

	task := &DownloadTask{
		ID:             generateID(),
		URL:            opts.URL,
		Filename:       filepath.Base(finalFilePath),
		SavePath:       finalFilePath,
		Category:       category,
		TotalSize:      size,
		Downloaded:     0,
		Progress:       0.0,
		Speed:          0,
		ETA:            "--:--",
		NumConnections: numConns,
		SupportsRange:  supportsRange,
		Status:         StatusQueued,
		Headers:        opts.Headers,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Prepare chunks
	if size > 0 && supportsRange && numConns > 1 {
		chunkSize := size / int64(numConns)
		task.Chunks = make([]Chunk, numConns)
		for i := 0; i < numConns; i++ {
			start := int64(i) * chunkSize
			end := start + chunkSize - 1
			if i == numConns-1 {
				end = size - 1
			}
			task.Chunks[i] = Chunk{
				Index:      i,
				Start:      start,
				End:        end,
				Downloaded: 0,
				Total:      end - start + 1,
				Status:     "pending",
			}
		}
	} else {
		task.Chunks = []Chunk{
			{
				Index:      0,
				Start:      0,
				End:        size - 1,
				Downloaded: 0,
				Total:      size,
				Status:     "pending",
			},
		}
	}

	dm.mu.Lock()
	dm.tasks[task.ID] = task
	dm.taskOrder = append([]string{task.ID}, dm.taskOrder...) // Add to top
	dm.mu.Unlock()

	dm.triggerSave()

	if opts.AutoStart {
		_ = dm.StartTask(task.ID)
	}

	return task, nil
}

func (dm *DownloadManager) StartTask(id string) error {
	dm.mu.Lock()
	task, ok := dm.tasks[id]
	if !ok {
		dm.mu.Unlock()
		return fmt.Errorf("task not found")
	}

	if task.Status == StatusDownloading {
		dm.mu.Unlock()
		return nil
	}

	activeCount := dm.countActiveTasksUnlocked()
	if activeCount >= dm.maxConcurrent {
		task.Status = StatusQueued
		dm.mu.Unlock()
		return nil
	}

	task.Status = StatusDownloading
	task.ErrorMessage = ""
	ctx, cancel := context.WithCancel(context.Background())
	task.CtxCancel = cancel
	dm.mu.Unlock()

	go dm.executeDownload(ctx, task)
	return nil
}

func (dm *DownloadManager) PauseTask(id string) error {
	dm.mu.Lock()
	task, ok := dm.tasks[id]
	if !ok {
		dm.mu.Unlock()
		return fmt.Errorf("task not found")
	}

	if task.Status != StatusDownloading && task.Status != StatusConnecting && task.Status != StatusQueued {
		dm.mu.Unlock()
		return nil
	}

	if task.CtxCancel != nil {
		task.CtxCancel()
	}
	if task.OutFile != nil {
		_ = task.OutFile.Close()
		task.OutFile = nil
	}

	task.Status = StatusPaused
	task.Speed = 0
	task.ETA = "--:--"
	dm.mu.Unlock()

	dm.triggerSave()
	return nil
}

func (dm *DownloadManager) ResumeTask(id string) error {
	return dm.StartTask(id)
}

func (dm *DownloadManager) DeleteTask(id string, deleteFile bool) error {
	dm.mu.Lock()
	task, ok := dm.tasks[id]
	if !ok {
		dm.mu.Unlock()
		return fmt.Errorf("task not found")
	}

	if task.CtxCancel != nil {
		task.CtxCancel()
	}
	if task.OutFile != nil {
		_ = task.OutFile.Close()
		task.OutFile = nil
	}

	filePath := task.SavePath
	delete(dm.tasks, id)

	newOrder := make([]string, 0, len(dm.taskOrder)-1)
	for _, tid := range dm.taskOrder {
		if tid != id {
			newOrder = append(newOrder, tid)
		}
	}
	dm.taskOrder = newOrder
	dm.mu.Unlock()

	if deleteFile && filePath != "" {
		_ = os.Remove(filePath)
	}

	dm.triggerSave()
	return nil
}

func (dm *DownloadManager) RetryTask(id string) error {
	dm.mu.Lock()
	task, ok := dm.tasks[id]
	if !ok {
		dm.mu.Unlock()
		return fmt.Errorf("task not found")
	}

	if task.CtxCancel != nil {
		task.CtxCancel()
	}
	if task.OutFile != nil {
		_ = task.OutFile.Close()
		task.OutFile = nil
	}

	// Reset progress if desired or retry from existing
	task.Status = StatusQueued
	task.ErrorMessage = ""
	dm.mu.Unlock()

	return dm.StartTask(id)
}

func (dm *DownloadManager) countActiveTasksUnlocked() int {
	count := 0
	for _, t := range dm.tasks {
		if t.Status == StatusDownloading || t.Status == StatusConnecting {
			count++
		}
	}
	return count
}

func (dm *DownloadManager) checkQueue() {
	dm.mu.Lock()
	activeCount := dm.countActiveTasksUnlocked()
	if activeCount >= dm.maxConcurrent {
		dm.mu.Unlock()
		return
	}

	var nextTask *DownloadTask
	for _, id := range dm.taskOrder {
		t := dm.tasks[id]
		if t.Status == StatusQueued {
			nextTask = t
			break
		}
	}

	if nextTask == nil {
		dm.mu.Unlock()
		return
	}

	nextTask.Status = StatusDownloading
	nextTask.ErrorMessage = ""
	ctx, cancel := context.WithCancel(context.Background())
	nextTask.CtxCancel = cancel
	dm.mu.Unlock()

	go dm.executeDownload(ctx, nextTask)
}

func (dm *DownloadManager) executeDownload(ctx context.Context, task *DownloadTask) {
	defer func() {
		task.Lock()
		if task.OutFile != nil {
			_ = task.OutFile.Close()
			task.OutFile = nil
		}
		task.Speed = 0
		task.ETA = "--:--"
		task.Unlock()
		dm.triggerSave()
		dm.checkQueue()
	}()

	// Open or create destination file
	file, err := os.OpenFile(task.SavePath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		task.Lock()
		task.Status = StatusError
		task.ErrorMessage = fmt.Sprintf("Failed to open file: %v", err)
		task.Unlock()
		return
	}

	// Pre-allocate or truncate if size is known and file is smaller
	if task.TotalSize > 0 {
		stat, statErr := file.Stat()
		if statErr == nil && stat.Size() < task.TotalSize {
			_ = file.Truncate(task.TotalSize)
		}
	}

	task.Lock()
	task.OutFile = file
	task.Unlock()

	// If server does not support range or single chunk, do single stream download
	if !task.SupportsRange || len(task.Chunks) <= 1 {
		dm.executeSingleStreamDownload(ctx, task)
		return
	}

	// Multi-part download
	var wg sync.WaitGroup
	errChan := make(chan error, len(task.Chunks))

	for i := range task.Chunks {
		chunkIndex := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := NewChunkWorker(dm.client, task, chunkIndex, dm.rateLimiter)
			if err := worker.Download(ctx); err != nil {
				if ctx.Err() == nil {
					errChan <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// Check if canceled or paused
	if ctx.Err() != nil {
		return
	}

	// Check if any chunk failed
	var downloadErr error
	for e := range errChan {
		if downloadErr == nil {
			downloadErr = e
		}
	}

	task.Lock()
	if downloadErr != nil {
		task.Status = StatusError
		task.ErrorMessage = downloadErr.Error()
	} else if task.Downloaded >= task.TotalSize {
		task.Status = StatusCompleted
		now := time.Now()
		task.CompletedAt = &now
		task.Progress = 100.0
	}
	task.Unlock()
}

func (dm *DownloadManager) executeSingleStreamDownload(ctx context.Context, task *DownloadTask) {
	req, err := http.NewRequestWithContext(ctx, "GET", task.URL, nil)
	if err != nil {
		task.Lock()
		task.Status = StatusError
		task.ErrorMessage = err.Error()
		task.Unlock()
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0")
	for k, v := range task.Headers {
		req.Header.Set(k, v)
	}

	resp, err := dm.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			task.Lock()
			task.Status = StatusError
			task.ErrorMessage = err.Error()
			task.Unlock()
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		task.Lock()
		task.Status = StatusError
		task.ErrorMessage = fmt.Sprintf("HTTP status %d", resp.StatusCode)
		task.Unlock()
		return
	}

	buf := make([]byte, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			task.Lock()
			file := task.OutFile
			offset := task.Downloaded
			task.Unlock()

			if file != nil {
				_, _ = file.WriteAt(buf[:n], offset)
			}

			task.Lock()
			task.Downloaded += int64(n)
			task.UpdateProgress()
			task.Unlock()

			if dm.rateLimiter != nil {
				dm.rateLimiter.Wait(ctx, n)
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			task.Lock()
			task.Status = StatusError
			task.ErrorMessage = readErr.Error()
			task.Unlock()
			return
		}
	}

	task.Lock()
	task.Status = StatusCompleted
	now := time.Now()
	task.CompletedAt = &now
	task.Progress = 100.0
	task.Unlock()
}

func (dm *DownloadManager) updateSpeeds(now time.Time) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	for _, t := range dm.tasks {
		if t.Status == StatusDownloading {
			t.CalculateInstantSpeed(now)
		}
	}
}

func (dm *DownloadManager) triggerSave() {
	if dm.saveCallback == nil {
		return
	}
	tasks := dm.GetTasks()
	dm.saveCallback(tasks)
}

func (dm *DownloadManager) OpenFile(id string) error {
	task, err := dm.GetTask(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(task.SavePath); err != nil {
		return fmt.Errorf("file not found on disk")
	}

	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecuteW := shell32.NewProc("ShellExecuteW")
	uOp, _ := syscall.UTF16PtrFromString("open")
	uPath, _ := syscall.UTF16PtrFromString(task.SavePath)
	r, _, _ := shellExecuteW.Call(0, uintptr(unsafe.Pointer(uOp)), uintptr(unsafe.Pointer(uPath)), 0, 0, 1)
	if r <= 32 {
		return fmt.Errorf("failed to open file")
	}
	return nil
}

func (dm *DownloadManager) OpenFolder(id string) error {
	task, err := dm.GetTask(id)
	if err != nil {
		return err
	}

	targetPath := filepath.Clean(task.SavePath)
	dir := filepath.Dir(targetPath)
	_ = os.MkdirAll(dir, 0755)

	var cmd *exec.Cmd
	if _, err := os.Stat(targetPath); err == nil {
		// File physically exists on disk: Open folder and select/highlight the file
		cmd = exec.Command("explorer.exe")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CmdLine: fmt.Sprintf(`explorer.exe /select,"%s"`, targetPath),
		}
	} else {
		// File not created yet or in progress: Open destination folder directly
		cmd = exec.Command("explorer.exe", dir)
	}

	return cmd.Start()
}
