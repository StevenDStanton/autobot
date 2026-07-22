// autobotgo generates a narrated story video every day and publishes it:
// story -> TTS -> images -> ffmpeg -> YouTube -> optional S3 archive.
//
// Run with no arguments it becomes a long-running scheduler: every day at
// config.run_time, in config.timezone and never the server's own timezone,
// it executes the pipeline. Creating an empty file named `test-run` next to
// the config triggers an immediate run; the file is deleted first.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata" // config.timezone must resolve even without system tzdata

	"github.com/thesimpledev/autobotgo/internal/config"
	"github.com/thesimpledev/autobotgo/internal/notify"
	"github.com/thesimpledev/autobotgo/internal/run"
	"github.com/thesimpledev/autobotgo/internal/stage"
	"github.com/thesimpledev/autobotgo/internal/yt"
)

var version = "dev" // set via -ldflags "-X main.version=..."

const usage = `autobotgo: daily AI story video pipeline

Usage:
  autobotgo             run the daily scheduler (fires at run_time in config.timezone;
                        touch a file named test-run next to config.json to run now)
  autobotgo init        write config.json + story.md templates
  autobotgo auth        one-time YouTube OAuth (run on a machine with a browser)
  autobotgo run [flags] run the pipeline once, right now (see autobotgo run -h)
  autobotgo version     print version
`

func main() {
	if len(os.Args) < 2 {
		os.Exit(cmdScheduler(nil))
	}
	// Bare flags (e.g. `autobotgo --dry-run`) belong to the scheduler.
	if strings.HasPrefix(os.Args[1], "-") && os.Args[1] != "-h" && os.Args[1] != "--help" {
		os.Exit(cmdScheduler(os.Args[1:]))
	}
	var code int
	switch os.Args[1] {
	case "start", "daemon":
		code = cmdScheduler(os.Args[2:])
	case "init":
		code = cmdInit()
	case "auth":
		code = cmdAuth(os.Args[2:])
	case "run":
		code = cmdRun(os.Args[2:])
	case "version":
		fmt.Println("autobotgo", version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		code = 2
	}
	os.Exit(code)
}

// defaultConfigPath prefers config.json next to the binary (the installed
// server layout) and falls back to the current directory (the dev loop).
func defaultConfigPath() string {
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "config.json")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "config.json"
}

func cmdInit() int {
	if err := config.WriteTemplate("config.json"); err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
	} else {
		fmt.Println("wrote config.json. Fill in your keys and settings")
	}
	if _, err := os.Stat("story.md"); err == nil {
		fmt.Println("story.md already exists, not overwriting")
		return 0
	}
	if err := run.WriteFileAtomic("story.md", []byte(storyTemplate)); err != nil {
		fmt.Fprintln(os.Stderr, "story template:", err)
		return 1
	}
	fmt.Println("wrote story.md. Replace its contents with your own story brief")
	return 0
}

