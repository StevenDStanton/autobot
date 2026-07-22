package stage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/thesimpledev/autobotgo/internal/ffmpeg"
	"github.com/thesimpledev/autobotgo/internal/run"
)

// Encoder settings. x264 memory scales with thread count, and this whole
// stage trades speed for RAM so it fits on a 1 GB server.
const (
	encThreads = "2"
	// Intermediates are near-transparent on purpose: viewers only ever
	// see the final pass's encode, so quality here only has to survive
	// one more generation without visible loss.
	intermediatePreset = "fast"
	intermediateCRF    = "15"

	audioCodec      = "aac"
	audioBitrate    = "192k"
	audioSampleRate = "48000"
)

// intermediateCodec is shared by every pre-final render so the concat
// demuxer sees uniform streams.
func intermediateCodec() []string {
	return []string{
		"-c:v", "libx264", "-preset", intermediatePreset, "-crf", intermediateCRF,
		"-pix_fmt", "yuv420p",
		"-threads", encThreads,
	}
}

// renderPiece renders one exact-frame-count piece of a segment from its
// source image. Finished pieces are kept, so a failed run resumes
// mid-stage.
func renderPiece(ctx context.Context, r *run.Run, tl *Timeline, seg Segment, p piece) error {
	out := filepath.Join(DirSegments(r), p.file)
	if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
		r.Log.Debug("piece exists, keeping", "piece", p.file)
		return nil
	}
	if seg.Type != "image" {
		return fmt.Errorf("segment type %q not supported yet", seg.Type)
	}
	filter, err := pieceFilter(seg, tl, p)
	if err != nil {
		return err
	}
	r.Log.Info("rendering piece", "piece", p.file, "frames", p.frames, "motion", seg.Motion)
	tmp := out + ".tmp.mp4"
	args := []string{"-i", seg.Src, "-vf", filter}
	args = append(args, intermediateCodec()...)
	args = append(args, tmp)
	if err := ffmpeg.Run(ctx, r.Log, args...); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, out)
}

// renderCrossfade blends segment k's tail with segment k+1's head. Both
// inputs are exactly crossfade-length, so xfade runs at offset 0 with no
// trimming and only two streams are open.
func renderCrossfade(ctx context.Context, r *run.Run, tl *Timeline, k int) error {
	out := filepath.Join(DirSegments(r), crossfadeName(k))
	if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
		r.Log.Debug("crossfade exists, keeping", "boundary", k)
		return nil
	}
	cf := secs(tl.CrossfadeFrames, tl.FPS)
	graph := fmt.Sprintf("[0:v][1:v]xfade=transition=fade:duration=%s:offset=0[v]", cf)

	r.Log.Info("rendering crossfade", "boundary", k+1, "of", len(tl.Segments)-1)
	tmp := out + ".tmp.mp4"
	args := []string{
		"-threads", "1", "-i", filepath.Join(DirSegments(r), tailName(k)),
		"-threads", "1", "-i", filepath.Join(DirSegments(r), headName(k+1)),
		"-filter_complex", graph,
		"-map", "[v]",
	}
	args = append(args, intermediateCodec()...)
	args = append(args, tmp)
	if err := ffmpeg.Run(ctx, r.Log, args...); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, out)
}

// renderFinal stitches middles and crossfade clips with the concat
// demuxer (sequential, one file open at a time) and muxes the audio.
func renderFinal(ctx context.Context, r *run.Run, tl *Timeline) error {
	listing := buildConcatList(tl)
	concatPath := filepath.Join(DirSegments(r), "concat.txt")
	if err := writeArtifact(concatPath, []byte(listing)); err != nil {
		return err
	}

	musicPath := ""
	if r.Cfg.Video.BackgroundMusic != "" {
		musicPath = r.Cfg.Resolve(r.Cfg.Video.BackgroundMusic)
	}
	graph := buildAudioGraph(tl, r.Cfg.Video.MusicVolume, musicPath != "")
	if err := r.WriteFile(FileFiltergraph, []byte(graph)); err != nil {
		return err
	}

	args := []string{
		"-threads", "1", "-f", "concat", "-safe", "0", "-i", concatPath,
		"-i", PathNarration(r),
	}
	if musicPath != "" {
		args = append(args, "-stream_loop", "-1", "-i", musicPath)
	}
	out := PathVideo(r)
	if err := ensureDirOf(out); err != nil {
		return err
	}
	tmp := out + ".tmp.mp4" // ffmpeg picks the muxer from the extension
	v := r.Cfg.Video
	args = append(args,
		"-filter_complex_script", r.Path(FileFiltergraph),
		"-map", "0:v", "-map", "[aout]",
		"-c:v", "libx264", "-preset", v.X264Preset, "-crf", strconv.Itoa(v.X264CRF),
		"-pix_fmt", "yuv420p",
		"-c:a", audioCodec, "-b:a", audioBitrate, "-ar", audioSampleRate,
		"-movflags", "+faststart",
		// No -shortest: it makes the muxer buffer the longer stream
		// (measured ~900 MB extra on a 5-minute video), and the timeline
		// math already sizes audio and video to identical lengths.
		"-threads", encThreads,
		tmp,
	)
	r.Log.Info("rendering final video")
	if err := ffmpeg.Run(ctx, r.Log, args...); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, out)
}
