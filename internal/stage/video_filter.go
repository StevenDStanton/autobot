package stage

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	// kenBurnsUpscale is how much the source image is enlarged before
	// zoompan runs. zoompan moves in whole pixels of its input, so at 3x
	// each step lands sub-pixel in the output and the drift that would
	// otherwise show up as visible jitter disappears.
	kenBurnsUpscale = 3
	// musicFadeOutSecs is how long the background music takes to fade to
	// silence at the end of the video.
	musicFadeOutSecs = 3.0
)

// buildConcatList alternates segment middles with crossfade clips. Every
// listed file already has its exact frame count, so no inpoint/outpoint
// trimming is involved and the total is sum(frames) - (N-1)*cf by
// construction. Paths are relative to the listing file, which sits in
// the same directory as the clips.
func buildConcatList(tl *Timeline) string {
	var b strings.Builder
	b.WriteString("ffconcat version 1.0\n")
	n := len(tl.Segments)
	for i := range tl.Segments {
		fmt.Fprintf(&b, "file '%s'\n", midName(i))
		if i < n-1 && tl.CrossfadeFrames > 0 {
			fmt.Fprintf(&b, "file '%s'\n", crossfadeName(i))
		}
	}
	return b.String()
}

// buildAudioGraph prepares the audio for the final pass: narration padded
// by the end hold; music (if present) attenuated, faded out over the last
// musicFadeOutSecs, and mixed under the narration with normalize=0 so
// narration keeps full level.
func buildAudioGraph(tl *Timeline, musicVolume float64, withMusic bool) string {
	var b strings.Builder
	hold := strconv.FormatFloat(tl.EndHoldSecs, 'f', 3, 64)
	if withMusic {
		fadeStart := math.Max(0, tl.NarrationSecs+tl.EndHoldSecs-musicFadeOutSecs)
		fmt.Fprintf(&b,
			"[1:a]aformat=sample_rates=%s:channel_layouts=stereo,apad=pad_dur=%s[nar];\n",
			audioSampleRate, hold)
		fmt.Fprintf(&b,
			"[2:a]aformat=sample_rates=%s:channel_layouts=stereo,volume=%s,afade=t=out:st=%.3f:d=%g[mus];\n",
			audioSampleRate, strconv.FormatFloat(musicVolume, 'f', 3, 64), fadeStart, musicFadeOutSecs)
		b.WriteString("[nar][mus]amix=inputs=2:duration=first:dropout_transition=0:normalize=0[aout]\n")
	} else {
		fmt.Fprintf(&b,
			"[1:a]aformat=sample_rates=%s:channel_layouts=stereo,apad=pad_dur=%s[aout]\n",
			audioSampleRate, hold)
	}
	return b.String()
}

// pieceFilter builds the pass-1 filter chain for one piece of an image:
// upscale, center-crop to the output aspect, then animate the piece's
// frame range.
func pieceFilter(seg Segment, tl *Timeline, p piece) (string, error) {
	upW, upH := tl.Width*kenBurnsUpscale, tl.Height*kenBurnsUpscale
	expr, err := zoompanExpr(seg.Motion, seg.Frames, p.startFrame, tl.MaxZoom)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,"+
			"zoompan=%s:d=%d:s=%dx%d:fps=%d,format=yuv420p",
		upW, upH, upW, upH,
		expr, p.frames, tl.Width, tl.Height, tl.FPS), nil
}

// zoompanExpr returns the zoompan z/x/y expressions for a motion preset.
// Motion is a function of the global frame index (on + startFrame) over
// the segment's full frame count, so a piece rendered separately
// continues exactly where the previous piece stopped. Deterministic and
// fps-independent. Never use the stateful `zoom+0.001` idiom, it drifts
// with fps.
func zoompanExpr(motion string, segFrames, startFrame int, maxZoom float64) (string, error) {
	if segFrames < 2 {
		return "", fmt.Errorf("segment needs at least 2 frames, got %d", segFrames)
	}
	if startFrame < 0 || startFrame >= segFrames {
		return "", fmt.Errorf("start frame %d outside segment of %d frames", startFrame, segFrames)
	}
	t := fmt.Sprintf("((on+%d)/%d)", startFrame, segFrames-1) // 0..1 across the segment
	z := strconv.FormatFloat(maxZoom, 'f', 4, 64)
	dz := strconv.FormatFloat(maxZoom-1, 'f', 4, 64)
	centerX := "x='iw/2-(iw/zoom/2)'"
	centerY := "y='ih/2-(ih/zoom/2)'"

	switch motion {
	case "zoom_in":
		return fmt.Sprintf("z='1+%s*%s':%s:%s", dz, t, centerX, centerY), nil
	case "zoom_out":
		return fmt.Sprintf("z='%s-%s*%s':%s:%s", z, dz, t, centerX, centerY), nil
	case "pan_right":
		return fmt.Sprintf("z=%s:x='(iw-iw/zoom)*%s':%s", z, t, centerY), nil
	case "pan_left":
		return fmt.Sprintf("z=%s:x='(iw-iw/zoom)*(1-%s)':%s", z, t, centerY), nil
	default:
		return "", fmt.Errorf("unknown motion preset %q", motion)
	}
}
