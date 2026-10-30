package engine

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type memoryTransport struct {
	payload []byte
}

func (m *memoryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := &http.Response{
		Header:     make(http.Header),
		Request:    req,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}

	resp.Header.Set("Accept-Ranges", "bytes")
	resp.Header.Set("Content-Type", "application/octet-stream")
	resp.Header.Set("Content-Disposition", `attachment; filename="test_large_file.bin"`)

	totalLen := len(m.payload)

	if req.Method == "HEAD" {
		resp.StatusCode = http.StatusOK
		resp.Status = "200 OK"
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", totalLen))
		resp.Body = io.NopCloser(bytes.NewReader(nil))
		return resp, nil
	}

	rangeHeader := req.Header.Get("Range")
	if rangeHeader == "" {
		resp.StatusCode = http.StatusOK
		resp.Status = "200 OK"
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", totalLen))
		resp.Body = io.NopCloser(bytes.NewReader(m.payload))
		return resp, nil
	}

	var start, end int64
	_, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
	if err != nil || end >= int64(totalLen) || start > end {
		resp.StatusCode = http.StatusRequestedRangeNotSatisfiable
		resp.Status = "416 Range Not Satisfiable"
		resp.Body = io.NopCloser(bytes.NewReader(nil))
		return resp, nil
	}

	resp.StatusCode = http.StatusPartialContent
	resp.Status = "206 Partial Content"
	resp.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalLen))
	chunkLen := end - start + 1
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", chunkLen))
	resp.Body = io.NopCloser(bytes.NewReader(m.payload[start : end+1]))
	return resp, nil
}

func TestDetermineCategory(t *testing.T) {
	tests := []struct {
		filename string
		expected Category
	}{
		{"archive.zip", CatCompressed},
		{"disk.bin", CatCompressed},
		{"image.iso", CatCompressed},
		{"movie.mkv", CatVideo},
		{"clip.mp4", CatVideo},
		{"song.mp3", CatMusic},
		{"track.flac", CatMusic},
		{"document.pdf", CatDocument},
		{"notes.md", CatDocument},
		{"config.json", CatDocument},
		{"installer.exe", CatPrograms},
		{"setup.msi", CatPrograms},
		{"app.apk", CatPrograms},
		{"random.unknown", CatGeneral},
	}

	for _, tc := range tests {
		got := DetermineCategory(tc.filename)
		if got != tc.expected {
			t.Errorf("DetermineCategory(%s) = %v; want %v", tc.filename, got, tc.expected)
		}
	}
}

