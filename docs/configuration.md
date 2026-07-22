# Configuration

Everything AutoBotGo needs lives in one file, `config.json`, next to the
binary. `autobotgo init` writes a template with every field present and
`************************` marking the values only you can supply.
[config.example.json](../config.example.json) in the repo is the same
template, committed so you can read it without running anything.

The file holds your API key and your YouTube refresh token, so it is mode
0600 and must never be committed. It is already in `.gitignore`.

Unknown keys are rejected. A typo in a field name fails at startup rather
than being silently ignored, so you never end up believing a setting is
applied when it is not.

Relative paths are resolved against the directory holding `config.json`.

## Schedule

| Key | Default | What it does |
|---|---|---|
| `run_time` | `"08:00"` | Daily start time, 24 hour `HH:MM`. |
| `timezone` | `"America/New_York"` | IANA timezone name for `run_time`. The server's own timezone is ignored, so moving the server never moves your publish time. |
| `run_days` | `null` | Weekdays to run on, lowercase, for example `["monday","wednesday","friday"]`. Empty or absent means every day. |
| `runs_dir` | `"runs"` | Where per run bookkeeping goes. |
| `log_file` | `"autobotgo.log"` | Log file path. Set to `""` to log only to the journal. |
| `notify_url` | `""` | A URL to POST a one line status to after every run. An [ntfy.sh](https://ntfy.sh) topic gets you a phone notification. |

The `test-run` trigger file works regardless of `run_days`.

## OpenAI

| Key | Default | What it does |
|---|---|---|
| `openai.api_key` | placeholder | Your API key. |
| `openai.text_model` | `"gpt-5.4"` | Handles the utility calls: scene prompts, metadata, world updates, story audits. |
| `openai.story_model` | `""` | Writes and revises the story itself. Usually a slower reasoning model. Empty means use `text_model`. |
| `openai.tts_model` | `"gpt-4o-mini-tts"` | Text to speech model. |
| `openai.tts_voice` | `"onyx"` | Narration voice. |
| `openai.tts_instructions` | see template | Plain English delivery direction, for example "Warm, unhurried storyteller. Slight pauses between paragraphs." |
| `openai.tts_max_chunk_tokens` | `1500` | How much text goes to the speech API at once. The API limit is in characters, so this is converted conservatively and clamped. |
| `openai.image_model` | `"gpt-image-1.5"` | Image model. |
| `openai.image_size` | `"1536x1024"` | Generated image size. Wider than the 16:9 output on purpose, so the pan and zoom has room to move. |
| `openai.image_quality` | `"high"` | Set to `"low"` while testing. This is the single biggest lever on cost. |
| `openai.image_style_prompt` | see template | Appended to every image prompt, which is what keeps the look consistent across a video. |

Model names change. If a run fails saying the model does not exist, check
which models your account can reach and set these to something current.

## Story

| Key | Default | What it does |
|---|---|---|
| `story.prompt_file` | `"story.md"` | Your brief: theme, voice, rules, tone. |
| `story.target_minutes` | `5` | Target length of the finished video. |
| `story.target_words_min` | `750` | Lower word bound given to the model. |
| `story.target_words_max` | `850` | Upper word bound. |

Word counts drive length more reliably than asking for minutes, so both
are given. Roughly 150 words per narrated minute.

## World

The world is what makes the videos a series instead of a pile of
unrelated stories.

| Key | Default | What it does |
|---|---|---|
| `world.max_words` | `4000` | Cap on `world.md`. When an update goes over, the model compresses the canon back to about 70% of the cap, keeping core facts and dropping episode detail. |
| `world.rag_enabled` | `true` | Uploads each story to an OpenAI vector store so story generation can search the whole archive, not just the canon summary. |
| `world.vector_store_id` | `""` | Created automatically on the first run and written back into this file. Leave it empty. |

Your local `stories/` folder is the source of truth. If the vector store
is ever lost, clear the id and let a new one be built from those files.

## Video

| Key | Default | What it does |
|---|---|---|
| `video.images_per_minute` | `2.0` | How often the picture changes. |
| `video.width` / `video.height` | `1920` / `1080` | Output size. |
| `video.fps` | `30` | Frame rate. |
| `video.crossfade_seconds` | `1.0` | Blend length between images. `0` disables crossfades. |
| `video.kenburns_max_zoom` | `1.12` | How far the slow zoom travels. `1.0` disables the motion. |
| `video.end_hold_seconds` | `2.0` | Silent hold on the last image. |
| `video.x264_preset` | `"medium"` | Encoder speed against file size. |
| `video.x264_crf` | `18` | Quality, lower is better. 18 is visually lossless. |
| `video.background_music_path` | `""` | Optional music file. It loops, sits under the narration at `music_volume`, and fades out at the end. |
| `video.music_volume` | `0.12` | Music level, 0 to 1. |
| `video.clips` | disabled | Reserved for mixing in generated video clips later. Not implemented. Turning it on is a config error. |

## YouTube

| Key | Default | What it does |
|---|---|---|
| `youtube.enabled` | `true` | Set false to build videos without uploading. |
| `youtube.client_id` | placeholder | From your OAuth desktop client. |
| `youtube.client_secret` | placeholder | From your OAuth desktop client. |
| `youtube.refresh_token` | placeholder | Written by `autobotgo auth`. Do not fill this in by hand. |
| `youtube.privacy_status` | `"private"` | `private`, `unlisted`, or `public`. |
| `youtube.category_id` | `"22"` | YouTube category. 22 is People and Blogs. |
| `youtube.made_for_kids` | `false` | The "made for kids" declaration. |
| `youtube.extra_tags` | `[]` | Tags added to every video on top of the generated ones. |
| `youtube.playlist_id` | `""` | Optional playlist to add each video to. |

Setting up these credentials is covered in
[youtube-oauth.md](youtube-oauth.md).

Uploads always declare synthetic media to YouTube, and the AI disclosure
is always appended to the description. Neither is configurable.

## AWS and S3 compatible storage

The archive stage is optional and off by default. It tars each run and
uploads it.

| Key | Default | What it does |
|---|---|---|
| `aws.enabled` | `false` | Turn the archive stage on. |
| `aws.region` | `"us-east-1"` | AWS region, or the region of your S3 compatible provider. |
| `aws.bucket` | `""` | Bucket name. Required when enabled. |
| `aws.prefix` | `"autobotgo/"` | Key prefix inside the bucket. |
| `aws.endpoint` | `""` | Custom S3 endpoint. Leave empty for real AWS. |
| `aws.access_key_id` | `""` | Access key. Required when enabled. |
| `aws.secret_access_key` | `""` | Secret key. Required when enabled. |

When `aws.enabled` is true, all four of `bucket`, `region`,
`access_key_id`, and `secret_access_key` are required.

To use something other than AWS, set `aws.endpoint` to your provider's S3
endpoint, for example `https://nyc3.digitaloceanspaces.com`. Setting an
endpoint also stops AutoBotGo sending the request checksums that many S3
compatible services reject, which is what makes DigitalOcean Spaces,
MinIO, Backblaze B2, and Cloudflare R2 work.

In that case `aws.region` has to carry the region your endpoint expects
(`nyc3`, not `us-east-1`).

## Changing settings on a running server

Config is read once at startup, so any change needs a restart:

```sh
sudo systemctl restart autobotgo
```

Be careful about when you restart. See the warning in
[operations.md](operations.md#updating-configjson-or-storymd).
