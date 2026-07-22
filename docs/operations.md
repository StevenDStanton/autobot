# Running AutoBotGo day to day

Everything after your first successful run. This applies to both clouds,
because by this point they are both just an Ubuntu box with a systemd
service. Where a task genuinely differs, the difference is noted inline.

Commands assume the default install at `/opt/autobotgo` with the service
user `autobotgo`.

## The three commands you will actually use

```sh
journalctl -u autobotgo -f                        # watch it work
sudo -u autobotgo touch /opt/autobotgo/test-run   # run right now
sudo systemctl restart autobotgo                  # after any config change
```

## Watching it

```sh
journalctl -u autobotgo -f              # follow live
journalctl -u autobotgo --since today   # today only
journalctl -u autobotgo -p err          # errors only
```

There is also a plain log file at `/opt/autobotgo/autobotgo.log`, and each
run keeps its own copy in `/opt/autobotgo/runs/<date>/run.log`. When
something goes wrong on a specific day, the per run log is the one you
want.

### What normal looks like

At startup you get one line with the schedule:

```
level=INFO msg="autobotgo scheduler started" run_time=08:00 run_days="every day" timezone=America/New_York
```

Then nothing until `run_time`. Silence is correct. The scheduler wakes
every few seconds but does not log when there is nothing to do.

During a run each stage announces itself, and a finished run ends with the
title and the YouTube URL.

## Triggering a run right now

Create an empty file named `test-run` next to the config:

```sh
sudo -u autobotgo touch /opt/autobotgo/test-run
```

The program notices within a few seconds, deletes the file, and starts.
Use `sudo -u autobotgo` so the file belongs to the service account.

Two things to know:

- If today's video already finished, the trigger is ignored and says so.
  To genuinely redo a day, delete `/opt/autobotgo/runs/<date>/` first, and
  understand that you are paying for the whole thing again.
- This works even on a day excluded by `run_days`.

## When a run fails

Failed runs are **not** retried automatically until the next day. That is
deliberate. A run failing for a reason nobody has fixed would otherwise
retry in a loop and spend real money doing it.

To recover: fix the cause, then trigger a run with the `test-run` file.

The important part is that this is cheap. Every stage declares the files
it produces, and a stage whose files already exist is skipped. So a run
that died during `images` picks up at `images`. The story that was already
written and the narration that was already synthesized are not bought
again.

If you want to force a stage to redo work it thinks is done:

```sh
sudo -u autobotgo /opt/autobotgo/autobotgo run --stage images --force
```

### After downtime

If the machine was off at `run_time` and today's video was never made, the
scheduler notices at startup and runs immediately. So a reboot at 3pm on a
day it missed will produce that day's video right away, and it will cost
what it normally costs.

## Updating the binary

```sh
make deploy HOST=root@YOUR_IP           # DigitalOcean
make deploy HOST=ubuntu@YOUR_IP         # AWS
```

That builds, uploads, installs with the right ownership, and restarts the
service. Read the restart warning below first.

## Updating config.json or story.md

Config is read once at startup, so a change needs a restart to take
effect.

```sh
scp config.json autobotgo:config.json.new
ssh autobotgo 'sudo install -o autobotgo -g autobotgo -m 0600 config.json.new /opt/autobotgo/config.json && rm config.json.new && sudo systemctl restart autobotgo'
```

`story.md` is the same, but mode 0644.

> **Prefer editing the server's config in place over uploading yours.**
> The server's copy is the live master. It accumulates
> `youtube.refresh_token` and `world.vector_store_id`, and overwriting it
> with a stale local copy loses both.

> **Restarting during a render kills it.** The program installs no signal
> handlers, so `systemctl restart` stops the work immediately. Because a
> failed run is not retried until tomorrow, restarting at the wrong hour
> silently costs you that day's video.
>
> Restart right after a run completes, or restart and then trigger a
> `test-run`. The trigger resumes from where it stopped and re-uses every
> completed stage, so you pay nothing twice.

## Disk and logs

Log rotation is installed by the setup script, weekly with four rotations
kept, so `autobotgo.log` does not grow forever.

What grows without limit is `stories/` (a few KB per day, so a few MB per
year, not a problem) and `runs/` (small bookkeeping and logs per day).

The media directories `audio/`, `images/`, and `videos/` are emptied by
the cleanup stage after every successful upload. If you find large files
sitting there, a run failed before cleanup and it is safe to delete them
by hand.

Check space with:

```sh
ssh autobotgo 'df -h /; du -sh /opt/autobotgo/*'
```

## What to back up

Only three things cannot be regenerated:

| Path | Why it matters |
|---|---|
| `config.json` | Your keys, your refresh token, your vector store id |
| `stories/` | Every story ever written, and the source of truth for the search archive |
| `world.md` | The canon your whole series is built on |

Everything else is either regenerable or already on YouTube.

```sh
scp -r autobotgo:/opt/autobotgo/stories \
       autobotgo:/opt/autobotgo/world.md \
       autobotgo:/opt/autobotgo/config.json ./backup/
```

On AWS an EBS snapshot covers all three at once. On DigitalOcean, weekly
droplet backups do.

## Editing the world

`world.md` is meant to be edited. It is the canon every future story is
written against, so correcting it steers everything that comes next:

```sh
ssh autobotgo
sudo -u autobotgo vim /opt/autobotgo/world.md
```

No restart needed. It is read fresh at the start of every run.

If a character has outstayed their welcome, remove them. If the model
invented a fact you dislike, correct it. If a thread is finished, retire
it.

## Rotating the YouTube token

Uploads failing with an auth error usually means the token expired, which
usually means the OAuth consent screen slipped back to "Testing". See
[youtube-oauth.md](youtube-oauth.md#when-uploads-start-failing).

Run `autobotgo auth` on your own machine, then copy the updated config up
and restart.

## Turning it off

```sh
sudo systemctl stop autobotgo      # until the next reboot
sudo systemctl disable --now autobotgo   # for good
```

Nothing is deleted either way. Re-enable with
`sudo systemctl enable --now autobotgo`.

## Troubleshooting

**The service restarts in a loop.** Check `journalctl -u autobotgo -n 50`.
Most often the config failed validation or a required credential is
missing, and the error names the exact field. Note that the scheduler
demands every credential at startup, on purpose, so misconfiguration
surfaces immediately rather than at tomorrow's run time.

**"Exec format error".** The binary is built for the wrong CPU
architecture. Build with `make build-linux-arm64` for ARM servers,
`make build-linux-amd64` otherwise.

**The render is killed partway through.** Out of memory. Confirm swap
exists with `swapon --show`. If it does and renders still die, move up one
instance size.

**A model does not exist.** The default config names recent OpenAI models
that not every account can reach. Check which models your key can use and
update `openai.text_model`, `openai.image_model`, and
`openai.tts_model`.

**Images keep getting rejected.** Content filters can refuse an
illustration prompt. AutoBotGo already retries once with a sanitized
rewrite of the same scene. If it happens constantly, your `story.md` brief
is probably steering toward imagery the filters dislike, and softening the
brief fixes it at the source.

**The video is made but the upload fails.** The run stops before cleanup,
so the finished MP4 is still in `videos/`. Fix the auth problem and
trigger a `test-run`; it resumes at the upload stage and does not rebuild
anything.

**It ran at the wrong time.** `run_time` is interpreted in
`timezone` from the config, not the server's clock. Check both with
`journalctl -u autobotgo | grep "scheduler started"`.

For problems that look like bugs rather than configuration, see
[architecture-notes.md](architecture-notes.md).
