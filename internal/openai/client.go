// Package openai is a thin wrapper over the official SDK exposing exactly
// the three calls the pipeline needs: text, speech, and images.
package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/responses"
	"github.com/openai/openai-go/v2/shared"

	"github.com/thesimpledev/autobotgo/internal/config"
)

// SpeechMaxChars is the API's hard cap on the TTS input field.
const SpeechMaxChars = 4096

// safetyCodes are the API error codes and types that mean "this prompt was
// refused on content grounds" rather than "this request was malformed".
var safetyCodes = map[string]bool{
	"moderation_blocked":       true,
	"content_policy_violation": true,
}

// IsSafetyRejection reports whether err is the image API refusing a prompt
// on content-policy grounds, which callers retry with a rewritten prompt
// instead of failing the run.
//
// The typed code is checked first. The message substring match is a
// fallback: the API has returned policy refusals under codes this list
// does not know about, and treating one as a hard failure would kill an
// otherwise healthy run over a single image.
func IsSafetyRejection(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		if safetyCodes[apiErr.Code] || safetyCodes[apiErr.Type] {
			return true
		}
		// Only 400-class refusals are content decisions. A 429 or 500
		// mentioning "rejected" is a transport problem, not a prompt one.
		if apiErr.StatusCode != http.StatusBadRequest {
			return false
		}
		return mentionsSafety(apiErr.Message)
	}
	return mentionsSafety(err.Error())
}

