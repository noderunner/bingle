# Plan: email turn notifications through a personal Gmail account

Status: **implemented** (2026-10-25; see [email.md](email.md) and ADR 0050), with the
changes listed in the status log at the bottom: a generic SMTP server instead of Gmail
only, the sender is `BINGLE_TLS_EMAIL`, and addresses are "contacts" owned by a browser. The
text below is the original plan and its reasoning.

## Why, and the constraint that shapes everything

Players want an email when it is their turn in a game they have left. Bingle runs
on a home connection with dynamic DNS, so it cannot deliver mail itself (home IP
ranges are blocklisted, port 25 is often blocked, SPF and reverse DNS are
impossible). The chosen route is **authenticated submission**: Bingle logs in to
Gmail's SMTP server (`smtp.gmail.com:587`, STARTTLS) with an app password and
Gmail delivers the mail.

That makes the Gmail account the thing to protect. Bingle today has **no
accounts, no authentication, no TLS and has not been security-reviewed**. A player
is a name in a room, and the API trusts the `name` it is given. So the design
assumes every caller is hostile and that Bingle itself might be compromised.

## What exists today

- `internal/config` reads `BINGLE_*` environment variables (addr, db path, TLS
  files, public URL, retention).
- `autoplay_prefs` (`internal/store/schema.sql`) is the model for a per-game,
  per-seat preference, with `GET/PUT /games/{code}/autoplay` in
  `internal/api/router.go` and `internal/session/autoplay.go`. The email setting
  follows the same shape.
- `session.Manager` already knows when the turn passes to a seat (the Auto-Play
  Timeout checks "it is their turn to act"). That is the hook for sending.
- The game options window (`game.js`, `S.settings` and the options dialog) holds
  per-viewer settings. Most live in browser storage; Auto-Play Timeout is stored
  server-side and is the pattern to copy.
- There is no login: anyone who knows a room code and a player name can call
  `PUT /games/{code}/...` as that player. **This is the central problem for email.**

## Threats this plan must defeat

1. **Credential theft.** A remote client, a bug (path traversal, SQL injection,
   debug endpoint, MCP tool) or a leaked backup obtains the Gmail password.
2. **Open relay.** An attacker makes Bingle send arbitrary mail: arbitrary
   recipients, arbitrary subject or body, or high volume. Gmail would suspend the
   account for spam.
3. **Harassment by proxy.** An attacker enters a victim's address as the target
   and Bingle mails the victim repeatedly. Even with fixed content this is abuse
   of your account's reputation.
4. **Address disclosure.** Players' addresses leak through game views, the
   WebSocket, the MCP server, logs, exports or the `GET` of the preference.
5. **Hijacking someone else's setting.** With no authentication, anyone can
   overwrite or read another player's notification preference by guessing their
   name.
6. **Header injection.** A newline in a name, room or email address adds headers
   or recipients to the message.
7. **Account lockout or cost.** A flood of notifications exhausts Gmail's daily
   quota (about 500/day) and blocks real ones.

## Design

### 1. Where the credentials live

The Gmail address and app password are **server operator configuration, never
application data**.

- Not in the SQLite database, not in a game, not in a setting, not in any JSON
  the API can return, and not in the repository or the container image.
- Supplied through files, not plain environment variables (an environment can be
  read from `/proc/<pid>/environ`, `docker inspect`, crash dumps and
  `ps eww`; a file with tight permissions is narrower):
  - `BINGLE_SMTP_PASSWORD_FILE=/run/secrets/bingle_smtp_password` (preferred), a
    file readable only by the user Bingle runs as (`chmod 0400`). Under Podman,
    create it with `podman secret create bingle_smtp_password -` and mount it
    with `--secret`, which puts it in a tmpfs inside the container only.
  - `BINGLE_SMTP_HOST` (default `smtp.gmail.com`), `BINGLE_SMTP_PORT` (587),
    `BINGLE_SMTP_USER`, `BINGLE_SMTP_FROM`.
  - `BINGLE_SMTP_PASSWORD` (a plain variable) is **not supported**, to make the
    weaker option unavailable.
