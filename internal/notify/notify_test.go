package notify

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSend(t *testing.T) {
	t.Run("posts the message as plain text", func(t *testing.T) {
		var (
			gotMethod string
			gotBody   string
			gotType   string
			calls     int
		)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			gotMethod = r.Method
			gotType = r.Header.Get("Content-Type")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
		}))
		defer srv.Close()

		Send(discardLogger(), srv.URL, "hello there")

		if calls != 1 {
			t.Fatalf("server received %d requests, want 1", calls)
		}
		if gotMethod != http.MethodPost {
			t.Errorf("method = %s, want POST", gotMethod)
		}
		if gotBody != "hello there" {
			t.Errorf("body = %q, want %q", gotBody, "hello there")
		}
		if !strings.HasPrefix(gotType, "text/plain") {
			t.Errorf("Content-Type = %q, want text/plain", gotType)
		}
	})

	t.Run("empty url sends nothing", func(t *testing.T) {
		// No server involved: the point is that Send returns without
		// attempting a request, and without panicking.
		Send(discardLogger(), "", "ignored")
	})

	// Notifications are best-effort. A broken webhook must never take the
	// pipeline down with it, so each of these has to return normally.
	t.Run("server error is swallowed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusInternalServerError)
		}))
		defer srv.Close()
		Send(discardLogger(), srv.URL, "msg")
	})

	t.Run("unreachable host is swallowed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close() // nothing is listening now
		Send(discardLogger(), url, "msg")
	})

	t.Run("malformed url is swallowed", func(t *testing.T) {
		Send(discardLogger(), "://not a url", "msg")
	})
}

func TestSuccess(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		videoURL string
		want     string
	}{
		{
			name:     "with url",
			title:    "The Lighthouse",
			videoURL: "https://youtu.be/abc123",
			want:     `autobotgo OK: "The Lighthouse" https://youtu.be/abc123`,
		},
		{
			name:  "without url",
			title: "The Lighthouse",
			want:  `autobotgo OK: "The Lighthouse"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Success(tc.title, tc.videoURL); got != tc.want {
				t.Errorf("Success() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFailure(t *testing.T) {
	got := Failure("images", errors.New("safety rejection"))
	want := "autobotgo FAILED at stage images: safety rejection"
	if got != want {
		t.Errorf("Failure() = %q, want %q", got, want)
	}
}
