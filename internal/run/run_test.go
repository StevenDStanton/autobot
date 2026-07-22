package run

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thesimpledev/autobotgo/internal/config"
)

func testRun(t *testing.T) *Run {
	t.Helper()
	dir := t.TempDir()
	return &Run{
		Dir:  dir,
		Base: dir,
		Date: "2026-01-02",
		Cfg:  config.Default(),
		Log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// marker declares a stage output at rel inside the run dir.
func marker(rel string) PathFn {
	return func(r *Run) string { return r.Path(rel) }
}

// counter returns a stage function that records how many times it ran.
func counter(runs *int) func(context.Context, *Run) error {
	return func(context.Context, *Run) error {
		*runs++
		return nil
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()

	t.Run("creates missing parent directories", func(t *testing.T) {
		path := filepath.Join(dir, "a", "b", "c.json")
		if err := WriteFileAtomic(path, []byte("hello")); err != nil {
			t.Fatalf("WriteFileAtomic: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading back: %v", err)
		}
		if string(got) != "hello" {
			t.Errorf("content = %q, want %q", got, "hello")
		}
	})

	t.Run("leaves no temp file behind", func(t *testing.T) {
		path := filepath.Join(dir, "clean.txt")
		if err := WriteFileAtomic(path, []byte("x")); err != nil {
			t.Fatalf("WriteFileAtomic: %v", err)
		}
		if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
			t.Errorf("temp sibling still present, want it renamed away")
		}
	})

	t.Run("overwrites an existing file", func(t *testing.T) {
		path := filepath.Join(dir, "twice.txt")
		if err := WriteFileAtomic(path, []byte("first")); err != nil {
			t.Fatalf("first write: %v", err)
		}
		if err := WriteFileAtomic(path, []byte("second")); err != nil {
			t.Fatalf("second write: %v", err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != "second" {
			t.Errorf("content = %q, want %q", got, "second")
		}
	})
}

// The resume behavior is the one that protects against paying twice for
// API calls that already succeeded, so it gets the most coverage.
func TestExecuteSkipsCompletedStages(t *testing.T) {
	t.Run("skips a stage whose outputs exist", func(t *testing.T) {
		r := testRun(t)
		if err := r.WriteFile("done.json", []byte("{}")); err != nil {
			t.Fatal(err)
		}
		ran := 0
		s := Stage{Name: "story", Outputs: []PathFn{marker("done.json")}, Fn: counter(&ran)}
		if err := Execute(context.Background(), r, []Stage{s}, false); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if ran != 0 {
			t.Errorf("stage ran %d times, want 0 (outputs already existed)", ran)
		}
	})

	t.Run("runs a stage whose output is empty", func(t *testing.T) {
		r := testRun(t)
		// A zero-byte file is treated as incomplete: an interrupted write
		// must not look like a finished stage.
		if err := r.WriteFile("empty.json", nil); err != nil {
			t.Fatal(err)
		}
		ran := 0
		s := Stage{Name: "story", Outputs: []PathFn{marker("empty.json")}, Fn: counter(&ran)}
		if err := Execute(context.Background(), r, []Stage{s}, false); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if ran != 1 {
			t.Errorf("stage ran %d times, want 1 (empty output is not done)", ran)
		}
	})

	t.Run("runs a stage when only some outputs exist", func(t *testing.T) {
		r := testRun(t)
		if err := r.WriteFile("one.json", []byte("{}")); err != nil {
			t.Fatal(err)
		}
		ran := 0
		s := Stage{
			Name:    "images",
			Outputs: []PathFn{marker("one.json"), marker("two.json")},
			Fn:      counter(&ran),
		}
		if err := Execute(context.Background(), r, []Stage{s}, false); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if ran != 1 {
			t.Errorf("stage ran %d times, want 1 (partial outputs are not done)", ran)
		}
	})

	t.Run("runs a stage that declares no outputs", func(t *testing.T) {
		r := testRun(t)
		ran := 0
		s := Stage{Name: "video", Fn: counter(&ran)}
		if err := Execute(context.Background(), r, []Stage{s}, false); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if ran != 1 {
			t.Errorf("stage ran %d times, want 1 (no outputs means never skippable)", ran)
		}
	})

	t.Run("force re-runs and clears existing outputs", func(t *testing.T) {
		r := testRun(t)
		if err := r.WriteFile("done.json", []byte("{}")); err != nil {
			t.Fatal(err)
		}
		ran := 0
		s := Stage{Name: "story", Outputs: []PathFn{marker("done.json")}, Fn: counter(&ran)}
		if err := Execute(context.Background(), r, []Stage{s}, true); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if ran != 1 {
			t.Errorf("stage ran %d times, want 1 under force", ran)
		}
		if _, err := os.Stat(r.Path("done.json")); !os.IsNotExist(err) {
			t.Errorf("force did not clear the stale output")
		}
	})
}

func TestExecuteOrderAndFailure(t *testing.T) {
	t.Run("runs stages in order", func(t *testing.T) {
		r := testRun(t)
		var order []string
		mk := func(name string) Stage {
			return Stage{Name: name, Fn: func(context.Context, *Run) error {
				order = append(order, name)
				return nil
			}}
		}
		stages := []Stage{mk("story"), mk("tts"), mk("video")}
		if err := Execute(context.Background(), r, stages, false); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := []string{"story", "tts", "video"}
		if len(order) != len(want) {
			t.Fatalf("ran %v, want %v", order, want)
		}
		for i := range want {
			if order[i] != want[i] {
				t.Fatalf("ran %v, want %v", order, want)
			}
		}
	})

	t.Run("first failure aborts the rest", func(t *testing.T) {
		r := testRun(t)
		boom := errors.New("boom")
		after := 0
		stages := []Stage{
			{Name: "story", Fn: func(context.Context, *Run) error { return boom }},
			{Name: "tts", Fn: counter(&after)},
		}
		err := Execute(context.Background(), r, stages, false)
		if err == nil {
			t.Fatal("Execute returned nil, want an error")
		}
		if after != 0 {
			t.Errorf("later stage ran %d times, want 0", after)
		}

		// The name has to survive so notifications can report which
		// stage failed.
		var se *StageError
		if !errors.As(err, &se) {
			t.Fatalf("error is %T, want *StageError", err)
		}
		if se.Stage != "story" {
			t.Errorf("StageError.Stage = %q, want %q", se.Stage, "story")
		}
		if !errors.Is(err, boom) {
			t.Errorf("errors.Is(err, boom) = false, want true (Unwrap must expose the cause)")
		}
	})

	t.Run("timeout is applied to the stage context", func(t *testing.T) {
		r := testRun(t)
		var deadlineSet bool
		s := Stage{
			Name:    "tts",
			Timeout: time.Minute,
			Fn: func(ctx context.Context, _ *Run) error {
				_, deadlineSet = ctx.Deadline()
				return nil
			},
		}
		if err := Execute(context.Background(), r, []Stage{s}, false); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !deadlineSet {
			t.Error("stage context had no deadline, want one from Stage.Timeout")
		}
	})

	t.Run("no timeout means no deadline", func(t *testing.T) {
		r := testRun(t)
		deadlineSet := true
		s := Stage{
			Name: "video",
			Fn: func(ctx context.Context, _ *Run) error {
				_, deadlineSet = ctx.Deadline()
				return nil
			},
		}
		if err := Execute(context.Background(), r, []Stage{s}, false); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if deadlineSet {
			t.Error("stage context had a deadline, want none")
		}
	})
}

func TestPrepare(t *testing.T) {
	now := time.Date(2026, 3, 4, 8, 0, 0, 0, time.UTC)

	t.Run("defaults to runs/<date> under the config base", func(t *testing.T) {
		cfg := config.Default()
		cfg.BaseDir = t.TempDir()
		dir, date, err := Prepare(cfg, "", now)
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		if date != "2026-03-04" {
			t.Errorf("date = %q, want %q", date, "2026-03-04")
		}
		want := filepath.Join(cfg.BaseDir, "runs", "2026-03-04")
		if dir != want {
			t.Errorf("dir = %q, want %q", dir, want)
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			t.Errorf("Prepare did not create the run directory")
		}
	})

	t.Run("explicit dir takes its date from the basename", func(t *testing.T) {
		cfg := config.Default()
		cfg.BaseDir = t.TempDir()
		explicit := filepath.Join(cfg.BaseDir, "scratch", "2026-12-25")
		dir, date, err := Prepare(cfg, explicit, now)
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		if date != "2026-12-25" {
			t.Errorf("date = %q, want %q", date, "2026-12-25")
		}
		if dir != explicit {
			t.Errorf("dir = %q, want %q", dir, explicit)
		}
	})

	t.Run("reusing an existing directory is not an error", func(t *testing.T) {
		cfg := config.Default()
		cfg.BaseDir = t.TempDir()
		if _, _, err := Prepare(cfg, "", now); err != nil {
			t.Fatalf("first Prepare: %v", err)
		}
		if _, _, err := Prepare(cfg, "", now); err != nil {
			t.Fatalf("second Prepare: %v", err)
		}
	})
}