- `config.Load` reads the file once at startup into a `config.Secret` type that
  holds the string privately and whose `String()`, `GoString()`, `MarshalJSON()`
  and `LogValue()` all return `"[redacted]"`. It is passed only to the mail
  sender, never stored in `Config` fields that get printed, and never logged. A
  test fails if the password text appears in the formatted config.
- The file's mode is checked at startup: refuse to start the mailer if it is
  readable by group or others.
- **Use a dedicated Google account, not your personal one.** Create a throwaway
  Gmail used only for Bingle, turn on 2-Step Verification, and generate an **app
  password** for it (Google Account → Security → App passwords). An app password
  works only for mail, can be revoked on its own, and never exposes your main
  password or your contacts. If the dedicated account is abused or banned, your
  personal mail, Drive and recovery email are untouched. This is the single
  strongest mitigation in this plan and costs nothing.
- Rotation: revoke the app password in Google, replace the file, restart (or
  `SIGHUP` re-reads it).

### 2. Mail can only say one fixed thing, to one verified address

The sender is a narrow function, not a general mail API.

- Fixed templates compiled into the binary: "It is your turn in {game} (room
  {code})" plus a link and an unsubscribe link. The only variable fields are the
  game's display name and the room code, both stripped to a safe character set and
  length-capped, and always encoded as text, never interpreted as HTML.
- **No API, MCP tool, game move, chat message or setting can choose the recipient
  per message, the subject or the body.** The recipient is only ever the verified
  address stored for that seat.
- Recipient and header values are checked with `net/mail.ParseAddress`, then
  rejected if they contain `\r`, `\n`, `,`, `;`, `<` or `>` after parsing, or
  exceed 254 characters. Only a bare single address is accepted.
- Messages are built with a fixed header set (`From`, `To`, `Subject`,
  `Date`, `Message-ID`, `List-Unsubscribe`), plain text, `Content-Type:
  text/plain; charset=utf-8`. No user value goes into a header except the
  validated `To`.
- The SMTP envelope sender is always the configured `BINGLE_SMTP_FROM`, and
  there is exactly one recipient per message (no Cc, no Bcc).
- TLS is mandatory: STARTTLS on 587 with `tls.Config{ServerName:
  "smtp.gmail.com", MinVersion: tls12}` and certificate verification on. The
  sender fails closed if the server does not offer STARTTLS, so the password never
  crosses the network in the clear.

### 3. Proving the address belongs to the person (double opt-in)

Because there is no login, typing an address into a form proves nothing. Without
verification, anyone could make Bingle mail a stranger.

- Submitting an address does **not** enable notifications. It stores the address as
  *pending* and sends **one** confirmation mail containing a single-use token
  (256 random bits, only a hash stored, expires after 24 hours). Clicking the link
  (`GET /notify/confirm?t=...`, which shows a page with a Confirm button that
  POSTs, so link-scanners and prefetchers cannot confirm for the victim) marks the
  address verified.
- Only a verified address receives turn mail. A pending address receives at most
  the one confirmation, and no new confirmation is sent to that address for 24
  hours.
- Every notification carries a one-click unsubscribe link with a per-seat token
  and a `List-Unsubscribe` header. A global **suppression list** (hashed
  addresses) records unsubscribes and "this wasn't me" clicks; a suppressed
  address is never mailed again, whatever a player types.
- Confirming an address that a different seat has already confirmed is allowed
  (families share addresses) but counted toward the per-address caps below.

### 4. Who may read or change the setting (the no-authentication problem)

Knowing a name must not be enough to read or change an email setting.

- **Write-only address.** No endpoint, WebSocket message, MCP tool, game view or
  log ever returns the address. The options window shows only whether
  notifications are on and a masked form (`m••••@gmail.com`) that the server
  generates itself and that is never enough to reconstruct the address.
