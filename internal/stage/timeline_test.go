package stage

import (
	"strings"
	"testing"
)

func imgs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "images/" + imageName(i)
	}
	return out
}

// The rendered length must equal narration + hold exactly, at frame
// granularity, for any image count.
func TestTimelineTotalIsExact(t *testing.T) {
	cases := []struct {
		name      string
		n         int
		narration float64
		hold      float64
		crossfade float64
		fps       int
	}{
		{"typical", 10, 290.13, 2.0, 1.0, 30},
		{"one image", 1, 60.0, 2.0, 1.0, 30},
		{"two images", 2, 45.7, 0.0, 1.5, 30},
		{"odd remainder", 7, 123.456, 2.0, 1.0, 30},
		{"25fps", 10, 290.0, 2.0, 1.0, 25},
		{"crossfade longer than segment", 30, 20.0, 0.0, 5.0, 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tl, err := BuildTimeline(imgs(tc.n), tc.narration, tc.hold, tc.crossfade, 1.12, tc.fps, 1920, 1080)
			if err != nil {
				t.Fatal(err)
			}
			want := int(tc.narration*float64(tc.fps) + tc.hold*float64(tc.fps) + 0.5)
			if got := tl.TotalFrames(); got != want {
				t.Errorf("total frames = %d, want %d", got, want)
			}
			for i, s := range tl.Segments {
				if s.Frames < 2 {
					t.Errorf("segment %d has %d frames", i, s.Frames)
				}
				if tl.CrossfadeFrames >= s.Frames {
					t.Errorf("crossfade %d >= segment %d frames %d", tl.CrossfadeFrames, i, s.Frames)
				}
			}
			offsets := tl.XfadeOffsets()
			if len(offsets) != tc.n-1 {
				t.Fatalf("got %d offsets, want %d", len(offsets), tc.n-1)
			}
			for i := 1; i < len(offsets); i++ {
				if offsets[i] <= offsets[i-1] {
					t.Errorf("offsets not increasing: %v", offsets)
				}
			}
		})
	}
}

func TestTimelineNoImages(t *testing.T) {
	if _, err := BuildTimeline(nil, 60, 2, 1, 1.12, 30, 1920, 1080); err == nil {
		t.Fatal("expected error for zero images")
	}
}

