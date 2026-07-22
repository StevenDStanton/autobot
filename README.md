# AutoBotGo

AutoBotGo writes a story, narrates it, illustrates it, turns it into a
video, and uploads it to YouTube as **private**, once a day, on its own.
You watch it and decide whether to publish.

It is one Go binary and one setup script. It runs on the smallest server
either AWS or DigitalOcean will sell you.

Every story shares one world. The program keeps a canon file that grows as
it writes, so characters, places, and unresolved threads carry from one
video to the next instead of each one starting from nothing.

## What it does each day

At `run_time` in your configured timezone, the pipeline runs nine stages:

```
story  ->  world  ->  tts  ->  images  ->  video
                                             |
                    cleanup  <-  archive  <-  upload  <-  metadata
```

1. **story** writes a story of about five minutes, guided by your
   `story.md` brief, the accumulated `world.md` canon, and (when retrieval
   is on) a search over every story it has written before.
2. **world** distills the new story into `world.md`: characters, places,
   timeline, open threads. When the file outgrows its cap the model
   compresses it back down. It also adds the story to the search archive.
3. **tts** narrates the story with OpenAI text to speech, in the voice and
   delivery style you set.
4. **images** generates roughly two illustrations per minute, kept
   visually consistent from scene to scene.
5. **video** assembles a 1080p MP4 with ffmpeg: a slow pan or zoom on each
   image, crossfades between them, the narration track, and optional
   background music.
6. **metadata** writes the YouTube title, description, and tags, including
   the AI disclosure.
7. **upload** sends it to YouTube as private.
8. **archive** optionally tars the run to S3. Off by default.
9. **cleanup** deletes the run's audio, images, and video. Stories are
   kept forever.

A run costs roughly $0.50 to $1.50 depending on image quality.

## What you need

- A server running Ubuntu 24.04 with at least 1 GB of RAM. The setup
  script adds a 2 GB swap file, which is what makes 1 GB enough. On one
  CPU a five minute video takes about an hour to render, which is fine for
  something that runs once a day.
- An OpenAI API key.
- A Google Cloud project with YouTube Data API v3 enabled and an OAuth
  desktop client. See [docs/youtube-oauth.md](docs/youtube-oauth.md).
- An S3 bucket, only if you want the optional archive.

## Quick start

Everything below happens on your own computer. You need a browser for the
YouTube step, which is why it does not happen on the server.

```sh
git clone https://github.com/thesimpledev/autobotgo.git
cd autobotgo
make build

./bin/autobotgo init
```

`init` writes two files:

- `config.json`, where every `************************` is a value you
  have to fill in. [docs/configuration.md](docs/configuration.md) explains
  every setting. There is a complete example in
  [config.example.json](config.example.json).
- `story.md`, the brief describing the stories you want. The template
  shows the kind of detail that works: theme, voice, things to avoid, and
  a sample opening for tone.

Then authorize YouTube once, following
[docs/youtube-oauth.md](docs/youtube-oauth.md):

```sh
./bin/autobotgo auth
```

That writes `youtube.refresh_token` straight into `config.json`, so that
one file holds everything the program needs.

## Try it before spending anything

`--dry-run` runs the whole pipeline with placeholder text, a generated
tone instead of narration, and numbered color cards instead of
illustrations. It makes zero API calls, but ffmpeg runs for real, so you
get a genuine MP4 and a genuine measurement of how long a render takes.

```sh
./bin/autobotgo run --dry-run     # the full pipeline, free
./bin/autobotgo --dry-run         # the scheduler, firing free runs
```

Then spend a little on one stage at a time:

```sh
./bin/autobotgo run --stage story    # read stories/<date>.md
./bin/autobotgo run --stage tts      # listen to audio/<date>.wav
```

For a cheap end to end test, set `"target_minutes": 1` and
`"image_quality": "low"` in `config.json` and run `./bin/autobotgo run`.
That is a real one minute video for a few cents. Delete the private upload
from YouTube Studio afterwards and put your real settings back.

## Deploy it

Both guides start from an empty account and end at a verified first run.
Pick the one you are using:

- [Deploy to AWS](docs/deploy-aws.md), about $12 a month
- [Deploy to DigitalOcean](docs/deploy-digitalocean.md), about $6 a month

The install steps are the same on both. Only picking the machine and
connecting to it differ.

Once it is running, [docs/operations.md](docs/operations.md) covers
everything after that: watching logs, triggering a run, updating the
binary, what to do when a run fails, and what to back up.

## Where files live

Artifacts sit in typed folders next to the binary:

| Path | Kept? | What it is |
|---|---|---|
| `stories/` | permanent | Every story ever written |
| `world.md` | permanent | The canon. Edit it by hand any time |
| `runs/<date>/` | permanent | Small logs and bookkeeping |
| `audio/`, `images/`, `videos/` | temporary | Deleted by cleanup after upload |

`world.md` is meant to be edited. Kill a character, correct a fact, retire
a thread, and future stories will honor it.

## Commands

```
autobotgo                  run the scheduler (this is what the service does)
    --dry-run              fire placeholder runs on schedule, no API calls
    --verbose              debug logging, including ffmpeg output
autobotgo init             write the config.json and story.md templates
autobotgo auth             one time YouTube authorization (needs a browser)
autobotgo run [flags]      run the pipeline once, right now
    --dry-run              placeholder artifacts, no API calls, real ffmpeg
    --stage a,b,c          run only these stages
    --run-dir PATH         operate on a specific run directory
    --force                re-run stages even when their output exists
    --config PATH          config file location
    --verbose              debug logging, including ffmpeg output
autobotgo version          print the version
```

Stage names for `--stage`: `story`, `world`, `tts`, `images`, `video`,
`metadata`, `upload`, `archive`, `cleanup`.

## A note on cost and failure

Stages are idempotent. Each one declares the files it produces, and a
stage whose files already exist is skipped. So when a run fails halfway,
fixing the cause and triggering it again resumes from the broken stage.
You never pay twice for a story that was already written.

A failed run is not retried automatically until the next day. That is
deliberate: a run that fails for a reason you have not fixed would
otherwise burn money on a loop.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). For how the pieces fit together
and why some of the stranger decisions were made, read
[docs/architecture-notes.md](docs/architecture-notes.md).

## License

MIT. See [LICENSE](LICENSE).
