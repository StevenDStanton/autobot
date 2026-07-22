package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/thesimpledev/autobotgo/internal/ffmpeg"
	"github.com/thesimpledev/autobotgo/internal/openai"
	"github.com/thesimpledev/autobotgo/internal/run"
)

// Images generates the visuals into images/<date>/. One text-model call
// derives all N scene prompts from the story (cheaper and more visually
// coherent than ad-hoc per-image prompting), then each prompt becomes
// one image.
var Images = run.Stage{
	Name:    NameImages,
	Outputs: []run.PathFn{runFile(FileImagesDone)},
	Timeout: 30 * time.Minute,
	Fn:      imagesFn,
}

const scenePromptSystem = `You design image prompts for illustrated story videos.
Given a story, produce exactly N scene descriptions that visually follow the story's arc in order.
Rules:
- Each description is one self-contained prompt for an image model: setting, subject, mood, lighting.
- Keep the setting visually consistent across all scenes: repeat the same concrete
  descriptions in every prompt (image generations are independent and share no memory).
- Favor unpeopled compositions: empty rooms, exteriors, doorways, objects, and the traces
  events leave behind. If a person appears, an ordinary fully-clothed adult, distant or
  partly obscured.
- NEVER depict children or minors, even when the story includes them.
- NEVER depict bathing, bedrooms, sleepwear, undressing, or any intimate or bodily context,
  however innocent, automated safety filters reject them and the whole video fails.
- No text, captions, or lettering in the images.
Return ONLY a JSON object: {"scenes": ["...", "..."]}`

// sanitizeSystem rewrites a single rejected prompt. Environment-only is
// the reliable escape hatch: mood without any person in frame.
const sanitizeSystem = `This image prompt was rejected by an automated image-safety system.
Rewrite it so it cannot possibly be rejected while keeping the same setting, mood, and lighting:
remove ALL people of any age and any bodily, bathing, bedroom, or intimate context. Describe only
the environment, objects, weather, lighting, and the traces of what happened.
Return ONLY the rewritten prompt text.`

type imagePrompts struct {
	Scenes []string `json:"scenes"`
}

func imagesFn(ctx context.Context, r *run.Run) error {
	story, err := os.ReadFile(PathStory(r))
	if err != nil {
		return err
	}
	dur, err := ffmpeg.ProbeDuration(ctx, PathNarration(r))
	if err != nil {
		return err
	}
	n := imageCount(r.Cfg.Video.ImagesPerMinute, dur)
	client := openai.New(&r.Cfg.OpenAI, r.Log)

	// Scene prompts (kept as an artifact for debugging).
	var prompts imagePrompts
	if data, err := os.ReadFile(r.Path(FileImagePrompts)); err == nil && json.Unmarshal(data, &prompts) == nil && len(prompts.Scenes) == n {
		r.Log.Info("scene prompts exist, keeping", "count", n)
	} else {
		r.Log.Info("deriving scene prompts", "count", n)
		user := fmt.Sprintf("N = %d\n\nStory:\n\n%s", n, story)
		reply, err := client.Text(ctx, scenePromptSystem, user)
		if err != nil {
			return err
		}
		jsonPart, err := openai.ExtractJSON(reply)
		if err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(jsonPart), &prompts); err != nil {
			return fmt.Errorf("parsing scene prompts: %w", err)
		}
		if len(prompts.Scenes) != n {
			return fmt.Errorf("asked for %d scene prompts, got %d", n, len(prompts.Scenes))
		}
		data, err := json.MarshalIndent(prompts, "", "  ")
		if err != nil {
			return err
		}
		if err := r.WriteFile(FileImagePrompts, data); err != nil {
			return err
		}
	}

	for i, scene := range prompts.Scenes {
		out := filepath.Join(DirImages(r), imageName(i))
		// Per-image resume: don't re-buy images that already exist.
		if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
			r.Log.Info("image exists, keeping", "image", i+1, "of", n)
			continue
		}
		style := r.Cfg.OpenAI.ImageStylePrompt
		withStyle := func(s string) string {
			if style == "" {
				return s
			}
			return s + "\n\nStyle: " + style
		}
		r.Log.Info("generating image", "image", i+1, "of", n)
		png, err := client.Image(ctx, withStyle(scene))
		if err != nil && openai.IsSafetyRejection(err) {
			// A false positive here would otherwise kill the whole run;
			// retry with an unpeopled rewrite of the same scene.
			r.Log.Warn("image rejected by safety system, retrying with sanitized prompt", "image", i+1, "error", err)
			cleaned, serr := client.Text(ctx, sanitizeSystem, scene)
			if serr != nil {
				return fmt.Errorf("image %d/%d: sanitizing rejected prompt: %w", i+1, n, serr)
			}
			png, err = client.Image(ctx, withStyle(cleaned))
		}
		if err != nil {
			return fmt.Errorf("image %d/%d: %w", i+1, n, err)
		}
		if err := writeArtifact(out, png); err != nil {
			return err
		}
	}
	return writeImagesDone(r, n)
}

// writeImagesDone records the generated image list; it doubles as the
// stage's completion marker, so it must be the last thing written.
func writeImagesDone(r *run.Run, n int) error {
	files := make([]string, n)
	for i := range files {
		files[i] = imageName(i)
	}
	data, err := json.MarshalIndent(map[string]any{"count": n, "files": files}, "", "  ")
	if err != nil {
		return err
	}
	return r.WriteFile(FileImagesDone, data)
}