// Golden-ish test: the pass-1 segment filter and pass-2 xfade graph have
// the expected structure for N=3.
func TestFiltergraphStructure(t *testing.T) {
	tl, err := BuildTimeline(imgs(3), 30.0, 2.0, 1.0, 1.12, 30, 1920, 1080)
	if err != nil {
		t.Fatal(err)
	}

	pieces := segmentPieces(tl, 0)
	segFilter, err := pieceFilter(tl.Segments[0], tl, pieces[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"scale=5760:3240:force_original_aspect_ratio=increase,crop=5760:3240,zoompan=",
		"s=1920x1080:fps=30",
		"format=yuv420p",
	} {
		if !strings.Contains(segFilter, want) {
			t.Errorf("piece filter missing %q\n\n%s", want, segFilter)
		}
	}

	graph := buildAudioGraph(tl, 0.12, false)
	if !strings.Contains(graph, "[1:a]aformat=sample_rates=48000:channel_layouts=stereo,apad=pad_dur=2.000[aout]") {
		t.Errorf("audio graph unexpected:\n%s", graph)
	}
	if strings.Contains(graph, "amix") {
		t.Error("no-music graph should not contain amix")
	}

	withMusic := buildAudioGraph(tl, 0.12, true)
	for _, want := range []string{"volume=0.120", "amix=inputs=2:duration=first:dropout_transition=0:normalize=0", "[2:a]"} {
		if !strings.Contains(withMusic, want) {
			t.Errorf("music audio graph missing %q\n\n%s", want, withMusic)
		}
	}
}

// The pieces plus crossfade clips must reproduce sum(frames) - (N-1)*cf
// exactly, and pieces must tile each segment with no gap or overlap.
func TestPiecesTileSegmentsExactly(t *testing.T) {
	tl, err := BuildTimeline(imgs(5), 123.4, 2.0, 1.0, 1.12, 30, 1920, 1080)
	if err != nil {
		t.Fatal(err)
	}
	cf := tl.CrossfadeFrames
	n := len(tl.Segments)

	totalFrames := 0
	for i, seg := range tl.Segments {
		pieces := segmentPieces(tl, i)
		next := 0
		for _, p := range pieces {
			if p.startFrame != next {
				t.Errorf("segment %d: piece %s starts at %d, want %d", i, p.kind, p.startFrame, next)
			}
			if p.frames < 1 {
				t.Errorf("segment %d: piece %s has %d frames", i, p.kind, p.frames)
			}
			next = p.startFrame + p.frames
			if p.kind == "mid" {
				totalFrames += p.frames
			}
		}
		if next != seg.Frames {
			t.Errorf("segment %d: pieces cover %d frames, segment has %d", i, next, seg.Frames)
		}
		if i > 0 && pieces[0].kind != "head" {
			t.Errorf("segment %d should start with a head piece", i)
		}
		if i == 0 && pieces[0].kind != "mid" {
			t.Error("first segment should not have a head piece")
		}
		if i < n-1 {
			totalFrames += cf // its crossfade clip
		}
	}
	if want := tl.TotalFrames(); totalFrames != want {
		t.Errorf("concat layout is %d frames, timeline wants %d", totalFrames, want)
	}

	list := buildConcatList(tl)
	if !strings.HasPrefix(list, "ffconcat version 1.0\n") {
		t.Error("missing ffconcat header")
	}
	for _, want := range []string{"file 'mid_00.mp4'\nfile 'xf_00.mp4'\n", "file 'mid_04.mp4'\n"} {
		if !strings.Contains(list, want) {
			t.Errorf("concat list missing %q\n\n%s", want, list)
		}
	}
	if strings.Contains(list, "inpoint") || strings.Contains(list, "outpoint") {
		t.Error("concat list must not rely on approximate inpoint/outpoint trimming")
	}
}

func TestPiecesNoCrossfade(t *testing.T) {
	tl, err := BuildTimeline(imgs(3), 60.0, 2.0, 0.0, 1.12, 30, 1920, 1080)
	if err != nil {
		t.Fatal(err)
	}
	for i, seg := range tl.Segments {
		pieces := segmentPieces(tl, i)
		if len(pieces) != 1 || pieces[0].kind != "mid" || pieces[0].frames != seg.Frames {
			t.Errorf("segment %d: zero-crossfade should be one full mid piece, got %+v", i, pieces)
		}
	}
	list := buildConcatList(tl)
	if strings.Contains(list, "xf_") {
		t.Errorf("zero-crossfade list should be plain middles:\n%s", list)
	}
}

// A piece's motion must continue exactly where the previous piece ended:
// the expression at the head's local frame f must equal the full-segment
// expression at global frame startFrame+f.
func TestZoompanExprContinuity(t *testing.T) {
	full, err := zoompanExpr("zoom_in", 300, 0, 1.12)
	if err != nil {
		t.Fatal(err)
	}
	offset, err := zoompanExpr("zoom_in", 300, 270, 1.12)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full, "((on+0)/299)") {
		t.Errorf("full-segment expression not normalized over segment: %s", full)
	}
	if !strings.Contains(offset, "((on+270)/299)") {
		t.Errorf("offset expression must shift by startFrame over the same span: %s", offset)
	}
}

func TestZoompanExprPresets(t *testing.T) {
	for _, motion := range motionPresets {
		expr, err := zoompanExpr(motion, 150, 0, 1.12)
		if err != nil {
			t.Fatalf("%s: %v", motion, err)
		}
		if strings.Contains(expr, "zoom+") {
			t.Errorf("%s uses stateful zoom idiom: %s", motion, expr)
		}
	}
	if _, err := zoompanExpr("wobble", 150, 0, 1.12); err == nil {
		t.Error("expected error for unknown preset")
	}
	if _, err := zoompanExpr("zoom_in", 1, 0, 1.12); err == nil {
		t.Error("expected error for single-frame segment")
	}
	if _, err := zoompanExpr("zoom_in", 150, 150, 1.12); err == nil {
		t.Error("expected error for start frame outside segment")
	}
}
