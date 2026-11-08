package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrRangeNotSupported is returned when the server returns 200 OK instead of 206 Partial Content for a chunk range request.
var ErrRangeNotSupported = errors.New("server does not support range requests (returned 200 OK)")

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

	if resp.StatusCode == http.StatusOK {
		// 200 OK means the server ignored the Range header and is returning the entire file from byte 0.
		// If this is a multi-part download or chunk begins past byte 0, this chunk cannot be processed safely.
		if len(w.task.Chunks) > 1 || rangeStart > 0 {
			w.task.Lock()
			w.task.Chunks[w.chunkIndex].Status = "error"
			w.task.Unlock()
			return ErrRangeNotSupported
		}
	} else if resp.StatusCode != http.StatusPartialContent {
		w.task.Lock()
		w.task.Chunks[w.chunkIndex].Status = "error"
		w.task.Unlock()
		return fmt.Errorf("unexpected status code %d for chunk %d", resp.StatusCode, w.chunkIndex)
	}

	buf := make([]byte, 32*1024) // 32KB read buffer for smoother rate limiting
	currentOffset := rangeStart
	bytesRemaining := chunk.End - currentOffset + 1

	for {
		select {
		case <-ctx.Done():
			w.task.Lock()
			w.task.Chunks[w.chunkIndex].Status = "pending"
			w.task.Unlock()
			return ctx.Err()
		default:
		}

		if bytesRemaining <= 0 {
			break
		}

		toRead := len(buf)
		if int64(toRead) > bytesRemaining {
			toRead = int(bytesRemaining)
		}

		n, readErr := resp.Body.Read(buf[:toRead])
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
			bytesRemaining -= int64(n)

			w.task.Lock()
			w.task.Chunks[w.chunkIndex].Downloaded += int64(n)
			if w.task.Chunks[w.chunkIndex].Downloaded > w.task.Chunks[w.chunkIndex].Total {
				w.task.Chunks[w.chunkIndex].Downloaded = w.task.Chunks[w.chunkIndex].Total
			}

			// Sum all chunks for task.Downloaded to guarantee accuracy and prevent exceeding total size
			var totalDownloaded int64
			for _, c := range w.task.Chunks {
				totalDownloaded += c.Downloaded
			}
			w.task.Downloaded = totalDownloaded
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
