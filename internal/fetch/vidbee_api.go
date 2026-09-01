package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// resolveViaVidBee queues a yt-dlp download on a VidBee API sidecar and waits for
// the file on disk (NIMBUS_VIDBEE_DOWNLOAD_DIR must match VidBee's download folder).
func resolveViaVidBee(ctx context.Context, pageURL string, opt Options) (Media, error) {
	base := vidbeeAPIBase()
	if base == "" {
		return Media{}, Err{Code: CodeLinkUnsupported, Message: "vidbee api not configured"}
	}
	dlRoot := vidbeeDownloadDir()
	if dlRoot == "" {
		return Media{}, Err{
			Code:    CodeFetchFail,
			Message: "set NIMBUS_VIDBEE_DOWNLOAD_DIR to VidBee's download folder",
		}
	}

	mode := strings.ToLower(strings.TrimSpace(opt.Mode))
	if mode == "" {
		mode = "video"
	}
	if mode != "video" && mode != "audio" {
		mode = "video"
	}

	input := map[string]any{
		"url":  pageURL,
		"type": mode,
	}
	if format := vidbeeFormat(opt.MaxHeight, mode); format != "" {
		input["format"] = format
	}
	if cookies := strings.TrimSpace(opt.Cookies); cookies != "" {
		input["settings"] = map[string]any{"cookiesPath": cookies}
	}

	createdOut, err := vidbeeRPC(ctx, base, "downloads/create", input)
	if err != nil {
		return Media{}, Err{Code: CodeFetchFail, Message: err.Error()}
	}
	created, _ := createdOut["download"].(map[string]any)
	if created == nil {
		return Media{}, Err{Code: CodeFetchFail, Message: "vidbee returned no download task"}
	}
	taskID, _ := created["id"].(string)
	if taskID == "" {
		return Media{}, Err{Code: CodeFetchFail, Message: "vidbee returned no task id"}
	}

	task, err := vidbeeWaitTask(ctx, base, taskID, opt.OnProgress)
	if err != nil {
		return Media{}, err
	}

	localPath, err := vidbeeTaskPath(task, dlRoot)
	if err != nil {
		return Media{}, err
	}
	name := filepath.Base(localPath)
	if saved, _ := task["savedFileName"].(string); saved != "" {
		name = filepath.Base(saved)
	}
	return Media{
		LocalPath: localPath,
		Filename:  name,
		Service:   "vidbee",
	}, nil
}

func vidbeeAPIConfigured() bool {
	return vidbeeAPIBase() != ""
}

func vidbeeAPIBase() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("NIMBUS_VIDBEE_API")), "/")
}

func vidbeeDownloadDir() string {
	if p := strings.TrimSpace(os.Getenv("NIMBUS_VIDBEE_DOWNLOAD_DIR")); p != "" {
		return filepath.Clean(p)
	}
	return ""
}

func vidbeeFormat(maxHeight int, mode string) string {
	if mode == "audio" {
		return "bestaudio/best"
	}
	h := SnapHeight(maxHeight)
	if h <= 0 {
		return ""
	}
	return fmt.Sprintf("bv*[height<=%d]+ba/b[height<=%d]/best", h, h)
}

func vidbeeRPC(ctx context.Context, base, procedure string, input map[string]any) (map[string]any, error) {
	body, _ := json.Marshal(map[string]any{"json": input})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/rpc/"+procedure, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("vidbee http %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}

	var envelope struct {
		JSON json.RawMessage `json:"json"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("bad vidbee response")
	}
	if len(envelope.JSON) == 0 {
		return nil, fmt.Errorf("empty vidbee response")
	}

	var out map[string]any
	if err := json.Unmarshal(envelope.JSON, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func vidbeeWaitTask(ctx context.Context, base, taskID string, onProgress ProgressFunc) (map[string]any, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		if task, ok, err := vidbeeFindTask(ctx, base, "downloads/list", taskID); err != nil {
			return nil, Err{Code: CodeFetchFail, Message: err.Error()}
		} else if ok {
			if onProgress != nil {
				vidbeeReportProgress(task, onProgress, false)
			}
			if status, _ := task["status"].(string); status == "error" || status == "cancelled" {
				msg, _ := task["error"].(string)
				if msg == "" {
					msg = "vidbee download failed"
				}
				return nil, Err{Code: CodeFetchFail, Message: msg}
			}
		}

		if task, ok, err := vidbeeFindTask(ctx, base, "history/list", taskID); err != nil {
			return nil, Err{Code: CodeFetchFail, Message: err.Error()}
		} else if ok {
			status, _ := task["status"].(string)
			switch status {
			case "completed":
				if onProgress != nil {
					onProgress(100, "VidBee download finished")
				}
				return task, nil
			case "error", "cancelled":
				msg, _ := task["error"].(string)
				if msg == "" {
					msg = "vidbee download failed"
				}
				return nil, Err{Code: CodeFetchFail, Message: msg}
			}
		}

		select {
		case <-ctx.Done():
			return nil, Err{Code: CodeFetchFail, Message: "vidbee download cancelled"}
		case <-ticker.C:
		}
	}
}

func vidbeeFindTask(ctx context.Context, base, procedure, taskID string) (map[string]any, bool, error) {
	out, err := vidbeeRPC(ctx, base, procedure, map[string]any{})
	if err != nil {
		return nil, false, err
	}
	key := "downloads"
	if procedure == "history/list" {
		key = "history"
	}
	items, _ := out[key].([]any)
	for _, item := range items {
		task, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := task["id"].(string); id == taskID {
			return task, true, nil
		}
	}
	return nil, false, nil
}

func vidbeeReportProgress(task map[string]any, onProgress ProgressFunc, done bool) {
	if done {
		onProgress(100, "VidBee download finished")
		return
	}
	progress, _ := task["progress"].(map[string]any)
	if progress == nil {
		onProgress(-1, "Downloading via VidBee…")
		return
	}
	pct, _ := progress["percent"].(float64)
	msg := "Downloading via VidBee…"
	if speed, _ := progress["currentSpeed"].(string); speed != "" {
		msg = "Downloading via VidBee… " + speed
	}
	if pct > 0 {
		onProgress(pct, msg)
	} else {
		onProgress(-1, msg)
	}
}

func vidbeeTaskPath(task map[string]any, dlRoot string) (string, error) {
	dir, _ := task["downloadPath"].(string)
	name, _ := task["savedFileName"].(string)
	candidates := []string{}
	if dir != "" && name != "" {
		candidates = append(candidates, filepath.Join(dir, name))
	}
	if name != "" {
		candidates = append(candidates, filepath.Join(dlRoot, name))
	}
	if dir != "" {
		candidates = append(candidates, dir)
	}
	for _, p := range candidates {
		p = filepath.Clean(p)
		st, err := os.Stat(p)
		if err == nil && !st.IsDir() && st.Size() > 1000 {
			return p, nil
		}
	}
	return "", Err{Code: CodeFetchEmpty, Message: "vidbee file missing on disk (check NIMBUS_VIDBEE_DOWNLOAD_DIR)"}
}
