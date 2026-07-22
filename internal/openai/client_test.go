package openai

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	sdk "github.com/openai/openai-go/v2"
)

// apiError builds an SDK error the way the API surfaces one, so the
// typed path in IsSafetyRejection is exercised rather than the fallback.
func apiError(status int, code, errType, msg string) *sdk.Error {
	return &sdk.Error{
		Code:       code,
		Type:       errType,
		Message:    msg,
		StatusCode: status,
	}
}

func TestIsSafetyRejection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil is not a rejection",
			err:  nil,
		},
		{
			name: "typed moderation_blocked code",
			err:  apiError(http.StatusBadRequest, "moderation_blocked", "invalid_request_error", "blocked"),
			want: true,
		},
		{
			name: "typed content_policy_violation code",
			err:  apiError(http.StatusBadRequest, "content_policy_violation", "invalid_request_error", "no"),
			want: true,
		},
		{
			name: "unknown code but the message says safety",
			err:  apiError(http.StatusBadRequest, "some_new_code", "invalid_request_error", "Rejected by our safety system"),
			want: true,
		},
		{
			name: "unknown code with an unrelated message",
			err:  apiError(http.StatusBadRequest, "invalid_size", "invalid_request_error", "size must be 1024x1024"),
		},
		{
			// A 500 that happens to say "rejected" is a transport problem.
			// Retrying it with a sanitized prompt would hide a real outage.
			name: "server error mentioning rejected is not a content decision",
			err:  apiError(http.StatusInternalServerError, "server_error", "server_error", "request rejected upstream"),
		},
		{
			name: "rate limit is not a content decision",
			err:  apiError(http.StatusTooManyRequests, "rate_limit_exceeded", "rate_limit_error", "slow down"),
		},
		{
			name: "untyped error mentioning content policy",
			err:  errors.New("image generation failed: content_policy violation"),
			want: true,
		},
		{
			name: "untyped unrelated error",
			err:  errors.New("connection reset by peer"),
		},
		{
			name: "wrapped typed rejection is still found",
			err: fmt.Errorf("image 3/8: %w",
				apiError(http.StatusBadRequest, "moderation_blocked", "invalid_request_error", "blocked")),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSafetyRejection(tc.err); got != tc.want {
				t.Errorf("IsSafetyRejection(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "bare object",
			in:   `{"a":1}`,
			want: `{"a":1}`,
		},
		{
			name: "fenced in markdown",
			in:   "```json\n{\"a\":1}\n```",
			want: `{"a":1}`,
		},
		{
			name: "surrounded by prose",
			in:   `Sure! Here you go: {"a":1} Let me know if you need changes.`,
			want: `{"a":1}`,
		},
		{
			name: "nested braces keep the outermost object",
			in:   `prefix {"a":{"b":2}} suffix`,
			want: `{"a":{"b":2}}`,
		},
		{
			name:    "no object at all",
			in:      "I cannot do that.",
			wantErr: true,
		},
		{
			name:    "closing brace before opening",
			in:      "} {",
			wantErr: true,
		},
		{
			name:    "empty input",
			in:      "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractJSON(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ExtractJSON(%q) = %q, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExtractJSON(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ExtractJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
