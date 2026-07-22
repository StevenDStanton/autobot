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

// World maintains the shared story world after each new story:
//
//  1. world.md, the capped canon file every future story is prompted
//     with. Updated with today's story; when it grows past
//     world.max_words it is compressed by the model (core canon kept,
//     episode detail dropped) to ~70% of the cap.
//  2. the vector store. When RAG is enabled, today's story is uploaded
//     so future stories can search the full archive. The store is
//     created on first use and its id saved back into config.json.
var World = run.Stage{
	Name:    NameWorld,
	Outputs: []run.PathFn{runFile(FileWorld)},
	Timeout: 10 * time.Minute,
	Fn:      worldFn,
}

const worldUpdateSystem = `You maintain the WORLD BIBLE for an ongoing series of short stories
that share one world. Given the current world bible (possibly empty) and today's new story,
return the updated world bible in full.
Rules:
- Markdown with these sections: Characters, Places, Objects & Lore, Timeline, Open Threads.
- Record durable facts: names, relationships, fates, distinctive details, unresolved hooks.
- Every Timeline entry is one line recording the broadcast's date, SETTING, and PHENOMENON
  (e.g. "2026-07-10: apartment building entry panel, familiar-voice mimic"). Future stories
  are audited against this list to prevent repeats, so keep it complete.
- Merge today's story into the existing entries; never contradict established canon.
- Be concise. This file is a reference, not prose. Do not retell stories.
- Return ONLY the world bible markdown, nothing else.`

const worldCompressSystem = `You maintain the WORLD BIBLE for an ongoing story series. It has grown
too long. Rewrite it to at most %d words while keeping what matters most:
every named character still alive in the story world, major places, unresolved open threads,
and hard canonical facts. Collapse minor detail, merge redundant entries, drop
episode-by-episode notes. Keep the same section structure.
Return ONLY the compressed world bible markdown, nothing else.`

// WorldResult is the stage's bookkeeping marker.
type WorldResult struct {
	WorldWords    int    `json:"world_words"`
	Compressed    bool   `json:"compressed"`
	VectorStoreID string `json:"vector_store_id,omitempty"`
	StoryFileID   string `json:"story_file_id,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

func worldFn(ctx context.Context, r *run.Run) error {
	story, err := os.ReadFile(PathStory(r))
	if err != nil {
		return err
	}
	client := openai.New(&r.Cfg.OpenAI, r.Log)
	res := WorldResult{UpdatedAt: time.Now().UTC().Format(time.RFC3339)}

	// 1. Update the canon file.
	current, _ := os.ReadFile(PathWorld(r)) // absent on the very first run
	user := fmt.Sprintf("CURRENT WORLD BIBLE:\n\n%s\n\n--- TODAY'S NEW STORY ---\n\n%s",
		strings.TrimSpace(string(current)), story)
	updated, err := client.Text(ctx, worldUpdateSystem, user)
	if err != nil {
		return fmt.Errorf("updating world bible: %w", err)
	}

	if words := len(strings.Fields(updated)); words > r.Cfg.World.MaxWords {
		// Compress to ~70% of the cap so this doesn't re-trigger daily.
		target := r.Cfg.World.MaxWords * 7 / 10
		r.Log.Info("world bible over cap, compressing", "words", words, "cap", r.Cfg.World.MaxWords, "target", target)
		compressed, err := client.Text(ctx, fmt.Sprintf(worldCompressSystem, target), updated)
		if err != nil {
			return fmt.Errorf("compressing world bible: %w", err)
		}
		updated = compressed
		res.Compressed = true
	}
	res.WorldWords = len(strings.Fields(updated))
	if err := writeArtifact(PathWorld(r), []byte(updated)); err != nil {
		return err
	}
	r.Log.Info("world bible updated", "words", res.WorldWords, "compressed", res.Compressed)

	// 2. Archive the story in the vector store.
	if r.Cfg.World.RAGEnabled {
		vsID := r.Cfg.World.VectorStoreID
		if vsID == "" {
			vsID, err = client.CreateVectorStore(ctx, "autobotgo-stories")
			if err != nil {
				return err
			}
			r.Cfg.World.VectorStoreID = vsID
			if err := r.Cfg.Save(r.Cfg.ConfigPath); err != nil {
				return fmt.Errorf("saving vector store id to config: %w", err)
			}
			r.Log.Info("created story vector store", "id", vsID)
		}
		fileID, err := client.UploadToVectorStore(ctx, vsID, r.Date+".md", story)
		if err != nil {
			return err
		}
		res.VectorStoreID = vsID
		res.StoryFileID = fileID
		r.Log.Info("story archived to vector store", "file_id", fileID)
	}

	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return r.WriteFile(FileWorld, data)
}