- **Seat ownership token.** When a person joins a seat, the server issues a random
  secret (stored as a hash, returned once, kept in the browser's storage) and the
  email endpoints require it. Without it, `PUT /games/{code}/notify` returns 403
  even for the right name. This does not fix Bingle's general lack of
  authentication, but it does stop another player rewriting your notification
  target. If Bingle later gets real accounts, the token is replaced by the
  account check.
- Changing an existing verified address requires re-verifying the new one, and
  sends the old address a "your notification address was changed" notice with an
  undo link.

### 5. Rate limits and a kill switch

All limits are enforced in the server, with counters in SQLite so they survive a
restart. Suggested defaults, all configurable:

| Limit | Default |
|---|---|
| Confirmation mails per address | 1 per 24 h, 3 ever |
| Confirmation mails per client IP | 5 per hour |
| Address changes per seat | 3 per day |
| Turn mails per address | 1 per game per 15 min, 20 per day |
| Mails per game | 100 per day |
| Mails sent by Bingle overall | 150 per day (well under Gmail's cap), 20 per hour |
| Verified addresses per game | at most the game's seat count |

- A global **circuit breaker**: if the global hourly cap is hit, or Gmail returns
  an authentication error (`535`) or rate error (`421/450/550 5.4.5`), sending stops
  for a cool-down and an alert is logged. Authentication failures disable the
  mailer until restart so a bad password is not retried into a lockout.
- A server-side switch: no SMTP configuration means the feature is off, the option
  is hidden from the options window and the endpoints return 404. An admin command
  (`bingle mail disable`, or deleting the secret file and restarting) turns it off
  in seconds.
- Sending happens in one background worker with a bounded queue (drop the oldest
  when full), so a slow Gmail connection never delays a move and a flood cannot
  build unbounded memory.

### 6. Protect the rest of the system from this feature

- Addresses are excluded from: game views, the WebSocket, `internal/mcpserver`,
  the REST game export, bot-batch data, logs, error messages and the retention job
  output. Add a test that serialises every public DTO and fails if it finds an
  address in it.
- Addresses and tokens are stored in their own tables (`notify_prefs`,
  `notify_tokens`, `notify_suppress`), deleted by `ON DELETE CASCADE` with the game
  and by the retention job.
- Logging records `seat`, `game` and a result, never the address, the token or the
  SMTP conversation. The SMTP library's debug output stays off.
- The mailer package has no HTTP-facing code and a tiny API (`Send(kind, seat,
  game)`), so a bug elsewhere cannot hand it free-form content.
- Dependency note: use the standard library (`net/smtp` with an explicit
  `tls.Config`, or write the few commands directly). No new third-party package.

## User-facing behaviour

In the game options window, a new **Email notifications** section (shown only when
the server has the mailer configured):

- Checkbox **"Email me when it is my turn"**, off by default.
- **Email address** field and a **Send confirmation** button. States shown:
  *not set*, *waiting for confirmation (m••••@gmail.com)*, *confirmed*.
- A note: "Your address is only used for turn notices in this game and is never
  shown to other players. Use the link in any email to stop them."
- **When to send**: *every time it is my turn*, or *only if I have been away for N
  minutes* (default 30), reusing the away-time idea from Auto-Play Timeout.
- **Stop emails** and **Forget my address** buttons.

Because Bingle currently runs over plain HTTP, the section warns when the page is
not on HTTPS ("This connection is not encrypted; your email address could be seen
by others on the network") and the confirmation step is the only thing proving
ownership. Do not enable the feature for the public internet until Stage 0 below
is done.

## Staged plan

**Stage 0: prerequisites (before any email code reaches the public server).**
1. TLS on the public listener (see `docs/history/webrtc-tls-plan.md` Stage 1). Without it,
   addresses, seat tokens and confirmation links are readable on the wire.
2. Seat ownership tokens (section 4), as a general mechanism, since other features
   also need them.
3. A code review of the REST and WebSocket surface for injection, path traversal
   and unauthenticated admin or debug endpoints, since a compromise of the process
   exposes the secret file.
4. Create the dedicated Gmail account and app password; store it as a Podman secret
   or `0400` file outside the repository.

**Stage 1: config and mailer.** `config.Secret`, the `BINGLE_SMTP_*` settings with
`*_FILE`, startup permission check, `internal/mail` with the fixed templates,
address validation and STARTTLS-required sending, plus a `bingle mail test <you@…>`
command that sends one test message to an address you pass on the command line
(never from the web).

**Stage 2: storage and limits.** Tables, hashed tokens, suppression list, counters,
circuit breaker, background worker.

**Stage 3: API.** `GET/PUT /games/{code}/notify` and the confirm and unsubscribe
endpoints, all behind the seat token, none returning the address.

**Stage 4: sending hook.** Call the worker where the turn passes to a seat (the
same place Auto-Play Timeout looks), skipping bots, people connected right now, and
seats whose notification is off, unverified or suppressed.

**Stage 5: options window and docs.** UI section, `docs/ui.md`, `docs/api.md`,
`docs/mcp.md` (state that no tool exposes addresses), ADR, and the deployment notes
for the secret.

## Tests

- Config: the password never appears in `fmt`, `%+v`, `%#v`, JSON or slog output of
  the config; a group-readable secret file is refused; a missing file disables the
  feature.
- Mailer: header injection attempts (`\r\n`, `,`, `;`, `<>`, very long values) are
  rejected; exactly one recipient; no STARTTLS means no send; a fake SMTP server
  checks the whole conversation, including that `AUTH` only happens after TLS.
- Abuse: unauthenticated or wrong-token calls return 403 and change nothing; the
  address is not returned by any endpoint, WebSocket frame or MCP tool; limits
  trigger at the stated counts; the circuit breaker opens on `535`.
- Verification: a pending address never receives turn mail; a token works once,
  expires, and a `GET` alone does not confirm; a suppressed address stays
  suppressed.
- Privacy: a test serialises every public DTO and fails on anything resembling an
  address.

## Residual risks, stated plainly

- If someone gets code execution on the Bingle process they can read the secret
  file, because the process has to use it. The dedicated account and app password
  limit that to "can send some spam from a throwaway account, revocable in one
  click" instead of "owns your Google account".
- Gmail may still suspend the dedicated account if a determined attacker gets many
  confirmation mails out to many addresses despite the limits. The per-IP limits
  help, but the clients' IP addresses are only as trustworthy as the reverse proxy
  headers in front of Bingle.
- Gmail rewrites the `From` and may add its own footer; deliverability to Gmail and
  other large providers is good, but not guaranteed.
- Mail is not end-to-end private: the content (a game name and a link) passes
  through Google.

## Alternatives if this feels too risky

- Skip email and send push notifications through a self-hosted ntfy or a Discord
  webhook: no credential that can be used as a relay, and the "address" is a topic
  name with the same verification and rate-limit design.
- Use a transactional provider (Postmark, SES, Brevo) with a **send-only API key**
  scoped to one verified sender domain: a leaked key can send spam but cannot touch
  a mailbox, and the provider enforces its own rate limits and has abuse tooling.

## Status log

- 2026-10-25: plan written. Nothing implemented.
- 2026-10-25: built (`internal/mail`, `internal/session/notify.go`, `internal/api/notify.go`, the
  `/notify/*` pages, `js/notify-ui.js`). Differences from the plan above:
  * **Any SMTP server, not only Gmail** (`BINGLE_SMTP_HOST/PORT/USER/TLS/CA_FILE`), STARTTLS or implicit TLS, never
    in the clear. The sender is `BINGLE_TLS_EMAIL` (the owner's choice) rather than a separate `BINGLE_SMTP_FROM`. A free
    proton.me account has no SMTP access, so a Proton sender needs a paid plan (SMTP token or Mail Bridge) or another
    provider; see email.md.
  * **Contacts instead of per-seat addresses.** An address is confirmed once per browser (a contact with a secret the
    browser keeps) and can then be pointed at any seat of that browser that has the seat's token, so a player does not
    confirm it again for every game. The harassment defence is unchanged: typing someone else's address only makes
    one confirmation mail go to them (limited per address and per client) and nothing more until they confirm.
  * **Seat tokens** (Stage 0.2) are issued by the create and join answers, keyed by the player's name, and needed only for
    the notification endpoints (not a general authentication mechanism).
  * Limits are the plan's, except confirmation mails: 1 per 10 minutes and 5 per day per address (the plan's 1 per day and
    3 ever is too harsh when a confirmation lands in a spam folder).
  * Not built: the "undo" link of the address-change notice (the notice carries a stop link instead), the `bingle mail
    test` command, `SIGHUP` re-reading of the password file, and the code review of Stage 0.3.
