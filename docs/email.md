# Email notifications

A player can ask to be emailed when it is their turn in a game. This page is the
operator's guide (how to turn it on) and a summary of how it behaves; the reasoning and the
threats it is built against are in [email-notifications-plan.md](history/email-notifications-plan.md)
and [ADR 0050](decisions/0050-email-notifications.md).

The feature is **off** unless the mail server is configured. Nothing else changes when it is off:
the profile and options show a note instead of the controls, and the endpoints answer `404`.

## What players see

* **Profile page** (`/profile`): an *Email* field, a switch *Send me an email when it is my turn*,
  and the state of the address (*waiting for you to confirm*, *confirmed*, *stopped*) with
  *Send the confirmation again* and *Forget my address*. Saving an address mails **one**
  confirmation link to it; nothing else is mailed until the person opens the link and presses
  **Confirm** (a link scanner that merely opens the link confirms nothing).
* **Game options → Email notifications**: for the person in a seat. A switch for this game, and
  *every time it is my turn (not while this game is open in a window)* or *only if I have not moved
  for N minutes*. The profile's switch is the default for games the player has not chosen for: the
  first time the browser that took the seat opens the game, it is switched on there.
* Both places say **which address the mail comes from** (the configured sender,
  `BINGLE_TLS_EMAIL`), so the person can add it to their contacts or safe-sender list and look for it
  in the spam folder.
* Every mail has a one-click *stop* link (and `List-Unsubscribe`). It silences the address
  for good: it cannot be registered again (an operator can lift it, see below).
* The address is **write-only**: no page, API call, WebSocket frame or log shows it back. The pages show
  a masked form (`a••••@example.com`).

The mail itself is fixed text: *"It is your turn in {game} (room {code})"*, a link to the game and the
stop link. Nothing a player types ends up in a message except the game's name and room code, both
reduced to letters, digits and a few punctuation marks.

## Turning it on

The mail is sent from `BINGLE_TLS_EMAIL` (the same address the certificate authority gets) through an
SMTP server with a login:

| Variable | Default | Meaning |
|---|---|---|
| `BINGLE_TLS_EMAIL` | *(required)* | The sender address, such as `bingle_games@proton.me` |
| `BINGLE_SMTP_HOST` | *(unset = off)* | The SMTP server |
| `BINGLE_SMTP_PORT` | `587` | Its port |
| `BINGLE_SMTP_TLS` | `starttls` | `starttls` (port 587) or `tls` (implicit TLS, port 465). Mail is **never** sent without TLS |
| `BINGLE_SMTP_USER` | the sender address | The login name |
| `BINGLE_SMTP_PASSWORD_FILE` | *(required)* | A file holding the password. There is deliberately **no** plain environment variable for it |
| `BINGLE_SMTP_CA_FILE` | system roots | A PEM file with the certificate that signs the server's, for a server with a certificate of its own (Proton Mail Bridge) |
| `BINGLE_MAIL_HOURLY_LIMIT` / `BINGLE_MAIL_DAILY_LIMIT` | `20` / `150` | All mail bingle will send per hour and per day |
| `BINGLE_PUBLIC_URL` | `https://<BINGLE_TLS_DOMAIN>` | Where the links in the mail point. Without one the feature stays off (the log says so) |

The password file must be readable by its owner only (`chmod 600`); bingle refuses to start
otherwise, and it never prints the password (not in the config, logs, errors or JSON).

```sh
mkdir -p ~/.config/bingle && umask 077
printf '%s' 'the-smtp-password' > ~/.config/bingle/smtp_password   # 0600
BINGLE_TLS_EMAIL=bingle_games@proton.me \
BINGLE_SMTP_HOST=smtp.example.com \
BINGLE_SMTP_PASSWORD_FILE=$HOME/.config/bingle/smtp_password \
BINGLE_PUBLIC_URL=https://bingle.example.com \
./bin/bingle
```

In a container, make the file a secret (`podman secret create bingle_smtp_password -` and
`--secret bingle_smtp_password`, then `BINGLE_SMTP_PASSWORD_FILE=/run/secrets/bingle_smtp_password`).

### Which SMTP server

Bingle must log in to a server that will send as the sender address. Use a **dedicated account**, not a personal
one: if it is abused or suspended, nothing else is affected.

