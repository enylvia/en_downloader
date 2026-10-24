package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"enylvia-downloader/internal/engine"
)

type TaskStore struct {
	filePath string
	mu       sync.Mutex
}

func NewTaskStore(appDataDir string) *TaskStore {
	return &TaskStore{
		filePath: filepath.Join(appDataDir, "tasks.json"),
	}
}

func (s *TaskStore) Load() ([]*engine.DownloadTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*engine.DownloadTask{}, nil
		}
		return nil, err
	}

	var tasks []*engine.DownloadTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return []*engine.DownloadTask{}, nil
	}

	for _, t := range tasks {
		// Clean up in-flight statuses on restart
		if t.Status == engine.StatusDownloading || t.Status == engine.StatusConnecting {
			t.Status = engine.StatusPaused
		}
		t.Speed = 0
		t.ETA = "--:--"
	}

	return tasks, nil
}

func (s *TaskStore) Save(tasks []*engine.DownloadTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clone to avoid data race while serializing
	cloned := make([]engine.DownloadTask, 0, len(tasks))
	for _, t := range tasks {
		t.RLock()
		cloned = append(cloned, *t)
		t.RUnlock()
	}

	data, err := json.MarshalIndent(cloned, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, s.filePath)
}
