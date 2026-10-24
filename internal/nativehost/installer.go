package nativehost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"enylvia-downloader/internal/winreg"
)

type NativeHostManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

const (
	HostName     = "com.enylvia.downloader"
	EdgeSubKey   = `Software\Microsoft\Edge\NativeMessagingHosts\` + HostName
	ChromeSubKey = `Software\Google\Chrome\NativeMessagingHosts\` + HostName
	ExtensionID  = "leeinjkmocmbogffanjegflpmbfmhogj"
)

func InstallEdgeHost(appDataDir string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return err
	}

	// Create Manifest JSON pointing directly to the compiled executable
	manifest := NativeHostManifest{
		Name:        HostName,
		Description: "EnylviaDownloader Native Messaging Host for Microsoft Edge",
		Path:        exePath,
		Type:        "stdio",
		AllowedOrigins: []string{
			fmt.Sprintf("chrome-extension://%s/", ExtensionID),
			fmt.Sprintf("extension://%s/", ExtensionID),
		},
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(appDataDir, HostName+".json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Also write batch wrapper just in case
	launcherPath := filepath.Join(appDataDir, "edge-host.bat")
	batContent := fmt.Sprintf("@echo off\r\n\"%s\" --native-host %%*\r\n", exePath)
	_ = os.WriteFile(launcherPath, []byte(batContent), 0755)

	// Register in Microsoft Edge & Google Chrome registry natively via Win32 API
	if err := winreg.SetStringValue(EdgeSubKey, "", manifestPath); err != nil {
		return fmt.Errorf("edge registry registration failed: %w", err)
	}
	_ = winreg.SetStringValue(ChromeSubKey, "", manifestPath)

	return nil
}

func UninstallEdgeHost() error {
	_ = winreg.DeleteKey(EdgeSubKey)
	_ = winreg.DeleteKey(ChromeSubKey)
	return nil
}

func CheckEdgeHostInstalled() bool {
	return winreg.KeyExists(EdgeSubKey) || winreg.KeyExists(ChromeSubKey)
}
