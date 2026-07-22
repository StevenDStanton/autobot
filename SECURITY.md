# Security

## Your config file holds live credentials

`config.json` contains, in plain text:

- Your OpenAI API key
- Your YouTube OAuth client secret
- Your YouTube refresh token, which grants upload access to your channel
- Your AWS or S3 credentials, if you set them

Treat it the way you would treat a private key.

- It is in `.gitignore`. Keep it there. **Never commit it.**
- The setup script installs it mode 0600 owned by the service account.
- `config.example.json` is the committed template and contains only
  placeholders. Check that you are looking at the right file before
  pasting anything anywhere.

If you think it has leaked, rotate all of it: a new OpenAI key, a new
OAuth client secret in Google Cloud Console, and a fresh refresh token
from `autobotgo auth`. Revoking the app at
[myaccount.google.com/permissions](https://myaccount.google.com/permissions)
invalidates the old refresh token.

## Reducing what a leak costs you

**If you enable the S3 archive, give its credentials write access to that
one bucket and nothing else.** The archive only ever uploads, so it never
needs permission to read, list, or delete.

**Keep the server's attack surface small.** Nothing in AutoBotGo listens
for incoming connections in normal operation. Only SSH needs an inbound
rule, and it should be limited to your own address. The one time
`autobotgo auth` opens a local port, that port binds to `127.0.0.1` and is
why authorization is meant to run on your own machine.

## Reporting a vulnerability

Email steven@thesimpledev.com rather than opening a public issue.

This is a personal project maintained in spare time, so please do not
expect a same day response. Include what you found, how to reproduce it,
and what an attacker could do with it.