func mentionsSafety(msg string) bool {
	msg = strings.ToLower(msg)
	for _, s := range []string{"safety", "content_policy", "content policy", "moderation", "rejected"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

type Client struct {
	c   sdk.Client
	cfg *config.OpenAI
	log *slog.Logger
}

func New(cfg *config.OpenAI, log *slog.Logger) *Client {
	return &Client{
		c:   sdk.NewClient(option.WithAPIKey(cfg.APIKey), option.WithMaxRetries(4)),
		cfg: cfg,
		log: log,
	}
}

// Text runs one chat completion and returns the assistant's text.
func (c *Client) Text(ctx context.Context, system, user string) (string, error) {
	resp, err := c.c.Chat.Completions.New(ctx, sdk.ChatCompletionNewParams{
		Model: shared.ChatModel(c.cfg.TextModel),
		Messages: []sdk.ChatCompletionMessageParamUnion{
			sdk.SystemMessage(system),
			sdk.UserMessage(user),
		},
	})
	if err != nil {
		return "", fmt.Errorf("chat completion (%s): %w", c.cfg.TextModel, err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("chat completion (%s): empty response", c.cfg.TextModel)
	}
	out := strings.TrimSpace(resp.Choices[0].Message.Content)
	if out == "" {
		return "", fmt.Errorf("chat completion (%s): empty message content", c.cfg.TextModel)
	}
	return out, nil
}

// Generate runs one generation through the Responses API on an explicit
// model, required for reasoning-tier models, which reject the Chat
// Completions endpoint. A non-empty vectorStoreID adds the file_search
// tool so the model can pull passages from the story archive.
func (c *Client) Generate(ctx context.Context, model, system, user, vectorStoreID string) (string, error) {
	params := responses.ResponseNewParams{
		Model:        shared.ResponsesModel(model),
		Instructions: sdk.String(system),
		Input:        responses.ResponseNewParamsInputUnion{OfString: sdk.String(user)},
	}
	if vectorStoreID != "" {
		params.Tools = []responses.ToolUnionParam{responses.ToolParamOfFileSearch([]string{vectorStoreID})}
	}
	resp, err := c.c.Responses.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("responses (%s): %w", model, err)
	}
	out := strings.TrimSpace(resp.OutputText())
	if out == "" {
		return "", fmt.Errorf("responses (%s): empty output", model)
	}
	return out, nil
}

// CreateVectorStore makes the story-archive vector store; called once,
// the id is then saved into config.json.
func (c *Client) CreateVectorStore(ctx context.Context, name string) (string, error) {
	vs, err := c.c.VectorStores.New(ctx, sdk.VectorStoreNewParams{Name: sdk.String(name)})
	if err != nil {
		return "", fmt.Errorf("creating vector store: %w", err)
	}
	return vs.ID, nil
}

// UploadToVectorStore uploads one story file and attaches it to the
// vector store, waiting until it is indexed.
//
// The polling is hand-rolled: the SDK's NewAndPoll (v2.7.1) passes the
// file and store IDs to its own Get in the wrong order and 400s.
func (c *Client) UploadToVectorStore(ctx context.Context, vectorStoreID, filename string, content []byte) (string, error) {
	f, err := c.c.Files.New(ctx, sdk.FileNewParams{
		File:    sdk.File(bytes.NewReader(content), filename, "text/markdown"),
		Purpose: sdk.FilePurposeAssistants,
	})
	if err != nil {
		return "", fmt.Errorf("uploading %s: %w", filename, err)
	}
	if _, err := c.c.VectorStores.Files.New(ctx, vectorStoreID,
		sdk.VectorStoreFileNewParams{FileID: f.ID}); err != nil {
		return "", fmt.Errorf("attaching %s to vector store: %w", filename, err)
	}
	for {
		vf, err := c.c.VectorStores.Files.Get(ctx, vectorStoreID, f.ID)
		if err != nil {
			return "", fmt.Errorf("checking indexing of %s: %w", filename, err)
		}
		switch vf.Status {
		case sdk.VectorStoreFileStatusCompleted:
			return f.ID, nil
		case sdk.VectorStoreFileStatusFailed, sdk.VectorStoreFileStatusCancelled:
			return "", fmt.Errorf("indexing %s in vector store: status %s", filename, vf.Status)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// ExtractJSON pulls the first top-level JSON object out of a model reply,
// tolerating markdown fences and prose around it.
func ExtractJSON(s string) (string, error) {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end <= start {
		return "", fmt.Errorf("no JSON object found in model reply: %.200s", s)
	}
	return s[start : end+1], nil
}

// Speech synthesizes one chunk of text (<= SpeechMaxChars) to WAV bytes.
func (c *Client) Speech(ctx context.Context, text string) ([]byte, error) {
	if len(text) > SpeechMaxChars {
		return nil, fmt.Errorf("speech input is %d chars, max is %d (chunker bug)", len(text), SpeechMaxChars)
	}
	params := sdk.AudioSpeechNewParams{
		Model:          c.cfg.TTSModel,
		Voice:          sdk.AudioSpeechNewParamsVoice(c.cfg.TTSVoice),
		Input:          text,
		ResponseFormat: sdk.AudioSpeechNewParamsResponseFormatWAV,
	}
	if c.cfg.TTSInstructions != "" {
		params.Instructions = sdk.String(c.cfg.TTSInstructions)
	}
	resp, err := c.c.Audio.Speech.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("tts (%s): %w", c.cfg.TTSModel, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tts (%s): reading audio: %w", c.cfg.TTSModel, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("tts (%s): empty audio response", c.cfg.TTSModel)
	}
	return data, nil
}

// Image generates one image and returns the PNG bytes.
func (c *Client) Image(ctx context.Context, prompt string) ([]byte, error) {
	resp, err := c.c.Images.Generate(ctx, sdk.ImageGenerateParams{
		Model:   sdk.ImageModel(c.cfg.ImageModel),
		Prompt:  prompt,
		Size:    sdk.ImageGenerateParamsSize(c.cfg.ImageSize),
		Quality: sdk.ImageGenerateParamsQuality(c.cfg.ImageQuality),
	})
	if err != nil {
		return nil, fmt.Errorf("image (%s): %w", c.cfg.ImageModel, err)
	}
	if len(resp.Data) == 0 || resp.Data[0].B64JSON == "" {
		return nil, fmt.Errorf("image (%s): response had no b64 image data", c.cfg.ImageModel)
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("image (%s): decoding b64: %w", c.cfg.ImageModel, err)
	}
	return data, nil
}
