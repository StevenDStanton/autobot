# Deploy to DigitalOcean

Setting up AutoBotGo on a droplet running Ubuntu 24.04.

The install steps are the same as the [AWS guide](deploy-aws.md). Only
picking the machine and connecting to it differ, so this page covers those
and then points at the shared steps.

## What you are building

One droplet running AutoBotGo as a systemd service. The service stays up
and fires the pipeline once a day on its own.

Outbound HTTPS only. Inbound is SSH from your address and nothing else.

## Before you start

Do the local setup in the [README](../README.md) first. Before you
provision anything you should have:

- A filled in `config.json`, including a real `youtube.refresh_token`.
- A `story.md` brief you are happy with.
- At least one successful `./bin/autobotgo run --dry-run`.

> **Do not run `autobotgo auth` on the droplet.** It needs a browser and a
> callback to `127.0.0.1`. Run it on your own machine and copy
> `config.json` up.

## Droplet size

**The $6 a month Basic droplet is enough**: 1 GB RAM, 1 vCPU, 25 GB SSD.

The $4 512 MB droplet is not, even with swap. ffmpeg peaks near 500 MB
during the encode and the machine would thrash through the whole render.

On 1 vCPU a five minute video takes around 1 to 1.5 hours. That is normal
for a once a day job. The $12 2 GB droplet removes any dependence on swap
if you would rather not think about it.

Basic droplets are x86_64, so `make build-linux-amd64` is always the right
binary.

## Choose the image

**Ubuntu 24.04 (LTS) x64**. The `doctl` slug is `ubuntu-24-04-x64`.

## Connect

Add your SSH public key in the control panel **when you create the
droplet**. Adding it afterwards means going through the emailed root
password and a forced password change.

```sh
ssh root@YOUR_DROPLET_IP
```

The login user is `root`, which is the main difference from AWS. Every
command in the [AWS install steps](deploy-aws.md#install) still works, and
you can drop the `sudo` prefix if you like.

Worth adding to `~/.ssh/config`:

```
Host autobotgo
    HostName YOUR_DROPLET_IP
    User root
```

## Firewall

This matters more here than on AWS. A new droplet has a public IP and **no
firewall at all**, where an EC2 instance always has a security group. So
this is a step you have to take rather than one you inherit.

Create a Cloud Firewall (free) under **Networking > Firewalls**:

| Direction | Port | Source |
|---|---|---|
| Inbound | 22 (SSH) | your IP only |
| Inbound | everything else | denied |
| Outbound | all | anywhere |

Attach it to the droplet. DigitalOcean does not enable `ufw` on this
image, so if you would rather use a host firewall you have to turn it on
yourself.

## Install

Follow [the install steps in the AWS guide](deploy-aws.md#install). They
are identical: ffmpeg, the swap file, the service account, uploading the
binary and config, the systemd unit, and log rotation.

Two small differences:

- You are `root`, so `sudo` is optional.
- `scp` straight to a path works, since root can write anywhere. Copying
  to the home directory first is still fine.

Then verify and start the service exactly as described under
[Verify before spending money](deploy-aws.md#verify-before-spending-money),
including the warning about the first run costing real money if
`run_time` has already passed.

On a 1 GB droplet the free dry run matters more than anywhere else. It is
what proves the swap file is doing its job before a paid render depends on
it.

## Optional: archive runs to storage

Off by default. Three options, in the order most people should consider
them.

### Leave it off

The right answer for most people. The only irreplaceable state is
`stories/` and `world.md`. The media is regenerable and the finished video
is already on YouTube.

Copying those two paths periodically covers the real risk:

```sh
scp -r autobotgo:/opt/autobotgo/stories autobotgo:/opt/autobotgo/world.md ./backup/
```

DigitalOcean's weekly droplet backups (about $1.20 a month) also cover it.

### Use AWS S3

Works with no extra setup beyond an S3 bucket and an IAM user. See the
[AWS guide's archive section](deploy-aws.md#optional-archive-runs-to-s3).
Under a dollar a month for a few GB.

### Use DigitalOcean Spaces

Supported through `aws.endpoint`:

```json
"aws": {
  "enabled": true,
  "region": "nyc3",
  "bucket": "your-space-name",
  "prefix": "autobotgo/",
  "endpoint": "https://nyc3.digitaloceanspaces.com",
  "access_key_id": "your-spaces-key",
  "secret_access_key": "your-spaces-secret"
}
```

Generate the key pair under **API > Spaces Keys**.

Note that `region` carries the Spaces region (`nyc3`, not an AWS region
name), because the endpoint is built around it. Setting an endpoint also
makes AutoBotGo stop sending the request checksums that many S3 compatible
services reject, so the same approach works for MinIO, Backblaze B2, and
Cloudflare R2.

Check the price first. Spaces starts at $5 a month for a 250 GB minimum,
where the same few GB on S3 costs under a dollar.

## Check it worked

- `systemctl status autobotgo` shows `active (running)`.
- `swapon --show` lists `/swapfile`.
- `journalctl -u autobotgo` shows the startup line with your run time.
- A test run produces a private video in YouTube Studio:

```sh
sudo -u autobotgo touch /opt/autobotgo/test-run
```

Everything after this is in [operations.md](operations.md).

## DigitalOcean specific notes

**The public IP is stable** across reboots and power cycles, unlike AWS.

**Locked out of SSH?** The control panel has a Recovery Console that does
not need the network path.

**Resizing.** CPU and RAM only is reversible. Disk resize is permanent, so
think before growing the disk.

**Backups.** Weekly backups under the Backups tab, about 20% of the
droplet price.
