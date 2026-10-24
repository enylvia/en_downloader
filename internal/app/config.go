package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	Port                int    `json:"port"`
	DownloadDir         string `json:"download_dir"`
	MaxConcurrentTasks  int    `json:"max_concurrent_tasks"`
	DefaultConnections  int    `json:"default_connections"`
	GlobalSpeedLimitKB  int    `json:"global_speed_limit_kb"` // 0 = unlimited
	AutoStart           bool   `json:"auto_start"`
	AutoCategorize      bool   `json:"auto_categorize"`
	OpenFolderOnFinish  bool   `json:"open_folder_on_finish"`
	EdgeIntegrated      bool   `json:"edge_integrated"`
}

type ConfigManager struct {
	configPath string
	appDataDir string
	cfg        Config
	mu         sync.RWMutex
}

func NewConfigManager() (*ConfigManager, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, _ := os.UserHomeDir()
		appData = filepath.Join(home, "AppData", "Roaming")
	}

	appDir := filepath.Join(appData, "EnylviaDownloader")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		// Fallback to local data folder if roaming AppData is restricted
		exePath, _ := os.Executable()
		exeDir := filepath.Dir(exePath)
		appDir = filepath.Join(exeDir, "data")
		_ = os.MkdirAll(appDir, 0755)
	}

	userHome, _ := os.UserHomeDir()
	defaultDownloadDir := filepath.Join(userHome, "Downloads", "Enylvia")
	if err := os.MkdirAll(defaultDownloadDir, 0755); err != nil {
		// Fallback to local downloads folder
		defaultDownloadDir = filepath.Join(appDir, "downloads")
		_ = os.MkdirAll(defaultDownloadDir, 0755)
	}

	cm := &ConfigManager{
		configPath: filepath.Join(appDir, "config.json"),
		appDataDir: appDir,
		cfg: Config{
			Port:               4242,
			DownloadDir:        defaultDownloadDir,
			MaxConcurrentTasks: 3,
			DefaultConnections: 8,
			GlobalSpeedLimitKB: 0,
			AutoStart:          true,
			AutoCategorize:     true,
			OpenFolderOnFinish: false,
			EdgeIntegrated:     false,
		},
	}

	cm.load()
	return cm, nil
}

func (cm *ConfigManager) load() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		cm.saveUnlocked()
		return
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err == nil {
		if loaded.Port == 0 {
			loaded.Port = 4242
		}
		if loaded.DownloadDir == "" {
			userHome, _ := os.UserHomeDir()
			loaded.DownloadDir = filepath.Join(userHome, "Downloads", "Enylvia")
		}
		if loaded.DefaultConnections <= 0 {
			loaded.DefaultConnections = 8
		}
		if loaded.MaxConcurrentTasks <= 0 {
			loaded.MaxConcurrentTasks = 3
		}
		cm.cfg = loaded
	}
}

func (cm *ConfigManager) saveUnlocked() error {
	data, err := json.MarshalIndent(cm.cfg, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := cm.configPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, cm.configPath)
}

func (cm *ConfigManager) Get() Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.cfg
}

func (cm *ConfigManager) SetSpeedLimitKB(kb int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.cfg.GlobalSpeedLimitKB = kb
	return cm.saveUnlocked()
}

func (cm *ConfigManager) Update(newCfg Config) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if newCfg.Port > 0 {
		cm.cfg.Port = newCfg.Port
	}
	if newCfg.DownloadDir != "" {
		cm.cfg.DownloadDir = newCfg.DownloadDir
		_ = os.MkdirAll(newCfg.DownloadDir, 0755)
	}
	if newCfg.MaxConcurrentTasks > 0 {
		cm.cfg.MaxConcurrentTasks = newCfg.MaxConcurrentTasks
	}
	if newCfg.DefaultConnections > 0 {
		cm.cfg.DefaultConnections = newCfg.DefaultConnections
	}
	cm.cfg.GlobalSpeedLimitKB = newCfg.GlobalSpeedLimitKB
	cm.cfg.AutoStart = newCfg.AutoStart
	cm.cfg.AutoCategorize = newCfg.AutoCategorize
	cm.cfg.OpenFolderOnFinish = newCfg.OpenFolderOnFinish
	cm.cfg.EdgeIntegrated = newCfg.EdgeIntegrated

	return cm.saveUnlocked()
}

func (cm *ConfigManager) AppDataDir() string {
	return cm.appDataDir
}
