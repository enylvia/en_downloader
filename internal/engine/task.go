package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type TaskStatus string

const (
	StatusQueued      TaskStatus = "queued"
	StatusConnecting  TaskStatus = "connecting"
	StatusDownloading TaskStatus = "downloading"
	StatusPaused      TaskStatus = "paused"
	StatusCompleted   TaskStatus = "completed"
	StatusError       TaskStatus = "error"
	StatusCanceled    TaskStatus = "canceled"
)

type Category string

const (
	CatCompressed Category = "Compressed"
	CatVideo      Category = "Video"
	CatMusic       Category = "Music"
	CatDocument   Category = "Documents"
	CatPrograms   Category = "Programs"
	CatGeneral    Category = "General"
)

type Chunk struct {
	Index      int    `json:"index"`
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Status     string `json:"status"` // pending, downloading, completed, error
}

type DownloadTask struct {
	ID             string            `json:"id"`
	URL            string            `json:"url"`
	Filename       string            `json:"filename"`
	SavePath       string            `json:"save_path"`
	Category       Category          `json:"category"`
	TotalSize      int64             `json:"total_size"`
	Downloaded     int64             `json:"downloaded"`
	Progress       float64           `json:"progress"`
	Speed          int64             `json:"speed"` // bytes/second
	ETA            string            `json:"eta"`
	NumConnections int               `json:"num_connections"`
	SupportsRange  bool              `json:"supports_range"`
	Status         TaskStatus        `json:"status"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Chunks         []Chunk           `json:"chunks"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`

	// Runtime fields (excluded from JSON persistence)
	CtxCancel   context.CancelFunc `json:"-"`
	OutFile     *os.File           `json:"-"`
	mu          sync.RWMutex       `json:"-"`
	lastBytes   int64              `json:"-"`
	lastTime    time.Time          `json:"-"`
	speedFilter []int64            `json:"-"`
}

func DetermineCategory(filename string) Category {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz", ".iso", ".tgz":
		return CatCompressed
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".ts":
		return CatVideo
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".wma", ".m4a":
		return CatMusic
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".csv", ".rtf", ".epub":
		return CatDocument
	case ".exe", ".msi", ".bat", ".cmd", ".apk", ".dmg", ".appimage", ".sh":
		return CatPrograms
	default:
		return CatGeneral
	}
}

func (t *DownloadTask) Lock() {
	t.mu.Lock()
}

func (t *DownloadTask) Unlock() {
	t.mu.Unlock()
}

func (t *DownloadTask) RLock() {
	t.mu.RLock()
}

func (t *DownloadTask) RUnlock() {
	t.mu.RUnlock()
}

func (t *DownloadTask) UpdateProgress() {
	if t.TotalSize > 0 {
		t.Progress = float64(t.Downloaded) / float64(t.TotalSize) * 100.0
		if t.Progress > 100.0 {
			t.Progress = 100.0
		}
	} else {
		t.Progress = 0.0
	}
}