* **Proton Mail.** A **free** proton.me account has no SMTP access (as far as the published
  documentation says; check Proton's current rules before relying on this). Paid plans have two routes:
  * *SMTP submission* (custom-domain addresses, business plans): `smtp.protonmail.ch`, port `587`,
    login = the address, password = an **SMTP token** created in the Proton settings.
  * *Proton Mail Bridge* (paid plans), which must run on a machine of yours: `BINGLE_SMTP_HOST=127.0.0.1`,
    `BINGLE_SMTP_PORT=1025`, `BINGLE_SMTP_USER` and the password it generates for you, and
    `BINGLE_SMTP_CA_FILE` pointing at the certificate you export from Bridge (its certificate is
    self-signed).
* **Gmail** (a dedicated account with 2-step verification and an **app password**): `smtp.gmail.com`,
  `587`, sender = that Gmail address.
* A **transactional provider** (Brevo, Postmark, SES and so on) with a send-only credential.

The sender address must be one the SMTP account is allowed to send as. If you send *as* an address at one
provider through another provider's server (say a `proton.me` sender through Gmail), most large mail
providers will treat the mail as forged and spam-fold or reject it. Send through the provider of the sender's own domain.

### Limits and safety nets

All counted in the database, so they survive a restart:

| What | Limit |
|---|---|
| Confirmation mails to one address | 1 per 10 minutes, 5 per day |
| Confirmation mails caused by one client | 5 per hour |
| Turn mails to one address in one game | 1 per 15 minutes |
| Turn mails to one address | 20 per day |
| Turn mails for one game | 100 per day |
| Notices to one address (its address changed; someone signed up with it again) | 1 per hour |
| Everything bingle sends | `BINGLE_MAIL_HOURLY_LIMIT` per hour, `BINGLE_MAIL_DAILY_LIMIT` per day |

A **login failure** (`535`, `534`, `530`) stops all mail until bingle is restarted, so a wrong password is
not retried into a lockout; a **rate error** from the server (`421`, `450`, `452`, `5.4.5`) pauses mail for an hour.
Mail is sent by one background worker with a queue of 64 (the oldest message is dropped when it is full), so a slow
mail server never delays a move.

### Seeing what the mail did

At startup the log says `notify: mail is on` with the host, port, TLS mode and limits, or
`notify: mail is off` and why. After that every message has lines of its own, with its `kind`
(`confirm`, `reset`, `turn` or `notice`) and its address only as `addr=`, twelve hex characters of a
hash ([ADR 0063](decisions/0063-what-a-log-line-may-say.md)); `logging.Addr` gives the same value for
an address you want to look for:

| Level | Line | Meaning |
|---|---|---|
| debug | `notify: queued a message` | It will be sent |
| info | `notify: sent a message` | The mail server took it, and how long that took |
| info | `notify: a message was held back by the sending limits` | One of the limits above |
| info | `notify: not sending a message: the address asked not to be mailed` | Its address followed a stop link |
| info | `signup: the address already belongs to an account, so no confirmation was sent` | Sign-up answers as usual, so it does not say whose the address is. The account's owner is mailed *You already have a Bingle account* instead (a `notice`, at most one an hour), and the audit log gets `signup-address-in-use` |
| info | `password reset: …, so no mail was sent` | No such account, or too many links lately |
| warn | `notify: a message was not delivered` / `…asks us to slow down` / `…not sent: …` | The server refused it, paused us, or sending is stopped |
| error | `notify: the mail server refused the login` | All mail stops until a restart |

The same outcomes are counted in `bingle_mail_total{kind,outcome}` on the metrics listener
([observability.md](observability.md#what-is-measured)).

### Maintenance

* **Stop the feature**: unset `BINGLE_SMTP_HOST` (or delete the password file) and restart.
* **Rotate the password**: replace the file and restart.
* **Lift a "stop"** (someone asks to be mailed again): the address is kept only as a hash. Ask them to use a
  different address, or delete the row from `notify_suppress` for the SHA-256 of the lower-cased address.
* Unconfirmed addresses older than a week and the records of mail sent older than a week are deleted hourly.

## How it works (short)

* Email notifications are for **accounts** ([ADR 0073](decisions/0073-account-only-features.md)): a guest
  cannot make a contact. A browser's address is a **contact**: the server stores it with a secret (only its hash)
  that the browser keeps, and a stop token. A seat's setting is for the account that holds the seat. Nobody else
  in the game can read or change yours.
* **A confirmed contact from before** keeps its turn mail: the settings it has go on working, and a guest whose
  browser confirmed an address can make it an account at once (`POST /auth/signup` with `contact_id` and
  `contact_secret` instead of `email`: a handle and a password, no second confirmation).
* `PUT /api/games/{code}/notify` needs the seat's owner **and** a confirmed contact the caller owns.
* The server watches the room's events; when the turn passes to a seat whose person has a confirmed address and
  the switch on, it mails them, unless the person has the game open (a connected WebSocket) in "every time" mode, or
  the turn has already moved on when the "N minutes" wait is over. Waits are kept in memory: a restart forgets them.
* Only addresses with a real seat are mailed: bots are never, and nobody can mail another person's address
  from their own seat, because confirming needs the link that went to that address.

Endpoints are in [api.md](api.md); the UI pieces in [ui.md](ui.md).

## Tests

`tests/mail` (addresses, header injection, TLS-only sending against a fake SMTP server in `internal/mail/fakesmtp`,
login failures, the secret never printed), `tests/app/notify_test.go` (the whole flow, tokens, limits, privacy,
the stop link, the circuit breaker), `tests/app/notify_ui_test.go` (profile and options in a real browser).
Not covered: delivery through a real provider and what a provider's spam filter makes of the mail.
