package nativehost

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	EdgeRegKey   = `HKCU\Software\Microsoft\Edge\NativeMessagingHosts\` + HostName
	ChromeRegKey = `HKCU\Software\Google\Chrome\NativeMessagingHosts\` + HostName
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

	// Register in Microsoft Edge NativeMessagingHosts registry
	cmdEdge := exec.Command("reg", "add", EdgeRegKey, "/ve", "/t", "REG_SZ", "/d", manifestPath, "/f")
	if output, err := cmdEdge.CombinedOutput(); err != nil {
		return fmt.Errorf("edge registry registration failed: %s (%w)", string(output), err)
	}

	// Also register in Chrome key for complete Chromium ecosystem coverage
	cmdChrome := exec.Command("reg", "add", ChromeRegKey, "/ve", "/t", "REG_SZ", "/d", manifestPath, "/f")
	_ = cmdChrome.Run()

	return nil
}

func UninstallEdgeHost() error {
	cmd1 := exec.Command("reg", "delete", EdgeRegKey, "/f")
	_ = cmd1.Run()
	cmd2 := exec.Command("reg", "delete", ChromeRegKey, "/f")
	_ = cmd2.Run()
	return nil
}

func CheckEdgeHostInstalled() bool {
	cmdEdge := exec.Command("reg", "query", EdgeRegKey)
	if cmdEdge.Run() == nil {
		return true
	}
	cmdChrome := exec.Command("reg", "query", ChromeRegKey)
	return cmdChrome.Run() == nil
}
