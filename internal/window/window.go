package window

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

type WindowManager struct {
	edgePath     string
	cmd          *exec.Cmd
	appDataDir   string
	url          string
	mu           sync.Mutex
	isWindowOpen bool
}

func NewWindowManager(appDataDir string, port int) *WindowManager {
	edgePath := findEdgeExecutable()
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	return &WindowManager{
		edgePath:   edgePath,
		appDataDir: appDataDir,
		url:        url,
	}
}

func findEdgeExecutable() string {
	candidates := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Microsoft\Edge\Application\msedge.exe`),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "msedge.exe"
}

func (wm *WindowManager) prepareCleanProfile(profileDir string) {
	defaultDir := filepath.Join(profileDir, "Default")
	_ = os.MkdirAll(defaultDir, 0755)

	// Remove Web Data SQLite files to wipe any cached autofill entries
	_ = os.Remove(filepath.Join(defaultDir, "Web Data"))
	_ = os.Remove(filepath.Join(defaultDir, "Web Data-journal"))

	// Ensure Preferences explicitly turns off browser autofill, sync, and password prompts
	prefsPath := filepath.Join(defaultDir, "Preferences")
	var prefs map[string]any
	if data, err := os.ReadFile(prefsPath); err == nil {
		_ = json.Unmarshal(data, &prefs)
	}
	if prefs == nil {
		prefs = make(map[string]any)
	}

	prefs["autofill"] = map[string]any{
		"profile_enabled":                 false,
		"credit_card_enabled":             false,
		"edge_autofill_advanced_ml_enabled": false,
	}
	prefs["credentials_enable_service"] = false
	prefs["profile"] = map[string]any{
		"password_manager_enabled": false,
		"content_settings": map[string]any{
			"exceptions": map[string]any{
				"clipboard": map[string]any{
					"http://127.0.0.1:4242,*": map[string]any{"setting": 1},
					"http://localhost:4242,*": map[string]any{"setting": 1},
				},
			},
		},
	}
	prefs["sync"] = map[string]any{
		"autofill":  false,
		"passwords": false,
	}

	if updatedBytes, err := json.Marshal(prefs); err == nil {
		_ = os.WriteFile(prefsPath, updatedBytes, 0644)
	}
}

func isEdgeProfileLocked(profileDir string) bool {
	lockPath := filepath.Join(profileDir, "lockfile")
	if _, err := os.Stat(lockPath); err != nil {
		return false
	}
	file, err := os.OpenFile(lockPath, os.O_WRONLY, 0666)
	if err != nil {
		// File is in use by running Edge process
		return true
	}
	_ = file.Close()
	return false
}

func (wm *WindowManager) OpenWindow() error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	profileDir := filepath.Join(wm.appDataDir, "edge_profile")

	// If already open (process lock or marked open), do not spawn duplicate window
	if wm.isWindowOpen || isEdgeProfileLocked(profileDir) {
		wm.isWindowOpen = true
		return nil
	}

	wm.prepareCleanProfile(profileDir)

	args := []string{
		fmt.Sprintf("--app=%s", wm.url),
		"--window-size=1380,880",
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-features=TranslateUI,AutofillServerCommunication,AutofillEnableAccountWalletStorage,PasswordManager,PasswordGeneration,AutofillAddressSuggestions,AutofillCreditCard,EdgePersonalization,EdgeAutofill,EdgeWallet,msEdgePasswordManager",
		"--disable-save-password-bubble",
		"--disable-single-click-autofill",
		"--disable-autofill",
		"--disable-sync",
		"--password-store=basic",
		"--deny-permission-prompts",
		"--disable-notifications",
	}

	wm.cmd = exec.Command(wm.edgePath, args...)
	if err := wm.cmd.Start(); err != nil {
		// Fallback to ShellExecute without any console window
		shell32 := syscall.NewLazyDLL("shell32.dll")
		shellExecuteW := shell32.NewProc("ShellExecuteW")
		uOp, _ := syscall.UTF16PtrFromString("open")
		uURL, _ := syscall.UTF16PtrFromString(wm.url)
		r, _, _ := shellExecuteW.Call(0, uintptr(unsafe.Pointer(uOp)), uintptr(unsafe.Pointer(uURL)), 0, 0, 1)
		if r <= 32 {
			return fmt.Errorf("failed to open window")
		}
		wm.isWindowOpen = true
		return nil
	}

	wm.isWindowOpen = true

	go func() {
		_ = wm.cmd.Wait()
		wm.mu.Lock()
		wm.isWindowOpen = false
		wm.cmd = nil
		wm.mu.Unlock()
	}()

	return nil
}

func (wm *WindowManager) CloseWindow() error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.cmd != nil && wm.cmd.Process != nil {
		_ = wm.cmd.Process.Kill()
		wm.cmd = nil
		wm.isWindowOpen = false
	}
	return nil
}

func (wm *WindowManager) IsOpen() bool {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	return wm.isWindowOpen
}
