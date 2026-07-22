package stage

import (
	"fmt"
	"strconv"
)

// Segment is one visual in the video. Type "image" gets a Ken Burns
// zoompan branch. A future type "clip" (Veo etc.) would get scale/trim
// instead. That seam is the whole reason this file exists as data.
type Segment struct {
	Type     string  `json:"type"`
	Src      string  `json:"src"`      // run-relative
	Frames   int     `json:"frames"`   // display length in output frames
	Duration float64 `json:"duration"` // Frames/FPS, for humans reading the JSON
	Motion   string  `json:"motion"`
}

// Timeline is the computed plan the filtergraph builder consumes. It is
// written to timeline.json before ffmpeg runs, as a debug artifact.
type Timeline struct {
	FPS             int       `json:"fps"`
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	CrossfadeFrames int       `json:"crossfade_frames"`
	NarrationSecs   float64   `json:"narration_seconds"`
	EndHoldSecs     float64   `json:"end_hold_seconds"`
	MaxZoom         float64   `json:"max_zoom"`
	Segments        []Segment `json:"segments"`
}

// motionPresets rotate per image for variety. Deterministic on purpose:
// same inputs, same video.
var motionPresets = []string{"zoom_in", "pan_right", "zoom_out", "pan_left"}

// BuildTimeline distributes the narration duration (plus end hold) across
// the images. All arithmetic is in integer output frames so chained xfade
// offsets never accumulate float drift; the rounding remainder lands on
// the last image.
//
// With chained crossfades of c frames, the rendered length is
// sum(frames_i) - (N-1)*c, which must equal round((narration+hold)*fps).
func BuildTimeline(images []string, narrationSecs, endHoldSecs, crossfadeSecs, maxZoom float64, fps, width, height int) (*Timeline, error) {
	n := len(images)
	if n == 0 {
		return nil, fmt.Errorf("no images to build a timeline from")
	}
	totalFrames := int(narrationSecs*float64(fps) + endHoldSecs*float64(fps) + 0.5)
	cf := int(crossfadeSecs*float64(fps) + 0.5)

	sumFrames := totalFrames + (n-1)*cf
	base := sumFrames / n
	rem := sumFrames - base*n

	// A crossfade longer than a segment breaks xfade; shrink it rather
	// than fail. Only reachable with extreme config values.
	if n > 1 && cf >= base {
		cf = base / 2
		sumFrames = totalFrames + (n-1)*cf
		base = sumFrames / n
		rem = sumFrames - base*n
	}
	if base < 2 {
		return nil, fmt.Errorf("segments too short: %d images across %d frames", n, totalFrames)
	}

	tl := &Timeline{
		FPS:             fps,
		Width:           width,
		Height:          height,
		CrossfadeFrames: cf,
		NarrationSecs:   narrationSecs,
		EndHoldSecs:     endHoldSecs,
		MaxZoom:         maxZoom,
	}
	for i, src := range images {
		frames := base
		if i == n-1 {
			frames += rem
		}
		tl.Segments = append(tl.Segments, Segment{
			Type:     "image",
			Src:      src,
			Frames:   frames,
			Duration: float64(frames) / float64(fps),
			Motion:   motionPresets[i%len(motionPresets)],
		})
	}
	return tl, nil
}

// TotalFrames returns the rendered video length in frames.
func (t *Timeline) TotalFrames() int {
	sum := 0
	for _, s := range t.Segments {
		sum += s.Frames
	}
	return sum - (len(t.Segments)-1)*t.CrossfadeFrames
}

// XfadeOffsets returns the offset (in frames) of each crossfade: the k-th
// xfade starts at sum(frames_0..k) - (k+1)*cf into the accumulated chain.
func (t *Timeline) XfadeOffsets() []int {
	offsets := make([]int, 0, len(t.Segments)-1)
	acc := 0
	for k := 0; k < len(t.Segments)-1; k++ {
		acc += t.Segments[k].Frames
		offsets = append(offsets, acc-(k+1)*t.CrossfadeFrames)
	}
	return offsets
}

// secs renders a frame count as a seconds literal for ffmpeg args.
func secs(frames, fps int) string {
	return strconv.FormatFloat(float64(frames)/float64(fps), 'f', 6, 64)
}