func TestAutoCategorizeSubfolders(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "enylvia_cat_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	client := &http.Client{
		Transport: &memoryTransport{payload: []byte("test")},
	}

	// 1. With autoCategorize = true
	dm := NewDownloadManager(tempDir, 2, 4, 0, true)
	dm.client = client

	taskBin, err := dm.AddTask(AddTaskOptions{
		URL:      "http://virtual-test.local/archive.bin",
		Filename: "archive.bin",
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	expectedBinDir := filepath.Join(tempDir, "Compressed")
	if filepath.Dir(taskBin.SavePath) != expectedBinDir {
		t.Errorf("Expected save dir %s, got %s", expectedBinDir, filepath.Dir(taskBin.SavePath))
	}

	taskDoc, err := dm.AddTask(AddTaskOptions{
		URL:      "http://virtual-test.local/manual.pdf",
		Filename: "manual.pdf",
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	expectedDocDir := filepath.Join(tempDir, "Documents")
	if filepath.Dir(taskDoc.SavePath) != expectedDocDir {
		t.Errorf("Expected save dir %s, got %s", expectedDocDir, filepath.Dir(taskDoc.SavePath))
	}

	taskGen, err := dm.AddTask(AddTaskOptions{
		URL:      "http://virtual-test.local/unknown.xyz",
		Filename: "unknown.xyz",
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	expectedGenDir := filepath.Join(tempDir, "General")
	if filepath.Dir(taskGen.SavePath) != expectedGenDir {
		t.Errorf("Expected save dir %s, got %s", expectedGenDir, filepath.Dir(taskGen.SavePath))
	}

	// 2. With autoCategorize = false
	dmDisabled := NewDownloadManager(tempDir, 2, 4, 0, false)
	dmDisabled.client = client

	taskNoCat, err := dmDisabled.AddTask(AddTaskOptions{
		URL:      "http://virtual-test.local/plain.zip",
		Filename: "plain.zip",
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	if filepath.Dir(taskNoCat.SavePath) != tempDir {
		t.Errorf("Expected save dir %s, got %s", tempDir, filepath.Dir(taskNoCat.SavePath))
	}
}

func TestFormatBytesAndSpeed(t *testing.T) {
	if got := FormatBytes(500); got != "500 B" {
		t.Errorf("FormatBytes(500) = %s; want 500 B", got)
	}
	if got := FormatBytes(1024); got != "1.00 KB" {
		t.Errorf("FormatBytes(1024) = %s; want 1.00 KB", got)
	}
	if got := FormatBytes(1048576); got != "1.00 MB" {
		t.Errorf("FormatBytes(1048576) = %s; want 1.00 MB", got)
	}
	if got := FormatSpeed(2048); got != "2.00 KB/s" {
		t.Errorf("FormatSpeed(2048) = %s; want 2.00 KB/s", got)
	}
}

func TestDownloadManagerChunkedDownload(t *testing.T) {
	testPayload := make([]byte, 512*1024) // 512 KB
	for i := range testPayload {
		testPayload[i] = byte((i * 7) % 256)
	}

	client := &http.Client{
		Transport: &memoryTransport{payload: testPayload},
	}

	tempDir, err := os.MkdirTemp("", "enylvia_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dm := NewDownloadManager(tempDir, 2, 4, 0, false)
	dm.client = client

	task, err := dm.AddTask(AddTaskOptions{
		URL:            "http://virtual-test.local/test_large_file.bin",
		NumConnections: 4,
		AutoStart:      true,
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	if task.TotalSize != int64(len(testPayload)) {
		t.Fatalf("Expected total size %d, got %d", len(testPayload), task.TotalSize)
	}
	if len(task.Chunks) != 4 {
		t.Fatalf("Expected 4 chunks, got %d", len(task.Chunks))
	}

	// Wait for completion
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task.RLock()
		st := task.Status
		task.RUnlock()
		if st == StatusCompleted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	task.RLock()
	defer task.RUnlock()
	if task.Status != StatusCompleted {
		t.Fatalf("Task did not complete in time, status: %s, error: %s", task.Status, task.ErrorMessage)
	}

	// Verify file on disk
	downloadedBytes, err := os.ReadFile(task.SavePath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if len(downloadedBytes) != len(testPayload) {
		t.Fatalf("File size mismatch: got %d, expected %d", len(downloadedBytes), len(testPayload))
	}

	for i := range testPayload {
		if downloadedBytes[i] != testPayload[i] {
			t.Fatalf("Byte mismatch at offset %d: got %x, expected %x", i, downloadedBytes[i], testPayload[i])
		}
	}

	_ = filepath.Base(task.SavePath)
}

func TestRateLimiterThrottling(t *testing.T) {
	// 100 KB/s
	rl := NewRateLimiter(100)
	ctx := t.Context()

	start := time.Now()
	// Consume 150 KB (initial 100KB tokens + 50KB deficit)
	rl.Wait(ctx, 150*1024)
	elapsed := time.Since(start)

	// 50KB deficit at 100KB/s requires ~500ms sleep
	if elapsed < 250*time.Millisecond {
		t.Fatalf("Rate limiter did not throttle: elapsed %v", elapsed)
	}

	// Dynamic change to unlimited
	rl.SetLimit(0)
	start = time.Now()
	rl.Wait(ctx, 500*1024)
	elapsed = time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("Rate limiter did not unthrottle immediately: elapsed %v", elapsed)
	}
}
