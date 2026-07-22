package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/thesimpledev/autobotgo/internal/ffmpeg"
	"github.com/thesimpledev/autobotgo/internal/run"
)

// Pipeline returns the ordered stages. In dry-run mode every stage that
// would hit the network is swapped for a fake-artifact generator; the
// video stage always runs for real, since exercising ffmpeg cheaply is
// the whole point of --dry-run. The dry cleanup also keeps the media on
// disk so it can be inspected.
func Pipeline(dryRun bool) []run.Stage {
	stages := []run.Stage{Story, World, TTS, Images, Video, Metadata, Upload, Archive, Cleanup}
	if !dryRun {
		return stages
	}
	dry := map[string]func(context.Context, *run.Run) error{
		NameStory:    dryStory,
		NameWorld:    dryWorld,
		NameTTS:      dryTTS,
		NameImages:   dryImages,
		NameMetadata: dryMetadata,
		NameUpload:   dryUpload,
		NameArchive:  dryArchive,
		NameCleanup:  dryCleanup,
	}
	for i, s := range stages {
		if fn, ok := dry[s.Name]; ok {
			stages[i].Fn = fn
		}
	}
	return stages
}

// Names lists the pipeline's stage names in run order, for help text and
// for validating what the user asked for.
func Names() []string {
	stages := Pipeline(false)
	names := make([]string, len(stages))
	for i, s := range stages {
		names[i] = s.Name
	}
	return names
}

// imageCount decides how many images a narration of the given length
// needs. Shared by the real and dry image stages.
func imageCount(imagesPerMinute, narrationSecs float64) int {
	return max(int(math.Ceil(imagesPerMinute*narrationSecs/60)), 1)
}

func imageName(i int) string {
	return fmt.Sprintf("img_%02d.png", i)
}

func dryStory(ctx context.Context, r *run.Run) error {
	paras := []string{
		"The lighthouse keeper counted storms the way other people counted birthdays. Each one left a mark somewhere: a cracked pane, a bent railing, a story he would polish for years until it shone brighter than the lamp itself.",
		"On the morning this story begins, the sea was flat and silver, and that worried him more than any gale. Calm water, his grandmother used to say, is the sea holding its breath.",
		"He climbed the one hundred and twelve steps slowly, coffee in one hand, and looked out at a horizon that had forgotten how to move. Far off, a small boat sat perfectly still, though no anchor line ran from its bow.",
		"By noon he had rowed out to it. The boat was empty except for a brass compass that pointed, unwaveringly, back at his lighthouse. He turned the compass over twice. He turned himself around once. The needle did not care.",
		"That night he lit the lamp early and watched the beam sweep the silver water, and for the first time in thirty years he understood that the light was not a warning to ships. It was an invitation, and something out there had finally accepted.",
	}
	text := strings.Join(paras, "\n\n")
	r.Log.Info("dry-run: wrote canned story", "words", len(strings.Fields(text)))
	return writeArtifact(PathStory(r), []byte(text))
}

// dryWorld leaves the real world.md alone, since a test run must not pollute
// the canon.
func dryWorld(ctx context.Context, r *run.Run) error {
	r.Log.Info("dry-run: skipping world bible update and archive upload")
	return r.WriteFile(FileWorld, []byte(`{"dry_run": true}`+"\n"))
}

// dryTTS synthesizes a sine tone the length of the configured target so
// the video stage exercises the real probe-and-time path.
func dryTTS(ctx context.Context, r *run.Run) error {
	dur := max(r.Cfg.Story.TargetMinutes*60-10, 10)
	r.Log.Info("dry-run: generating placeholder narration", "seconds", dur)
	out := PathNarration(r)
	if err := ensureDirOf(out); err != nil {
		return err
	}
	tmp := out + ".tmp.wav"
	if err := ffmpeg.Run(ctx, r.Log,
		"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=220:duration=%d", dur),
		"-ac", "1", "-ar", "24000", tmp,
	); err != nil {
		return err
	}
	return renameFile(tmp, out)
}

// dryImages writes numbered flat-color frames so ordering and crossfade
// timing are visually checkable in the rendered MP4.
func dryImages(ctx context.Context, r *run.Run) error {
	dur, err := ffmpeg.ProbeDuration(ctx, PathNarration(r))
	if err != nil {
		return err
	}
	n := imageCount(r.Cfg.Video.ImagesPerMinute, dur)
	colors := []string{"0x336699", "0x996633", "0x339966", "0x663399", "0x993344", "0x557722"}
	size := r.Cfg.OpenAI.ImageSize
	if size == "" {
		size = "1536x1024"
	}
	r.Log.Info("dry-run: generating placeholder images", "count", n, "size", size)
	for i := range n {
		out := filepath.Join(DirImages(r), imageName(i))
		if err := ensureDirOf(out); err != nil {
			return err
		}
		tmp := out + ".tmp.png"
		if err := ffmpeg.Run(ctx, r.Log,
			"-f", "lavfi", "-i", fmt.Sprintf("color=c=%s:s=%s", colors[i%len(colors)], size),
			"-frames:v", "1",
			"-vf", fmt.Sprintf("drawtext=text='%d':fontsize=400:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2", i),
			tmp,
		); err != nil {
			return err
		}
		if err := renameFile(tmp, out); err != nil {
			return err
		}
	}
	return writeImagesDone(r, n)
}

func dryMetadata(ctx context.Context, r *run.Run) error {
	md := VideoMetadata{
		Title:       "Dry Run: The Lighthouse Keeper",
		Description: "Placeholder metadata from autobotgo --dry-run.\n\nThis story, narration, and imagery were generated with AI.",
		Tags:        []string{"dry run", "autobotgo"},
	}
	data, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	return r.WriteFile(FileMetadata, data)
}

func dryUpload(ctx context.Context, r *run.Run) error {
	r.Log.Info("dry-run: skipping YouTube upload")
	return r.WriteFile(FileYouTube, []byte(`{"dry_run": true}`+"\n"))
}

func dryArchive(ctx context.Context, r *run.Run) error {
	r.Log.Info("dry-run: skipping S3 archive")
	return r.WriteFile(FileArchive, []byte(`{"dry_run": true}`+"\n"))
}

// dryCleanup keeps the media so the rendered video can be inspected.
func dryCleanup(ctx context.Context, r *run.Run) error {
	r.Log.Info("dry-run: keeping media for inspection (real runs delete audio/images/video here)")
	return writeJSON(r, FileCleanup, CleanupResult{Skipped: true})
}
