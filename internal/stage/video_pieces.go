package stage

import "fmt"

// piece is a contiguous frame range of one segment, rendered to its own
// clip. startFrame offsets the Ken Burns expression so motion continues
// seamlessly from piece to piece.
type piece struct {
	kind       string // "head", "mid", "tail"
	file       string
	startFrame int
	frames     int
}

// segmentPieces splits segment i into head / mid / tail. The first
// segment has no head, the last no tail, and with crossfades disabled
// everything is one mid piece.
func segmentPieces(tl *Timeline, i int) []piece {
	cf := tl.CrossfadeFrames
	total := tl.Segments[i].Frames
	left, right := 0, 0
	if i > 0 && cf > 0 {
		left = cf
	}
	if i < len(tl.Segments)-1 && cf > 0 {
		right = cf
	}

	var out []piece
	if left > 0 {
		out = append(out, piece{"head", headName(i), 0, left})
	}
	out = append(out, piece{"mid", midName(i), left, total - left - right})
	if right > 0 {
		out = append(out, piece{"tail", tailName(i), total - right, right})
	}
	return out
}

// Piece names are bare filenames inside the segments dir, because the concat
// listing lives there too, and its entries resolve relative to it.
func headName(i int) string { return fmt.Sprintf("head_%02d.mp4", i) }
func midName(i int) string  { return fmt.Sprintf("mid_%02d.mp4", i) }
func tailName(i int) string { return fmt.Sprintf("tail_%02d.mp4", i) }

func crossfadeName(k int) string {
	return fmt.Sprintf("xf_%02d.mp4", k)
}
