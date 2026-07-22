# Deploy to AWS

Setting up AutoBotGo on an EC2 instance running Ubuntu 24.04.

The same steps work on any Ubuntu server. Nothing here is specific to AWS
except choosing the instance and connecting to it.

## What you are building

One EC2 instance running AutoBotGo as a systemd service. The service stays
up and fires the pipeline once a day on its own.

Outbound HTTPS only. Inbound is SSH from your address and nothing else,
because nothing listens for incoming traffic.

## Before you start

Do the local setup in the [README](../README.md) first. Before you
provision anything you should have:

- A filled in `config.json`, including a real `youtube.refresh_token`.
- A `story.md` brief you are happy with.
- At least one successful `./bin/autobotgo run --dry-run`.

> **Do not run `autobotgo auth` on the server.** It needs a browser and a
> callback to `127.0.0.1`. Run it on your own machine and copy
> `config.json` up. This is why the server never needs an inbound port
> beyond SSH.

## Instance size

**A `t3.micro` is enough**: 2 vCPU, 1 GB RAM, x86_64, so it takes the same
`amd64` binary as any other server.

ffmpeg peaks around 500 MB during the encode, so 1 GB works with a swap
file as the safety margin. Creating that swap file is a step below, and it
is not optional on a 1 GB instance.

A five minute video takes roughly 1 to 1.5 hours to render on one CPU.
That is expected for a batch job that runs once a day.

`t3` instances are burstable, which matters if you test heavily. A
`t3.micro` earns about 288 CPU credits a day and one render spends roughly
90 to 180 of them. One render a day is sustainable. Several test renders
back to back will drain the balance and slow everything down until it
recovers.

If you want ARM, `t4g.micro` is cheaper. Build with `make
build-linux-arm64` instead. Nothing else changes.

## Choose the AMI

Ubuntu 24.04 LTS, published by Canonical. In the console this is **Launch
instance > Quick Start > Ubuntu**, then 24.04 LTS.

Give the root volume 20 GB rather than the 8 GB default. Renders need room
for intermediates, and stories accumulate.

## Connect

Create an ED25519 key pair at launch and download the `.pem`:

```sh
chmod 400 ~/Downloads/autobotgo.pem
ssh -i ~/Downloads/autobotgo.pem ubuntu@YOUR_INSTANCE_IP
```

The login user is `ubuntu`, not `root` and not `ec2-user`. A "Permission
denied (publickey)" on the first connection is usually the wrong username.

Adding this to `~/.ssh/config` makes every later command shorter:

```
Host autobotgo
    HostName YOUR_INSTANCE_IP
    User ubuntu
    IdentityFile ~/Downloads/autobotgo.pem
```

## Security group

| Direction | Port | Source |
|---|---|---|
| Inbound | 22 (SSH) | your IP only |
| Inbound | everything else | denied |
| Outbound | all | anywhere |

Choose "My IP" rather than `0.0.0.0/0` when creating the SSH rule. Do not
open 80 or 443. Nothing listens on them.

## Install

### 1. Install ffmpeg

```sh
sudo apt-get update
sudo apt-get install -y ffmpeg ca-certificates
```

Check both tools are present. The pipeline calls each of them, and a
missing `ffprobe` shows up later as a confusing mid-render failure:

```sh
ffmpeg -version && ffprobe -version
```

### 2. Create the swap file

On a 1 GB instance this is what keeps the encode from being killed:

