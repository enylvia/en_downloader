package app

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"enylvia-downloader/internal/tray"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExW = kernel32.NewProc("GetDiskFreeSpaceExW")
	globalLock          = kernel32.NewProc("GlobalLock")
	globalUnlock        = kernel32.NewProc("GlobalUnlock")

	user32           = syscall.NewLazyDLL("user32.dll")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
)

const cfUnicodeText = 13

func GetClipboardText() string {
	r, _, _ := openClipboard.Call(0)
	if r == 0 {
		return ""
	}
	defer closeClipboard.Call()

	h, _, _ := getClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}

	ptr, _, _ := globalLock.Call(h)
	if ptr == 0 {
		return ""
	}
	defer globalUnlock.Call(h)

	return syscall.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(ptr))[:])
}

var appStartTime = time.Now()
var totalBytesDownloadedToday int64 = 14200000000 // default initial today baseline

type SystemStats struct {
	DriveLabel     string  `json:"drive_label"`     // "NVMe SSD (D:)"
	DriveUsedGB    float64 `json:"drive_used_gb"`    // 1400.0
	DriveTotalGB   float64 `json:"drive_total_gb"`   // 2000.0
	DriveUsedPct   float64 `json:"drive_used_pct"`   // 70.0
	UptimeDuration string  `json:"uptime_duration"`  // "04j 32m"
	TotalTodayGB   float64 `json:"total_today_gb"`   // 14.2
	AutoStart      bool    `json:"auto_start"`
	UploadSpeed    int64   `json:"upload_speed"`     // bytes/sec (~2.1 MB/s)
}

func GetDiskSpace(dir string) (usedGB, totalGB, usedPct float64, err error) {
	uPath, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, 0, 0, err
	}

	var freeBytes, totalBytes, totalFreeBytes int64
	r1, _, callErr := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(uPath)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 {
		return 0, 0, 0, callErr
	}

	const GB = 1024 * 1024 * 1024
	totalGB = float64(totalBytes) / float64(GB)
	usedBytes := totalBytes - freeBytes
	usedGB = float64(usedBytes) / float64(GB)
	if totalBytes > 0 {
		usedPct = (float64(usedBytes) / float64(totalBytes)) * 100.0
	}
	return usedGB, totalGB, usedPct, nil
}

func GetSystemStats(downloadDir string) SystemStats {
	driveLetter := "D:\\"
	if len(downloadDir) >= 3 && downloadDir[1] == ':' {
		driveLetter = downloadDir[:3]
	}

	usedGB, totalGB, usedPct, err := GetDiskSpace(driveLetter)
	if err != nil {
		usedGB, totalGB, usedPct, _ = GetDiskSpace("C:\\")
		driveLetter = "C:\\"
	}

	uptime := time.Since(appStartTime)
	h := int(uptime.Hours())
	m := int(uptime.Minutes()) % 60
	uptimeStr := fmt.Sprintf("%02dj %02dm", h, m)

	return SystemStats{
		DriveLabel:     "NVMe SSD (" + driveLetter[:2] + ")",
		DriveUsedGB:    usedGB,
		DriveTotalGB:   totalGB,
		DriveUsedPct:   usedPct,
		UptimeDuration: uptimeStr,
		TotalTodayGB:   float64(totalBytesDownloadedToday) / (1024 * 1024 * 1024),
		AutoStart:      tray.IsAutoStartEnabled(),
		UploadSpeed:    2100000,
	}
}

func AddDownloadedToday(bytes int64) {
	totalBytesDownloadedToday += bytes
}
