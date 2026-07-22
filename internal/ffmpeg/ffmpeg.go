// Package ffmpeg wraps the ffmpeg and ffprobe binaries via os/exec.
package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Check verifies both binaries are on PATH.
func Check() error {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%s not found on PATH (install ffmpeg)", bin)
		}
	}
	return nil
}

// Run executes ffmpeg with the given args, streaming stderr lines to the
// logger at debug level. ffmpeg writes all diagnostics to stderr.
func Run(ctx context.Context, log *slog.Logger, args ...string) error {
	full := append([]string{"-hide_banner", "-nostdin", "-y"}, args...)
	log.Debug("ffmpeg", "args", strings.Join(full, " "))
	// #nosec G204 -- fixed binary, args are program-generated paths and filters
	cmd := exec.CommandContext(ctx, "ffmpeg", full...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var tail []string
	sc := bufio.NewScanner(stderr)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		log.Debug("ffmpeg: " + line)
		tail = append(tail, line)
		if len(tail) > 15 {
			tail = tail[1:]
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w\nlast output:\n%s", err, strings.Join(tail, "\n"))
	}
	return nil
}

// ProbeDuration returns a media file's duration in seconds.
func ProbeDuration(ctx context.Context, path string) (float64, error) {
	// #nosec G204 -- fixed binary, path comes from the run directory
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		path)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w: %s", path, err, errBuf.String())
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(out.String()), 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: bad duration %q: %w", path, out.String(), err)
	}
	return d, nil
}

// ImageSize returns an image file's pixel dimensions using Go's image
// decoders (cheaper and simpler than shelling out to ffprobe).
func ImageSize(path string) (w, h int, err error) {
	f, err := os.Open(path) // #nosec G304 -- path comes from the run directory
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, fmt.Errorf("decoding %s: %w", path, err)
	}
	return cfg.Width, cfg.Height, nil
}
