package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/thesimpledev/autobotgo/internal/ffmpeg"
	"github.com/thesimpledev/autobotgo/internal/run"
)

// Video assembles final.mp4 with peak memory as the priority, because this is a
// batch job with no deadline, so every ffmpeg invocation is kept to at
// most two open video streams:
//
//  1. each image is rendered straight from the PNG into exact-frame-count
//     pieces: an optional head (the incoming crossfade window), the
//     middle, and an optional tail (the outgoing window). The Ken Burns
//     expression takes a frame offset, so motion is continuous across the
//     pieces.
//  2. each crossfade blends one tail with the next head (both exactly
//     crossfade-length, so xfade needs no trimming)
//  3. one sequential concat pass (the concat demuxer reads one file at a
//     time) alternates middles and crossfades and muxes the narration
//
// Rejected alternatives, both measured on a 5-minute video: a single
// filtergraph chaining xfade across all N segments decodes every segment
// concurrently and peaks over 1 GB; trimming full segments in the concat
// list with inpoint/outpoint leaked one extra frame (those points are
// documented as approximate). This layout is exact by construction and
// peaks under 500 MB.
//
// The work is split across video.go (this orchestration), video_pieces.go
// (how a segment divides into clips), video_render.go (the ffmpeg calls),
// and video_filter.go (the filter strings).
var Video = run.Stage{
	Name:    NameVideo,
	Outputs: []run.PathFn{PathVideo},
	Fn:      videoFn,
}

// keepSegmentsEnv keeps the pass-1 and pass-2 intermediates on disk when
// set to any non-empty value, for inspecting a render that looks wrong.
const keepSegmentsEnv = "AUTOBOTGO_KEEP_SEGMENTS"

func videoFn(ctx context.Context, r *run.Run) error {
	if err := ffmpeg.Check(); err != nil {
		return err
	}

	narration := PathNarration(r)
	dur, err := ffmpeg.ProbeDuration(ctx, narration)
	if err != nil {
		return err
	}
	images, err := listImages(DirImages(r))
	if err != nil {
		return err
	}
	r.Log.Info("assembling video", "narration_secs", fmt.Sprintf("%.1f", dur), "images", len(images))

	v := r.Cfg.Video
	tl, err := BuildTimeline(images, dur, v.EndHoldSeconds, v.CrossfadeSeconds, v.KenBurnsMaxZoom, v.FPS, v.Width, v.Height)
	if err != nil {
		return err
	}
	tlJSON, err := json.MarshalIndent(tl, "", "  ")
	if err != nil {
		return err
	}
	if err := r.WriteFile(FileTimeline, tlJSON); err != nil {
		return err
	}

	if err := os.MkdirAll(DirSegments(r), 0o750); err != nil {
		return err
	}

	// Pass 1: exact-length pieces per image.
	for i, seg := range tl.Segments {
		for _, p := range segmentPieces(tl, i) {
			if err := renderPiece(ctx, r, tl, seg, p); err != nil {
				return fmt.Errorf("segment %d %s: %w", i, p.kind, err)
			}
		}
	}

	// Pass 2: one clip per crossfade boundary.
	for k := 0; k < len(tl.Segments)-1 && tl.CrossfadeFrames > 0; k++ {
		if err := renderCrossfade(ctx, r, tl, k); err != nil {
			return fmt.Errorf("crossfade %d: %w", k, err)
		}
	}

	// Pass 3: sequential concat + audio mux.
	if err := renderFinal(ctx, r, tl); err != nil {
		return err
	}

	// Intermediates only exist to feed pass 3; reclaim the disk.
	if os.Getenv(keepSegmentsEnv) == "" {
		if err := os.RemoveAll(DirSegments(r)); err != nil {
			r.Log.Warn("could not remove segment intermediates", "error", err)
		}
	}
	return nil
}
