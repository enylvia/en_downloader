package engine

import (
	"fmt"
	"time"
)

func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func FormatSpeed(bytesPerSec int64) string {
	return FormatBytes(bytesPerSec) + "/s"
}

func CalculateETA(remainingBytes int64, speedBytesPerSec int64) string {
	if speedBytesPerSec <= 0 || remainingBytes <= 0 {
		return "--:--"
	}

	seconds := remainingBytes / speedBytesPerSec
	if seconds > 86400*30 { // > 30 days
		return "> 30d"
	}

	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

func (t *DownloadTask) CalculateInstantSpeed(now time.Time) {
	t.Lock()
	defer t.Unlock()

	if t.lastTime.IsZero() {
		t.lastTime = now
		t.lastBytes = t.Downloaded
		return
	}

	deltaSec := now.Sub(t.lastTime).Seconds()
	if deltaSec < 0.25 {
		return
	}

	deltaBytes := t.Downloaded - t.lastBytes
	if deltaBytes < 0 {
		deltaBytes = 0
	}

	instantSpeed := int64(float64(deltaBytes) / deltaSec)
	t.lastBytes = t.Downloaded
	t.lastTime = now

	// Smoothing filter (moving average of last 4 samples)
	t.speedFilter = append(t.speedFilter, instantSpeed)
	if len(t.speedFilter) > 4 {
		t.speedFilter = t.speedFilter[len(t.speedFilter)-4:]
	}

	var sum int64
	for _, s := range t.speedFilter {
		sum += s
	}
	t.Speed = sum / int64(len(t.speedFilter))

	remaining := t.TotalSize - t.Downloaded
	if remaining > 0 && t.Speed > 0 {
		t.ETA = CalculateETA(remaining, t.Speed)
	} else if remaining <= 0 {
		t.ETA = "00:00"
	} else {
		t.ETA = "--:--"
	}
}
