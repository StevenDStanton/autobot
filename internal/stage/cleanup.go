package stage

import (
	"context"
	"os"
	"time"

	"github.com/thesimpledev/autobotgo/internal/run"
)

// Cleanup is the final stage: once the video is on YouTube (and archived,
// if S3 is on), the heavy media for this run is deleted: the audio, the
// images, and the video file. Stories are the accumulating asset and are
// never touched; neither is world.md or the small bookkeeping in runs/.
var Cleanup = run.Stage{
	Name:    NameCleanup,
	Outputs: []run.PathFn{runFile(FileCleanup)},
	Timeout: 5 * time.Minute,
	Fn:      cleanupFn,
}

// CleanupResult is the run's final marker; the scheduler treats its
// presence as "this day is done".
type CleanupResult struct {
	Removed   []string `json:"removed"`
	CleanedAt string   `json:"cleaned_at"`
	Skipped   bool     `json:"skipped,omitempty"`
}

func cleanupFn(ctx context.Context, r *run.Run) error {
	res := CleanupResult{CleanedAt: time.Now().UTC().Format(time.RFC3339)}
	targets := []string{
		PathNarration(r),
		DirTTSChunks(r),
		DirImages(r),
		PathVideo(r),
		DirSegments(r), // normally already gone; sweep leftovers from failed runs
		// The images marker asserts "the images exist", so it must die
		// with them, or a later re-run would skip regenerating media
		// that is gone.
		r.Path(FileImagesDone),
	}
	for _, t := range targets {
		if _, err := os.Stat(t); os.IsNotExist(err) {
			continue
		}
		if err := os.RemoveAll(t); err != nil {
			// The upload already succeeded; a cleanup failure should be
			// visible but must not fail the run.
			r.Log.Warn("cleanup: could not remove", "path", t, "error", err)
			continue
		}
		res.Removed = append(res.Removed, t)
	}
	r.Log.Info("media cleaned up", "removed", len(res.Removed))
	return writeJSON(r, FileCleanup, res)
}
