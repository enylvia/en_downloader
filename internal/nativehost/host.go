package nativehost

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

type NativeMessageIn struct {
	Action   string            `json:"action"` // "ping", "download"
	URL      string            `json:"url"`
	Filename string            `json:"filename"`
	Referrer string            `json:"referrer"`
	Cookies  string            `json:"cookies"`
	Headers  map[string]string `json:"headers"`
}

type NativeMessageOut struct {
	Status  string `json:"status"` // "ok", "error"
	Message string `json:"message,omitempty"`
	TaskID  string `json:"task_id,omitempty"`
}

func RunNativeHost(port int) error {
	client := &http.Client{Timeout: 10 * time.Second}
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/api/tasks/add", port)

	for {
		var length uint32
		if err := binary.Read(os.Stdin, binary.LittleEndian, &length); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if length == 0 || length > 10*1024*1024 { // max 10MB
			return fmt.Errorf("invalid message length: %d", length)
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(os.Stdin, payload); err != nil {
			return err
		}

		var msg NativeMessageIn
		if err := json.Unmarshal(payload, &msg); err != nil {
			sendResponse(NativeMessageOut{Status: "error", Message: "invalid json payload"})
			continue
		}

		switch msg.Action {
		case "ping":
			sendResponse(NativeMessageOut{Status: "ok", Message: "EnylviaDownloader Native Host is active"})
		case "download":
			if msg.URL == "" {
				sendResponse(NativeMessageOut{Status: "error", Message: "url is required"})
				continue
			}

			headers := msg.Headers
			if headers == nil {
				headers = make(map[string]string)
			}
			if msg.Referrer != "" {
				headers["Referer"] = msg.Referrer
			}
			if msg.Cookies != "" {
				headers["Cookie"] = msg.Cookies
			}

			reqBody, _ := json.Marshal(map[string]any{
				"url":        msg.URL,
				"filename":   msg.Filename,
				"headers":    headers,
				"auto_start": true,
			})

			resp, err := client.Post(apiURL, "application/json", bytes.NewReader(reqBody))
			if err != nil {
				// Server might be offline. Launch main application and retry
				exePath, _ := os.Executable()
				if exePath != "" {
					cmd := exec.Command(exePath)
					_ = cmd.Start()
					time.Sleep(1500 * time.Millisecond)
					resp, err = client.Post(apiURL, "application/json", bytes.NewReader(reqBody))
				}
			}

			if err != nil {
				sendResponse(NativeMessageOut{
					Status:  "error",
					Message: fmt.Sprintf("failed to contact EnylviaDownloader server: %v", err),
				})
				continue
			}

			var addResp struct {
				Task struct {
					ID string `json:"id"`
				} `json:"task"`
				Error string `json:"error"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&addResp)
			resp.Body.Close()

			if addResp.Error != "" {
				sendResponse(NativeMessageOut{Status: "error", Message: addResp.Error})
			} else {
				sendResponse(NativeMessageOut{Status: "ok", Message: "download added successfully", TaskID: addResp.Task.ID})
			}
		default:
			sendResponse(NativeMessageOut{Status: "error", Message: fmt.Sprintf("unknown action '%s'", msg.Action)})
		}
	}
}

func sendResponse(resp NativeMessageOut) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	length := uint32(len(data))
	_ = binary.Write(os.Stdout, binary.LittleEndian, length)
	_, _ = os.Stdout.Write(data)
}
