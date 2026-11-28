# 0151. The server offers the 3D client, and a setup file that carries no session

Status: accepted (2026-11-26), for #309. It builds on [ADR 0138](0138-a-native-client-signs-in-with-a-session-token.md)
(a native client's session), [0067](0067-principals-and-guest-keys.md) (guest keys),
[0065](0065-the-connection-must-be-secure.md) (the certificate a server makes for itself) and
[0110](0110-branding.md) (an instance's own name).

## Context

[bingle-3d](https://github.com/noderunner/bingle-3d) is released on its own, as three archives whose names carry no
version. Somebody who plays on a server had to find those releases, and then type the server's address and sign in
again in the client. The maintainer asked that a server hand out the client itself, from a menu on its logo, with a
config file made for that server and for the player who asks.

Three things in that needed deciding: where builds come from and which of them a server offers, what the logo
becomes, and what a file that lands in a downloads folder may carry.

## Decision

- **The operator keeps the builds; the server reads a directory.** `BINGLE_CLIENT_BUILDS` names it, with a folder
  for each version holding that release's archives unrenamed: the version is the folder's name, since the archives
  do not say it. It is read once at start, each archive on offer hashed then and sent from the disk as a pack's
  client files are (its hash a strong ETag, ranges served). The server fetches nothing, and never looks inside an
  archive.
- **For each platform, the newest version that has it and that can use this server.** A release may come with a
  `release.json` naming the API it was made for; such a release is offered only by a server of the same major
  version whose minor has reached it, which is the client's own rule. A release without one is offered as it is:
  the server cannot know, and the client's light on its login screen says so. This needs bingle-3d's releases to
  publish that file.
- **Unset, nothing changes.** The logo of a server whose operator has nothing to do with bingle-3d stays the link
  home, and the new calls are not there. An operator who wants the setup file without builds names an empty
  directory.
- **The logo opens the menu, and the name beside it stays the link home.** The menu's first item is *Home* too,
  because the phone layout and a game fitted to the screen show the logo alone. It is written into the page by
  the server, so no page asks what is on offer.
- **The menu names no product.** Its heading is *Download Game Client*, and its items are each platform's icon and version and *<name> Profile Configs*. So
  a branded instance needs no word of its own for the download, and the files keep `bingle-3d` in their names as
  the code keeps `BINGLE_*`.
- **The setup file never carries a session. For an account it carries a sign-in code: one use, 24 hours.** A
  session token is the whole account, an administrator's included, for up to 90 days, and it would stay live in
  the downloaded file after the client had copied it. A code that is spent or a day old is worth nothing. The
  client spends it with `POST /auth/login {code}`, which answers as ADR 0138's sign-in does, so the session is
  made by the client's own request: it is listed under the client's name and ended on its own. The code is a row
  of `account_tokens`, kept by its hash like the mailed links, and no second factor is asked for when it is spent,
  since the session that asked for it had given one.
- **Asking for the file is behind a sign-in's gates.** It is a `POST` from the page (`X-Bingle-Session`), never
  cached, refused to an API token, and outside `/auth`, so the middleware refuses an account whose password must
  be changed and an administrator with no second factor yet. Whatever ends an account's other sessions (a new
  password, a reset, *sign out everywhere else*) ends its unspent codes. An account is given ten an hour.
- **A guest's file carries the guest's key.** A guest is nothing but its key, and the browser already holds it.
  The browser and the client are then one guest, so the client sits in the seats the browser took.
- **`server` is the address the request came to**, not `BINGLE_PUBLIC_URL`: it is the one known to work from the
  machine the file is downloaded to.
- **The file carries the fingerprint of a certificate the server made for itself** (`BINGLE_TLS_SELFSIGNED`), and
  only over a connection that certificate made. The player has already accepted it in the browser the file came
  through, so the client need not ask again. With certificate files, automatic certificates or a proxy there is
  nothing to carry: no pin is needed, or the server does not know the certificate.
- **One file, with a format number**, which the client reads once, splits into its own two files and deletes.
  Where the client looks for it is bingle-3d's to say.

## Considered

- *A session made for the download, written into the file* (the issue's first suggestion): no new call and
  nothing for the client to exchange, but a live credential for the whole account in a downloads folder for as
  long as the client keeps using it.
- *A choice between a file with and without the sign-in*: one more item in a menu, for a risk the code already
  takes away.
- *The server fetches releases itself*: a server that reaches out to GitHub, with a token or its rate limits, to
  save an operator a copy.
- *The archives' hashes in a manifest, checked at start*: the server would refuse a damaged copy, but it hashes
  each archive anyway and reports the hash, so a client or a person can check it against the release.
- *The whole logo and name open the menu*: one more click to go home from every page, on the control everybody
  expects to be the way home.
- *A menu whenever the server runs*: every server's logo would stop being a link for the sake of a setup file most
  of them have no use for.

## Consequences

- `version.API` 1.18.0: `GET /api/client`, `GET /api/client/files/{file}`, `POST /api/client/setup`, and `code` in
  `POST /auth/login`, whose `handle` and `password` the schema no longer requires.
- bingle-3d has to learn to take the file and to publish `release.json`; until it does, a server offers its
  builds, and the setup file is of use only to somebody who copies its values by hand.
- A link that opens an installed client (bingle-3d #77) could carry the same code later, with no file at all.
- An operator restarts the server after adding a version.

## Not decided here

Whether a server should say, in a game's page, that a 3D client exists for it; and anything about updating a
client that is already installed.
