// Package notify sends plain-text status pings to a webhook URL.
// The body format works as-is with ntfy.sh topics and generic webhooks.
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Send POSTs msg to url. A notification failure must never fail the
// pipeline, so errors are logged and swallowed.
func Send(log *slog.Logger, url, msg string) {
	if url == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(msg))
	if err != nil {
		log.Warn("notify: building request failed", "error", err)
		return
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Warn("notify: send failed", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Warn("notify: non-success response", "status", resp.Status)
	}
}

// Success formats the all-good message.
func Success(title, videoURL string) string {
	if videoURL == "" {
		return fmt.Sprintf("autobotgo OK: %q", title)
	}
	return fmt.Sprintf("autobotgo OK: %q %s", title, videoURL)
}

// Failure formats the something-broke message.
func Failure(stage string, err error) string {
	return fmt.Sprintf("autobotgo FAILED at stage %s: %v", stage, err)
}