```sh
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

Make the kernel treat it as a safety net rather than somewhere to live:

```sh
echo 'vm.swappiness=10' | sudo tee /etc/sysctl.d/99-autobotgo.conf
sudo sysctl -w vm.swappiness=10
```

Confirm with `swapon --show`.

### 3. Create the service account and directory

```sh
sudo useradd --system --home-dir /opt/autobotgo --shell /usr/sbin/nologin autobotgo
sudo install -d -o autobotgo -g autobotgo -m 0750 /opt/autobotgo /opt/autobotgo/runs
```

### 4. Upload the binary and your files

On your own machine:

```sh
make build-linux-amd64
scp bin/autobotgo-linux-amd64 config.json story.md autobotgo:
```

Copy to the home directory, not straight to `/opt`, because `ubuntu`
cannot write there. Then on the server:

```sh
sudo install -o autobotgo -g autobotgo -m 0755 autobotgo-linux-amd64 /opt/autobotgo/autobotgo
sudo install -o autobotgo -g autobotgo -m 0600 config.json /opt/autobotgo/config.json
sudo install -o autobotgo -g autobotgo -m 0644 story.md /opt/autobotgo/story.md
rm autobotgo-linux-amd64 config.json story.md
```

`config.json` is mode 0600 because it holds your API key and refresh
token.

### 5. Create the systemd service

```sh
sudo tee /etc/systemd/system/autobotgo.service > /dev/null <<'EOF'
[Unit]
Description=AutoBotGo daily story video pipeline
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=autobotgo
WorkingDirectory=/opt/autobotgo
ExecStart=/opt/autobotgo/autobotgo
Restart=always
RestartSec=30

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
```

`Restart=always` matters. A crash must not end the daily schedule, and
because completed stages are skipped on resume, a restart does not re-buy
work that already succeeded.

Do not add a `MemoryMax`. The whole point of the swap file is letting
ffmpeg exceed RAM briefly.

### 6. Rotate the log

The program opens its log once and holds it open, so rotation has to use
`copytruncate`. Without it the log grows forever:

```sh
sudo tee /etc/logrotate.d/autobotgo > /dev/null <<'EOF'
/opt/autobotgo/autobotgo.log {
    weekly
    rotate 4
    compress
    delaycompress
    copytruncate
    missingok
    notifempty
}
EOF
```

## Verify before spending money

Do a free render first. This proves ffmpeg, the swap file, and the CPU are
all fine on this instance before any paid run happens:

```sh
sudo -u autobotgo /opt/autobotgo/autobotgo run --dry-run
```

Real ffmpeg, no API calls. Time it, and watch memory from a second session
with `free -m` to see the swap absorb the peak.

Then start the service:

```sh
sudo systemctl enable --now autobotgo
```

> **Money warning.** The moment the service starts, if today's `run_time`
> has already passed and no video exists for today, it runs the full paid
> pipeline immediately. That catch up behavior is deliberate, since it
> recovers a day after downtime, but it surprises people on a first
> deploy.
>
> Either start the service before today's `run_time`, or deploy with
> `"target_minutes": 1` and `"image_quality": "low"` so the first
> automatic run is a cheap end to end test, then restore your settings.

Watch it:

```sh
journalctl -u autobotgo -f
```

A healthy idle service logs its startup line with the schedule and then
stays quiet until `run_time`.

## Optional: archive runs to S3

Off by default. Skip this unless you want it. The only irreplaceable state
is `stories/`, `world.md`, and `config.json`, and an EBS snapshot covers
all three.

Create a private bucket, then an IAM user with permission to write to it,
and put its access key in the config:

```json
"aws": {
  "enabled": true,
  "region": "us-east-1",
  "bucket": "your-bucket-name",
  "prefix": "autobotgo/",
  "endpoint": "",
  "access_key_id": "AKIA...",
  "secret_access_key": "..."
}
```

All four of `bucket`, `region`, `access_key_id`, and `secret_access_key`
are required when `enabled` is true.

The archive adds roughly 200 MB a day and never shrinks on its own, so a
lifecycle rule expiring objects after 90 days is worth setting unless you
want to keep every render forever.

## Check it worked

- `systemctl status autobotgo` shows `active (running)`.
- `swapon --show` lists `/swapfile`.
- `journalctl -u autobotgo` shows the startup line with your run time.
- A test run produces a private video in YouTube Studio:

```sh
sudo -u autobotgo touch /opt/autobotgo/test-run
```

Everything after this is in [operations.md](operations.md).

## AWS specific notes

**Stopping the instance changes its public IP.** Attach an Elastic IP if
you need it stable.

**Set a Budget alert** in Billing a few dollars above your expected total.

**Resizing.** Stop the instance, change the instance type, start it again.
`t3.small` doubles the RAM if renders are too slow.

**Backups.** Snapshot the EBS volume. That captures `stories/`,
`world.md`, and `config.json` together.