func cmdAuth(args []string) int {
	fs := flag.NewFlagSet("auth", flag.ExitOnError)
	cfgPath := fs.String("config", defaultConfigPath(), "path to config.json")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := cfg.CheckYouTubeClient(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	refreshToken, err := yt.Authorize(context.Background(),
		cfg.YouTube.ClientID, cfg.YouTube.ClientSecret)
	if err != nil {
		fmt.Fprintln(os.Stderr, "auth failed:", err)
		return 1
	}
	cfg.YouTube.RefreshToken = refreshToken
	if err := cfg.Save(*cfgPath); err != nil {
		fmt.Fprintln(os.Stderr, "saving refresh token to config:", err)
		return 1
	}
	fmt.Printf("refresh token saved into %s. Copy that file to the server.\n", *cfgPath)
	fmt.Println()
	fmt.Println("IMPORTANT: in Google Cloud Console, set the OAuth consent screen publishing")
	fmt.Println("status to \"In production\". Left in \"Testing\", this token dies every 7 days.")
	return 0
}

// cmdScheduler is the long-running mode: wake every few seconds, run the
// pipeline when the test-run file appears or the daily time arrives.
func cmdScheduler(args []string) int {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	cfgPath := fs.String("config", defaultConfigPath(), "path to config.json")
	dryRun := fs.Bool("dry-run", false, "fire placeholder runs on the schedule: no credentials needed, no API calls, real ffmpeg")
	verbose := fs.Bool("verbose", false, "debug logging (includes ffmpeg output)")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	// The scheduler runs the full pipeline, so demand every credential
	// up front, since misconfiguration should surface at start, not at
	// tomorrow's run time. Dry-run mode needs none of them: scheduling
	// behavior is independent of the pipeline it triggers.
	if !*dryRun {
		if err := cfg.CheckSecrets(true, true, true); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}

	loc := cfg.Location()
	runAt, err := time.Parse("15:04", cfg.RunTime)
	if err != nil { // Load validated this already; belt and braces
		fmt.Fprintln(os.Stderr, "run_time:", err)
		return 2
	}

	log, closeLogs, err := setupLogging(cfg, "", *verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closeLogs()

	days := "every day"
	if len(cfg.RunDays) > 0 {
		days = strings.Join(cfg.RunDays, ",")
	}
	testRunPath := filepath.Join(cfg.BaseDir, "test-run")
	log.Info("autobotgo scheduler started",
		"version", version, "run_time", cfg.RunTime, "run_days", days, "timezone", cfg.Timezone,
		"test_run_file", testRunPath, "dry_run", *dryRun)
	log.Info("note: config changes take effect after a restart")

	runMinutes := runAt.Hour()*60 + runAt.Minute()
	lastAttempt := "" // date of the last scheduled attempt; failures don't retry until tomorrow

	for {
		now := time.Now().In(loc)
		today := now.Format("2006-01-02")

		trigger := ""
		if _, err := os.Stat(testRunPath); err == nil {
			if err := os.Remove(testRunPath); err != nil {
				log.Error("could not remove test-run file", "error", err)
			}
			if dayCompleted(cfg, today) {
				// The video is already made, uploaded, and cleaned up;
				// re-running would rebuild (and in real mode re-buy)
				// media for a day that is done.
				log.Info("test-run ignored: today's run already completed",
					"date", today, "hint", "delete runs/"+today+" to redo the whole day")
				continue
			}
			trigger = "test-run file"
		} else if cfg.IsRunDay(now.Weekday()) &&
			lastAttempt != today &&
			now.Hour()*60+now.Minute() >= runMinutes &&
			!dayCompleted(cfg, today) {
			// Also catches up after downtime: starting the scheduler on
			// a run day past run_time with no finished video runs now.
			trigger = "scheduled run"
			lastAttempt = today
		}

		if trigger != "" {
			log.Info("starting pipeline run", "trigger", trigger, "date", today, "dry_run", *dryRun)
			dir := filepath.Join(cfg.Resolve(cfg.RunsDir), today)
			executeRun(cfg, dir, today, stage.Pipeline(*dryRun), false, *dryRun, *verbose)
		}

		time.Sleep(5 * time.Second)
	}
}

// dayCompleted reports whether the given date's run finished: cleanup is
// the last stage and always writes its marker.
func dayCompleted(cfg *config.Config, date string) bool {
	fi, err := os.Stat(filepath.Join(cfg.Resolve(cfg.RunsDir), date, stage.FileCleanup))
	return err == nil && fi.Size() > 0
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", defaultConfigPath(), "path to config.json")
	stagesFlag := fs.String("stage", "",
		"comma-separated subset of stages to run ("+strings.Join(stage.Names(), ",")+")")
	runDir := fs.String("run-dir", "", "run directory (default runs/<today in New York>)")
	dryRun := fs.Bool("dry-run", false, "generate placeholder artifacts instead of calling APIs; ffmpeg still runs for real")
	force := fs.Bool("force", false, "re-run selected stages even if their outputs exist")
	verbose := fs.Bool("verbose", false, "debug logging (includes ffmpeg output)")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	stages, err := selectStages(stage.Pipeline(*dryRun), *stagesFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if !*dryRun {
		if err := cfg.CheckSecrets(secretNeeds(stages)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}

	// Same clock as the scheduler so manual and scheduled runs share
	// run directories.
	now := time.Now().In(cfg.Location())
	dir, date, err := run.Prepare(cfg, *runDir, now)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return executeRun(cfg, dir, date, stages, *force, *dryRun, *verbose)
}

// executeRun performs one pipeline run in dir: logging, notifications,
// panic protection. Used by both the scheduler and `autobotgo run`.
func executeRun(cfg *config.Config, dir, date string, stages []run.Stage, force, dryRun, verbose bool) int {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	log, closeLogs, err := setupLogging(cfg, dir, verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closeLogs()

	r := &run.Run{Dir: dir, Base: cfg.BaseDir, Date: date, Cfg: cfg, Log: log, DryRun: dryRun}
	log.Info("run starting", "version", version, "run_dir", dir)

	defer notifyOnPanic(log, cfg.NotifyURL)

	if err := run.Execute(context.Background(), r, stages, force); err != nil {
		log.Error("run failed", "error", err)
		stageName := "unknown"
		var se *run.StageError
		if errors.As(err, &se) {
			stageName = se.Stage
		}
		notify.Send(log, cfg.NotifyURL, notify.Failure(stageName, err))
		return 1
	}

	title, url := readOutcome(r)
	log.Info("run complete", "title", title, "url", url)
	notify.Send(log, cfg.NotifyURL, notify.Success(title, url))
	return 0
}

// notifyOnPanic sends a failure notification for a panicking run and then
// re-panics on purpose. Deferred by executeRun.
//
// Re-panicking is the point. A panic means an unknown, possibly corrupt
// state, so the process must not carry on scheduling tomorrow's run as if
// nothing happened. Crashing hands the problem to systemd, which restarts
// a clean process, and the notification is what makes the crash visible
// rather than silent. Recovering without re-panicking would turn a loud
// failure into a scheduler that quietly keeps running in a bad state.
func notifyOnPanic(log *slog.Logger, notifyURL string) {
	p := recover()
	if p == nil {
		return
	}
	notify.Send(log, notifyURL, notify.Failure("panic", fmt.Errorf("%v", p)))
	panic(p)
}

// selectStages filters the pipeline to the requested names, preserving
// pipeline order regardless of the order given.
func selectStages(pipeline []run.Stage, csv string) ([]run.Stage, error) {
	if strings.TrimSpace(csv) == "" {
		return pipeline, nil
	}
	want := map[string]bool{}
	for name := range strings.SplitSeq(csv, ",") {
		want[strings.TrimSpace(name)] = true
	}
	var out []run.Stage
	for _, s := range pipeline {
		if want[s.Name] {
			out = append(out, s)
			delete(want, s.Name)
		}
	}
	if len(want) > 0 {
		var unknown []string
		for name := range want {
			unknown = append(unknown, name)
		}
		return nil, fmt.Errorf("unknown stage(s): %s", strings.Join(unknown, ", "))
	}
	return out, nil
}

// secretNeeds reports which credentials the selected stages require.
func secretNeeds(stages []run.Stage) (openai, youtube, aws bool) {
	for _, s := range stages {
		switch s.Name {
		case stage.NameStory, stage.NameWorld, stage.NameTTS, stage.NameImages, stage.NameMetadata:
			openai = true
		case stage.NameUpload:
			youtube = true
		case stage.NameArchive:
			aws = true
		}
	}
	return
}

// setupLogging writes to stdout (journald picks it up under systemd), the
// global log file, and, when runDir is set, the per-run run.log.
func setupLogging(cfg *config.Config, runDir string, verbose bool) (*slog.Logger, func(), error) {
	writers := []io.Writer{os.Stdout}
	var closers []io.Closer

	paths := []string{cfg.Resolve(cfg.LogFile)}
	if runDir != "" {
		paths = append(paths, filepath.Join(runDir, "run.log"))
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		// #nosec G304 -- paths are the configured log file and the run directory log
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, fmt.Errorf("opening log file: %w", err)
		}
		writers = append(writers, f)
		closers = append(closers, f)
	}

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: level}))
	return log, func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}, nil
}

