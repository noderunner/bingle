# Offering the 3D client

A server can hand out [bingle-3d](https://github.com/noderunner/bingle-3d), the native 3D client, to its own
players: the builds its operator keeps on its disk, and a setup file that connects a client to this server as the
player who asked for it. [ADR 0151](decisions/0151-the-server-offers-the-3d-client-and-a-setup-file.md) says why
it is built as it is.

This is the client itself. A game's own files for a client (its 3D look) come from its install pack and are
another thing: see [packs.md](packs.md#clients).

## The builds

Name a directory in `BINGLE_CLIENT_BUILDS`. It holds a folder for each version, named for it, with that release's
archives as they were published:

```
BINGLE_CLIENT_BUILDS=/srv/bingle/client ./bin/bingle

/srv/bingle/client/
  0.0.2/
    bingle-3d-windows-x86_64.zip
    bingle-3d-linux-x86_64.tar.gz
    bingle-3d-steam-frame.tar.gz
  0.0.3/
    bingle-3d-windows-x86_64.zip
    bingle-3d-linux-x86_64.tar.gz
    release.json
```

- A folder is named `0.0.3`, or `v0.0.3` as the release is tagged. An archive is named
  `bingle-3d-<platform>.zip` or `.tar.gz`, and its platform is read from that name. Anything else in the directory
  is left alone, and a folder that is not named for a version is logged at start.
- **For each platform the server offers the newest version that has an archive for it.** Above, Windows and Linux
  are 0.0.3 and Steam Frame is 0.0.2.
- **`release.json` says which API the builds were made for**: `{"version": "0.0.3", "api": "1.17.0"}`. A release
  that has one is offered only by a server it can use, by the client's own rule: the same major version of the API
  (`GET /api`'s `api_version`), and a minor the server has reached. A release without one is offered as it is, and
  the client's own light on its login screen is the check. A `release.json` that cannot be read, or that names
  another version than its folder, leaves the folder out.
- The directory is read once, at start: each archive on offer is hashed then, and sent from the disk afterwards.
  The server fetches nothing and never looks inside an archive. An archive may be up to 1 GiB, must be a plain
  file (a link is not followed), and is not sent if it was written to after the server started: restart the server
  after changing the directory.
- A directory that cannot be read stops the start. An empty one offers no builds, and still offers the setup file.

Unset, `BINGLE_CLIENT_BUILDS` changes nothing: the logo stays the link home it has always been, and the calls below
answer `404`.

## What a player sees

The logo in the top bar opens a menu: *Home*, then under *Download Game Client* a button for each platform (its icon
and version), *<name> Profile Configs* (the setup file) and *Client Quick Setup Guide*, which opens the page at
`/client`: numbered steps for Windows and Linux (download, unpack, profile configs, put the file beside the program,
run), the platform of the browser first. The page is there only while the server offers builds, and its path is
stable, so an announcement can link to it. The name beside the logo stays the link home; the phone layout and a game fitted to the screen show the
logo alone, which is why the menu starts with *Home*. The menu names neither Bingle nor an instance's own name
([ADR 0110](decisions/0110-branding.md)), and a platform is shown by its name: Windows, Linux, Steam Frame, or the
name in its archive's for one the page does not know.

## The setup file

`POST /api/client/setup` answers with `bingle-3d-setup.toml`, made for the server and for whoever asks:

```toml
# Setup for the bingle-3d client. Put this file beside the program, or start it with
# --setup <this file>. The client reads it once and then deletes it.
format = 1
server = "https://games.example.com"
name = "ana"
sign_in_code = "bgc_…"
```

| Key | When | What it is |
|---|---|---|
| `format` | always | `1`. A client refuses a file whose format it does not know |
| `server` | always | The address the request came to: the one known to work from that player's machine, not `BINGLE_PUBLIC_URL` |
| `name` | a signed-in account | The account's name |
| `sign_in_code` | a signed-in account | A code that signs a client in once: `POST /auth/login {"code": "bgc_…"}` answers with a session token as a native sign-in does (ADR 0138) |
| `guest_key` | a guest | The guest's own key (`bgg_…`). The browser and the client are then one guest, so the client sits in the browser's seats |
| `certificate_sha256` | `BINGLE_TLS_SELFSIGNED` | The fingerprint of the certificate the server made for itself, `AB:CD:…`, so the client's first run does not stop to ask about it |

Somebody who is neither signed in nor a guest gets `format` and `server` alone.

**The file never carries a session.** It lands in a downloads folder and stays there, so for an account it carries
a sign-in code instead:

- it works **once**, and for **24 hours**;
- the session it makes is the client's own: listed in the account's sessions under the client's name, and ended
  without ending the browser's;
- no second factor is asked for when it is spent: the session that asked for the file had given it;
- it is made by a `POST` from the page (`X-Bingle-Session`), never cached, refused to an API token, and refused to
  an account whose password must be changed and to an administrator who has no second factor yet, as every other
  thing a sign-in does is;
- a new password, a password reset and *sign out everywhere else* end the codes not yet spent;
- an account is given ten files an hour.

A guest's key is all a guest is, so the file carries it as it is. Whoever has the file is that guest, as whoever
has the browser is.

What the client does with the file (where it looks for it, and what it writes to its own `config.toml` and
`keys.toml`) is bingle-3d's, and is described there.

## The API

| Method & path | Response |
|---|---|
| `GET /api/client` | `{builds: [{platform, version, file, size, sha256, api?}]}`, in the order of their platforms |
| `GET /api/client/files/{file}` | one build's archive, sent from the disk. Its `sha256` is its strong `ETag`: `If-None-Match` answers `304`, and `Range` and `If-Range` are served. Anyone who can reach the server may fetch it |
| `POST /api/client/setup` | the setup file, `application/toml`, `Cache-Control: no-store` |
| `POST /auth/login` | with `{code}`: `{token, expires, me}`; `401` for a code that was spent, is too old or was never one |
