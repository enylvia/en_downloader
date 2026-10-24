package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type ChunkWorker struct {
	client      *http.Client
	task        *DownloadTask
	chunkIndex  int
	rateLimiter *RateLimiter
}

func NewChunkWorker(client *http.Client, task *DownloadTask, chunkIndex int, rateLimiter *RateLimiter) *ChunkWorker {
	return &ChunkWorker{
		client:      client,
		task:        task,
		chunkIndex:  chunkIndex,
		rateLimiter: rateLimiter,
	}
}

func (w *ChunkWorker) Download(ctx context.Context) error {
	w.task.Lock()
	if w.chunkIndex >= len(w.task.Chunks) {
		w.task.Unlock()
		return fmt.Errorf("invalid chunk index %d", w.chunkIndex)
	}
	chunk := &w.task.Chunks[w.chunkIndex]
	if chunk.Downloaded >= chunk.Total {
		chunk.Status = "completed"
		w.task.Unlock()
		return nil
	}

	rangeStart := chunk.Start + chunk.Downloaded
	rangeEnd := chunk.End
	chunk.Status = "downloading"
	w.task.Unlock()

	req, err := http.NewRequestWithContext(ctx, "GET", w.task.URL, nil)
	if err != nil {
		return err
	}

	// Set headers (custom headers, range header, default user agent)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0")
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd))

	for k, v := range w.task.Headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		w.task.Lock()
		w.task.Chunks[w.chunkIndex].Status = "error"
		w.task.Unlock()
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		w.task.Lock()
		w.task.Chunks[w.chunkIndex].Status = "error"
		w.task.Unlock()
		return fmt.Errorf("unexpected status code %d for chunk %d", resp.StatusCode, w.chunkIndex)
	}

	buf := make([]byte, 32*1024) // 32KB read buffer for smoother rate limiting
	currentOffset := rangeStart

	for {
		select {
		case <-ctx.Done():
			w.task.Lock()
			w.task.Chunks[w.chunkIndex].Status = "pending"
			w.task.Unlock()
			return ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			// Write directly to file at calculated byte offset
			w.task.Lock()
			file := w.task.OutFile
			w.task.Unlock()

			if file == nil {
				return fmt.Errorf("file handle is closed")
			}

			_, writeErr := file.WriteAt(buf[:n], currentOffset)
			if writeErr != nil {
				w.task.Lock()
				w.task.Chunks[w.chunkIndex].Status = "error"
				w.task.Unlock()
				return fmt.Errorf("failed to write chunk at offset %d: %w", currentOffset, writeErr)
			}

			currentOffset += int64(n)

			w.task.Lock()
			w.task.Chunks[w.chunkIndex].Downloaded += int64(n)
			w.task.Downloaded += int64(n)
			w.task.UpdateProgress()
			w.task.Unlock()

			// High-precision global rate limiting
			if w.rateLimiter != nil {
				w.rateLimiter.Wait(ctx, n)
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			w.task.Lock()
			w.task.Chunks[w.chunkIndex].Status = "error"
			w.task.Unlock()
			return readErr
		}
	}

	w.task.Lock()
	w.task.Chunks[w.chunkIndex].Status = "completed"
	w.task.Unlock()
	return nil
}
