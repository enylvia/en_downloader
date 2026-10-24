package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"enylvia-downloader/internal/app"
	"enylvia-downloader/internal/engine"
	"enylvia-downloader/internal/nativehost"
	"enylvia-downloader/internal/tray"
	"enylvia-downloader/internal/window"
)

var (
	kernel32DLL       = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = kernel32DLL.NewProc("AttachConsole")
)

func attachParentConsole() {
	const attachParentProcess = ^uintptr(0) // DWORD(-1)
	_, _, _ = procAttachConsole.Call(attachParentProcess)
}

func main() {
	// 0. Immediate Check: Edge / Chromium Native Messaging Host Invocation
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "chrome-extension://") || strings.HasPrefix(arg, "extension://") || arg == "--native-host" || arg == "-native-host" {
			cfgMgr, _ := app.NewConfigManager()
			port := 4242
			if cfgMgr != nil {
				port = cfgMgr.Get().Port
			}
			if err := nativehost.RunNativeHost(port); err != nil {
				os.Exit(1)
			}
			return
		}
	}

	isNativeHost := flag.Bool("native-host", false, "Run as Microsoft Edge Native Messaging Host")
	installEdge := flag.Bool("install-edge", false, "Register Native Messaging Host for Microsoft Edge")
	installAutoStart := flag.Bool("install-autostart", false, "Enable auto-start on Windows boot")
	uninstallAutoStart := flag.Bool("uninstall-autostart", false, "Disable auto-start on Windows boot")
	isMinimized := flag.Bool("minimized", false, "Start minimized directly in Windows System Tray")
	isTray := flag.Bool("tray", false, "Alias for -minimized")
	customPort := flag.Int("port", 0, "Custom HTTP port")
	flag.Parse()

	// If command-line maintenance flags are provided, attach to parent terminal
	if *installEdge || *installAutoStart || *uninstallAutoStart {
		attachParentConsole()
	}

	cfgMgr, err := app.NewConfigManager()
	if err != nil {
		log.Fatalf("Failed to initialize configuration: %v", err)
	}

	// Setup logging to file in AppData
	logPath := filepath.Join(cfgMgr.AppDataDir(), "app.log")
	if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		log.SetOutput(logFile)
	}
	log.Printf("[INFO] EnylviaDownloader starting (PID %d)...", os.Getpid())

	cfg := cfgMgr.Get()
	port := cfg.Port
	if *customPort > 0 {
		port = *customPort
	}

	// 1. Native Messaging Host Mode for Edge (if specified by flag)
	if *isNativeHost {
		if err := nativehost.RunNativeHost(port); err != nil {
			os.Exit(1)
		}
		return
	}

	// 2. Install Edge Native Host Mode
	if *installEdge {
		fmt.Println("[EnylviaDownloader] Mendaftarkan Native Messaging Host ke Microsoft Edge...")
		if err := nativehost.InstallEdgeHost(cfgMgr.AppDataDir()); err != nil {
			log.Fatalf("Gagal mendaftarkan ke Registry Edge: %v", err)
		}
		_ = cfgMgr.Update(app.Config{EdgeIntegrated: true})
		fmt.Println("[EnylviaDownloader] Berhasil! Ekstensi Microsoft Edge sekarang dapat terhubung langsung.")
		return
	}

	// 3. Windows Auto-Start Installation
	if *installAutoStart {
		if err := tray.EnableAutoStart(); err != nil {
			log.Fatalf("Gagal mendaftarkan auto-start di registry: %v", err)
		}
		fmt.Println("[EnylviaDownloader] Auto-start berhasil diaktifkan. Aplikasi akan berjalan otomatis di system tray saat PC dinyalakan.")
		return
	}
	if *uninstallAutoStart {
		if err := tray.DisableAutoStart(); err != nil {
			log.Fatalf("Gagal menghapus auto-start dari registry: %v", err)
		}
		fmt.Println("[EnylviaDownloader] Auto-start berhasil dinonaktifkan.")
		return
	}

	startMinimized := *isMinimized || *isTray

	// Single-instance check: If EnylviaDownloader is already running, show existing window and exit cleanly
	checkClient := &http.Client{Timeout: 600 * time.Millisecond}
	if resp, err := checkClient.Get(fmt.Sprintf("http://127.0.0.1:%d/api/config", port)); err == nil {
		_ = resp.Body.Close()
		if !startMinimized {
			_, _ = checkClient.Post(fmt.Sprintf("http://127.0.0.1:%d/api/window/show", port), "application/json", nil)
		}
		os.Exit(0)
	}

	// 4. Initialize Core Engine & Store
	store := app.NewTaskStore(cfgMgr.AppDataDir())
	savedTasks, err := store.Load()
	if err != nil {
		log.Printf("[Warning] Failed to load previous tasks: %v", err)
	}

	dm := engine.NewDownloadManager(
		cfg.DownloadDir,
		cfg.MaxConcurrentTasks,
		cfg.DefaultConnections,
		cfg.GlobalSpeedLimitKB,
		cfg.AutoCategorize,
	)
	dm.RestoreTasks(savedTasks)
	dm.StartLoop()

	// Window Manager (Desktop Window)
	wm := window.NewWindowManager(cfgMgr.AppDataDir(), port)

	srv := app.NewServer(cfgMgr, dm, store, func() {
		_ = wm.CloseWindow()
	}, func() {
		_ = wm.OpenWindow()
	})

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: srv.Routes(),
	}

	// Start Windows System Tray
	_ = tray.StartTray(tray.TrayCallbacks{
		OnOpenWindow: func() {
			_ = wm.OpenWindow()
		},
		OnResumeAll: func() {
			for _, t := range dm.GetTasks() {
				if t.Status == engine.StatusPaused || t.Status == engine.StatusQueued {
					_ = dm.ResumeTask(t.ID)
				}
			}
		},
		OnPauseAll: func() {
			for _, t := range dm.GetTasks() {
				if t.Status == engine.StatusDownloading || t.Status == engine.StatusConnecting {
					_ = dm.PauseTask(t.ID)
				}
			}
		},
		OnOpenFolder: func() {
			_ = exec.Command("explorer.exe", cfg.DownloadDir).Start()
		},
		OnExit: func() {
			dm.Stop()
			_ = store.Save(dm.GetTasks())
			_ = wm.CloseWindow()
			tray.StopTray()
			_ = httpServer.Close()
			os.Exit(0)
		},
	})

	// Graceful shutdown listener
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		dm.Stop()
		_ = store.Save(dm.GetTasks())
		_ = wm.CloseWindow()
		tray.StopTray()
		_ = httpServer.Close()
		os.Exit(0)
	}()

	// Open Desktop Window if not starting minimized
	if !startMinimized {
		go func() {
			time.Sleep(500 * time.Millisecond)
			_ = wm.OpenWindow()
		}()
	} else {
		tray.ShowBalloonNotification("EnylviaDownloader", "Mesin aktif di background (System Tray). Siap mengunduh.")
	}

	// Start HTTP server in background
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