// readOutcome pulls the title and video URL for the success notification.
func readOutcome(r *run.Run) (title, url string) {
	if data, err := os.ReadFile(r.Path(stage.FileMetadata)); err == nil {
		var md stage.VideoMetadata
		if json.Unmarshal(data, &md) == nil {
			title = md.Title
		}
	}
	if data, err := os.ReadFile(r.Path(stage.FileYouTube)); err == nil {
		var up stage.UploadResult
		if json.Unmarshal(data, &up) == nil {
			url = up.URL
		}
	}
	return
}

const storyTemplate = `# Story Brief

Describe the kind of stories you want, in plain language. The whole file is
given to the model as the writing brief every day. This file is the entry
point to the whole system, so it helps to know what the model sees besides it:

- **world.md**: every story so far has been distilled into a "world bible"
  (characters, places, objects, timeline, open threads) that is sent along
  with this brief. All stories share one continuing world: characters can
  return, places persist, events have consequences. The file is capped in
  size (config: world.max_words) and automatically compressed when it grows
  too big. You can edit world.md by hand any time: kill a character,
  correct a fact, retire a thread, and future stories will honor it.
- **the story archive**: every past story is also stored in a search index
  (config: world.rag_enabled). While writing, the model can look up old
  stories for details the world bible is too compact to hold. Local copies
  of every story stay in the stories/ folder.

So write this brief for a *series*, not a single story. Cover:

## Theme

Quiet, atmospheric folk tales with a hint of the mysterious. Coastal
villages, old forests, lighthouses, long winters. Never horror, never gore.
Wonder and melancholy instead.

## World

One region the stories live in: a stretch of cold coast and the forested
hills behind it. Feel free to seed it: name a village or two, a landmark,
a legend the locals repeat. The world bible will grow from whatever the
stories establish.

## Continuity

- Bringing back characters or places from earlier stories is welcome
- Consequences persist: what broke stays broken, who left stays gone
  unless a story brings them back
- Every story must still make sense to someone watching for the first time

## Voice and style

- Third person, past tense
- Simple, concrete language; short sentences
- A gentle twist or revelation near the end
- End on an image, not a moral

## Things to avoid

- Named real places or people
- Modern technology
- Violence

## Example opening (for tone)

The lighthouse keeper counted storms the way other people counted birthdays.
Each one left a mark somewhere: a cracked pane, a bent railing, a story he
would polish for years until it shone brighter than the lamp itself.
`
