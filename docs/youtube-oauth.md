# Authorizing YouTube

This happens once, on a computer with a browser. Not on the server.

That is worth stating plainly because it shapes the whole deployment: the
authorization flow needs a browser and a callback to `127.0.0.1`. Doing it
locally means your server never needs an inbound port open beyond SSH.

At the end you have a refresh token sitting in `config.json`, and you copy
that file to the server.

## 1. Create a Google Cloud project

1. Go to the [Google Cloud Console](https://console.cloud.google.com/).
2. Create a project, or pick an existing one.
3. Under **APIs and Services > Library**, find **YouTube Data API v3** and
   enable it.

## 2. Configure the consent screen

Under **APIs and Services > OAuth consent screen**:

1. Choose **External** unless you have a Google Workspace organization.
2. Fill in the app name and your email. Nothing here is user facing for
   private use.
3. Add your own Google account under **Test users**.

Then the step people skip:

> **Set the publishing status to "In production".**
>
> Left in "Testing", Google expires your refresh token every 7 days.
> Uploads then start failing once a week, quietly, and the error does not
> obviously point back here.

You do not need to complete Google's verification review for private use.
You will see an "unverified app" warning during authorization. Click
through it. That is expected.

## 3. Create the OAuth client

Under **APIs and Services > Credentials**:

1. **Create Credentials > OAuth client ID**.
2. Application type: **Desktop app**. This matters. Other types use a
   different redirect scheme and will not work here.
3. Copy the **client ID** and **client secret** into `config.json`:

```json
"youtube": {
  "client_id": "1234567890-abcdef.apps.googleusercontent.com",
  "client_secret": "GOCSPX-your-secret-here",
  "refresh_token": "************************"
}
```

Leave `refresh_token` as the placeholder. The next step fills it in.

## 4. Run the authorization

```sh
./bin/autobotgo auth
```

It prints a short local address like `http://127.0.0.1:41234/` and tries
to open your browser. The address is short on purpose: Google's real
authorization URL is long enough that copying it out of a terminal
reliably loses characters to line wrapping, so AutoBotGo serves a short
one that redirects.

In the browser: pick your account, click through the unverified app
warning, and grant access.

When it succeeds, `youtube.refresh_token` is written into `config.json`
and the tab says so.

## 5. Verify

```sh
./bin/autobotgo run --stage story,tts,images,video,metadata,upload
```

That writes a real story, narrates it, illustrates it, renders it, and
uploads it privately. It costs real money, so if you would rather test
cheaply first, set `"target_minutes": 1` and `"image_quality": "low"`
before running it, then restore your settings.

Check YouTube Studio. The video should be there, private.

## Why uploads are private

Apps that have not passed YouTube's audit can only upload private videos.
AutoBotGo uploads as private anyway, so this limit costs you nothing.
Review each video in YouTube Studio and publish it yourself.

If you want AutoBotGo to publish directly you would need to pass that
audit and set `youtube.privacy_status` to `"public"`. Given the whole
point is reviewing what a model wrote before the world sees it, private is
the better default.

## When uploads start failing

Almost always one of two things:

**The consent screen slipped back to "Testing", or was never moved to "In
production".** Tokens then die every 7 days. Fix the setting, then mint a
new token:

```sh
./bin/autobotgo auth
```

Copy the updated `config.json` to your server and restart the service.

**The app's access was revoked.** If `autobotgo auth` reports that no
refresh token came back, Google is reusing an existing grant. Remove the
app at [myaccount.google.com/permissions](https://myaccount.google.com/permissions)
and run it again.
