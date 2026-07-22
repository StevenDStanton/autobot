# Architecture notes

For people changing the code. Operators want
[operations.md](operations.md) instead.

Most of this is here because the obvious approach was tried first and did
not work. Where that is the case, the note says what was measured, so
nobody has to rediscover it.

## Layout

```
main.go                  CLI, the scheduler loop, logging, run orchestration
internal/config          the config schema, defaults, validation
internal/run             per run state and the idempotent stage executor
internal/stage           the nine pipeline stages
internal/openai          thin wrapper over the OpenAI SDK
internal/yt              YouTube OAuth and upload
internal/ffmpeg          ffmpeg and ffprobe process wrapper
internal/notify          webhook pings
```

Dependencies point inward. `config` and `run` are the shared core, `stage`
orchestrates, nothing imports `main`.

## The stage executor

`run.Execute` is the heart of the design. A `Stage` declares `Outputs` as
functions from a run to a path, and a stage whose outputs all exist and
are non-empty is skipped.

That single rule buys the resume behavior: a failed run can be re-triggered
and it will not re-buy work that already succeeded. It is also why a
zero-byte file counts as incomplete. An interrupted write must never look
like a finished stage.

Everything is written through `WriteFileAtomic`, which writes a `.tmp`
sibling and renames, so a crash mid-write cannot leave a
complete-looking artifact.

When adding a stage, declare every file it produces. A stage that declares
nothing runs every time. A stage that under-declares can be skipped while
its real output is missing, which is worse.

Stage names are constants in `internal/stage/stage.go`. They are used by
the `--stage` flag, the dry-run swap table, `secretNeeds`, and the help
text, so they are declared once.

## Video assembly, and why it looks over-engineered

Peak memory is the constraint. The target is a 1 GB server, so no ffmpeg
invocation may hold more than two video streams open.

The work is split across four files: `video.go` orchestrates,
`video_pieces.go` decides how a segment divides into clips,
`video_render.go` runs ffmpeg, and `video_filter.go` builds filter
strings.

Three passes:

1. Each image renders into exact-frame-count pieces: an optional head (the
   incoming crossfade window), a middle, and an optional tail (the
   outgoing window). The Ken Burns expression takes a frame offset, so
   motion continues seamlessly across piece boundaries.
2. Each crossfade blends one tail with the next head. Both are exactly
   crossfade-length, so `xfade` runs at offset 0 and needs no trimming.
3. One concat pass alternates middles and crossfades and muxes the audio.
   The concat demuxer reads one file at a time.

Two alternatives were measured on a five minute video and rejected:

- **One filtergraph chaining `xfade` across all N segments.** Decodes
  every segment concurrently. Peaks over 1 GB.
- **Trimming full segments in the concat list with `inpoint`/`outpoint`.**
  Leaked an extra frame. Those points are documented as approximate. The
  current layout is exact by construction.

The result peaks under 500 MB.

### Specific traps

**Do not add `-shortest`.** It makes the muxer buffer the longer stream,
measured at about 900 MB extra on a five minute video. The timeline math
already sizes audio and video to identical lengths, so it buys nothing.

**Do not use the stateful `zoom+0.001` zoompan idiom.** It drifts with
frame rate. The expressions in `zoompanExpr` are a function of the global
frame index over the segment's frame count, which makes them deterministic
and fps-independent, and is what lets a piece rendered separately continue
exactly where the previous one stopped.

**The 3x upscale before zoompan is not optional.** zoompan moves in whole
pixels of its input. At 3x those steps land sub-pixel in the output. At 1x
you get visible jitter.

**The concat list must stay trim-free.** Every file in it already has its
exact frame count. Total length is `sum(frames) - (N-1) * crossfade` by
construction, and any trimming breaks that guarantee.

## Timezone

`run_time` is interpreted in `config.timezone`, never the server's clock.
The `time/tzdata` package is imported blank so timezone names resolve on
minimal images that ship no system tzdata.

`Validate` resolves the name once and caches the `*time.Location`, which
`Config.Location()` returns.

## The panic handler

`notifyOnPanic` recovers, sends a failure notification, and then
re-panics on purpose.

Re-panicking is the point. A panic means unknown, possibly corrupt state,
so the process must not carry on scheduling tomorrow's run as if nothing
happened. Crashing hands the problem to systemd, which restarts a clean
process, and the notification is what makes the crash visible instead of
silent.

## Known SDK issues

**`openai-go` v2.7.1: `VectorStoreFileService.NewAndPoll` is broken.** It
passes the file and store IDs to its own `Get` in the wrong order and
returns a 400. `internal/openai/client.go` attaches and polls by hand.
Check whether this is fixed before upgrading, and delete the workaround if
it is.

**`aws-sdk-go-v2/feature/s3/manager` is deprecated** in favor of
`feature/s3/transfermanager`. The replacement is still pre-1.0, so its API
can still change. Given that the archive stage is optional, off by
default, and hard to test without a real bucket, staying on the stable
deprecated package is the better trade for now. Static analysis will report
the deprecation on `internal/stage/archive.go`. That is expected. Revisit
when `transfermanager` reaches 1.0.

## Safety rejections

Image generation sometimes refuses a prompt on content grounds. Failing
the run over one illustration would be a bad trade, so
`openai.IsSafetyRejection` detects it and the image stage retries once
with a sanitized rewrite of the same scene.

Detection checks the SDK's typed error code first, then falls back to
matching the message text, because the API has returned policy refusals
under codes the list does not know about. The fallback is deliberately
limited to 400-class errors: a 429 or 500 that happens to say "rejected"
is a transport problem, and retrying it with a rewritten prompt would hide
a real outage.

## The OAuth short URL

Google's authorization URL is long enough that copying it out of a
terminal reliably loses characters to line wrapping. So `autobotgo auth`
runs a local server that serves a short `http://127.0.0.1:<port>/` and
redirects to Google. Never print the long URL for the user to paste.

## Testing

The tested packages are `config`, `run`, `notify`, `openai` (pure
functions only), and the timeline and chunker math in `stage`. That covers
the logic a contributor is most likely to break.

Stages that call the network are not tested. Doing it properly means
extracting interfaces for the OpenAI, YouTube, and S3 clients so they can
be faked. That is a worthwhile change and nobody has made it yet.

`config.example.json` is generated from `config.Default()` and checked by
a test, so it cannot drift from the schema. After changing the config
struct:

```sh
go test . -update
```

## House style

- No em dashes anywhere in the repo, including code comments. Commas,
  periods, colons, and parentheses cover every case.
- Comments explain why, not what. If a line needs a comment to say what it
  does, rename something instead.
- Where a decision came from a measurement, write down the number.
