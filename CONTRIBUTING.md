# Contributing

## Getting set up

You need Go 1.25 or later and ffmpeg.

```sh
git clone https://github.com/thesimpledev/autobotgo.git
cd autobotgo
make build
make check          # format, vet, and test
```

You do not need an OpenAI key to work on most of this. Dry run mode
exercises the whole pipeline, including a real ffmpeg render, with no API
calls:

```sh
./bin/autobotgo init
./bin/autobotgo run --dry-run
```

That is the fastest feedback loop for anything touching the video stage.

## Before you open a pull request

```sh
make check
go test -race ./...
```

If you change the config struct, regenerate the committed example or the
test will fail:

```sh
go test . -update
```

## House style

The point of the style rules is that the repo reads like one person wrote
it.

- **No em dashes.** Anywhere. Not in docs, not in code comments, not in
  commit messages. Commas, periods, colons, and parentheses cover every
  case.
- **Plain English.** Short sentences, ordinary words. If a reader has to
  reread a sentence, it needs rewriting rather than shortening.
- **Comments explain why, not what.** If a line needs a comment to say
  what it does, rename something instead. Where a decision came from a
  measurement, write the number down.
- Errors start lowercase and do not end with punctuation, per Go
  convention.
- Table-driven tests, with the case name saying what behavior is being
  asserted.

[docs/architecture-notes.md](docs/architecture-notes.md) explains how the
pieces fit together and, more usefully, which obvious approaches were
already tried and rejected. Read it before changing the video stage.

## Commit messages

A short lowercase summary line saying what changed and why, then detail in
the body if the change is not self-evident.

**Do not add AI attribution trailers.** No `Co-authored-by`,
`Co-developed-by`, or `Signed-off-by` for an AI tool. If an assistant
helped, the tag is:

```
Assisted-by: AGENT_NAME:MODEL_VERSION
```

The human submitting the change is the author and takes responsibility for
it, including anything a tool generated.

## Things worth doing

- **Interfaces for the network clients.** The OpenAI, YouTube, and S3
  calls cannot be tested because they are concrete types. Extracting
  interfaces would let the stages be tested with fakes. This is the
  biggest gap in the test suite.
- **Video clip support.** `video.clips` is a config seam for mixing
  generated clips into the timeline. The timeline already models segments
  by type. Nothing is implemented behind it.
- **Move off the deprecated S3 uploader** once
  `feature/s3/transfermanager` reaches 1.0.

## Reporting bugs

Include what you ran, what happened, and what you expected. For pipeline
failures the per run log at `runs/<date>/run.log` is the useful one, and
`--verbose` adds ffmpeg output.

Please scrub API keys, tokens, and YouTube URLs before pasting logs.
