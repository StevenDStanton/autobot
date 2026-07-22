// Package run manages per-run working state and executes pipeline stages
// with idempotent resume: a stage whose declared outputs already exist
// (non-empty) is skipped, so re-running after a failure never re-spends
// on API calls that already succeeded.
package run

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/thesimpledev/autobotgo/internal/config"
)

// Run is the shared state handed to every stage. Artifacts are organized
// by type under Base (stories/, audio/, images/, videos/, where stories are
// permanent, the rest are emptied by the cleanup stage), while Dir holds
// the run's small bookkeeping files.
type Run struct {
	Dir    string // bookkeeping dir, runs/<date>
	Base   string // config base dir; typed artifact folders live here
	Date   string // YYYY-MM-DD (or the --run-dir basename when overridden)
	Cfg    *config.Config
	Log    *slog.Logger
	DryRun bool
}

// Path resolves a bookkeeping-file path inside the run dir.
func (r *Run) Path(rel string) string {
	return filepath.Join(r.Dir, rel)
}

// WriteFile writes data to a bookkeeping path atomically.
func (r *Run) WriteFile(rel string, data []byte) error {
	return WriteFileAtomic(r.Path(rel), data)
}

// WriteFileAtomic writes to path via a .tmp sibling and rename, so a
// crash mid-write never leaves a complete-looking artifact behind.
func WriteFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// PathFn resolves an artifact location for a run; stages declare their
// outputs as PathFns because artifacts live in different typed folders.
type PathFn func(*Run) string

// Stage is one pipeline step. Outputs are the artifacts whose existence
// marks the stage complete.
type Stage struct {
	Name    string
	Outputs []PathFn
	Timeout time.Duration
	Fn      func(ctx context.Context, r *Run) error
}

// done reports whether every declared output exists and is non-empty.
func (s Stage) done(r *Run) bool {
	if len(s.Outputs) == 0 {
		return false
	}
	for _, out := range s.Outputs {
		fi, err := os.Stat(out(r))
		if err != nil || fi.Size() == 0 {
			return false
		}
	}
	return true
}

// clearOutputs removes declared outputs so --force re-runs the stage.
func (s Stage) clearOutputs(r *Run) error {
	for _, out := range s.Outputs {
		if err := os.Remove(out(r)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// StageError wraps a stage failure with the stage name for notifications.
type StageError struct {
	Stage string
	Err   error
}

func (e *StageError) Error() string { return fmt.Sprintf("stage %s: %v", e.Stage, e.Err) }
func (e *StageError) Unwrap() error { return e.Err }

// Execute runs the given stages in order, skipping completed ones.
// The first failure aborts and is returned as a *StageError.
func Execute(ctx context.Context, r *Run, stages []Stage, force bool) error {
	for _, s := range stages {
		if force {
			if err := s.clearOutputs(r); err != nil {
				return &StageError{Stage: s.Name, Err: err}
			}
		} else if s.done(r) {
			r.Log.Info("stage skipped, outputs exist", "stage", s.Name)
			continue
		}
		r.Log.Info("stage starting", "stage", s.Name)
		start := time.Now()

		sctx := ctx
		var cancel context.CancelFunc
		if s.Timeout > 0 {
			sctx, cancel = context.WithTimeout(ctx, s.Timeout)
		}
		err := s.Fn(sctx, r)
		if cancel != nil {
			cancel()
		}
		if err != nil {
			return &StageError{Stage: s.Name, Err: err}
		}
		r.Log.Info("stage finished", "stage", s.Name, "took", time.Since(start).Round(time.Millisecond).String())
	}
	return nil
}

// Prepare creates (or reuses) the bookkeeping directory and derives the
// run's date label. An empty dir picks the default runs/<YYYY-MM-DD>
// under the config's runs dir.
func Prepare(cfg *config.Config, dir string, now time.Time) (absDir, date string, err error) {
	if dir == "" {
		date = now.Format("2006-01-02")
		dir = filepath.Join(cfg.Resolve(cfg.RunsDir), date)
	} else {
		date = filepath.Base(dir)
	}
	absDir, err = filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(absDir, 0o750); err != nil {
		return "", "", err
	}
	return absDir, date, nil
}
