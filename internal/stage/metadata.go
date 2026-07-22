package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/thesimpledev/autobotgo/internal/openai"
	"github.com/thesimpledev/autobotgo/internal/run"
)

// Metadata derives the YouTube title, description, and tags from the story.
var Metadata = run.Stage{
	Name:    NameMetadata,
	Outputs: []run.PathFn{runFile(FileMetadata)},
	Timeout: 5 * time.Minute,
	Fn:      metadataFn,
}

const metadataSystem = `You write YouTube metadata for narrated story videos.
Given the story, return ONLY a JSON object:
{"title": "...", "description": "...", "tags": ["...", "..."]}
Rules:
- title: under 90 characters, engaging but honest, no clickbait, no quotes around it.
- description: 2-4 sentences summarizing the story without spoiling the ending,
  then a blank line. Do not mention AI; a disclosure line is appended automatically.
- tags: 8-15 short lowercase topic tags.`

// aiDisclosure is always appended, since we never trust the model to disclose itself.
const aiDisclosure = "This story, narration, and imagery were generated with AI."

func metadataFn(ctx context.Context, r *run.Run) error {
	story, err := os.ReadFile(PathStory(r))
	if err != nil {
		return err
	}
	client := openai.New(&r.Cfg.OpenAI, r.Log)
	reply, err := client.Text(ctx, metadataSystem, string(story))
	if err != nil {
		return err
	}
	jsonPart, err := openai.ExtractJSON(reply)
	if err != nil {
		return err
	}
	var md VideoMetadata
	if err := json.Unmarshal([]byte(jsonPart), &md); err != nil {
		return fmt.Errorf("parsing metadata: %w", err)
	}
	if md.Title == "" {
		return fmt.Errorf("metadata came back without a title")
	}
	if r := []rune(md.Title); len(r) > 100 {
		md.Title = string(r[:97]) + "..."
	}
	md.Description = strings.TrimSpace(md.Description) + "\n\n" + aiDisclosure
	md.Tags = append(md.Tags, r.Cfg.YouTube.ExtraTags...)

	data, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	r.Log.Info("metadata generated", "title", md.Title)
	return r.WriteFile(FileMetadata, data)
}
