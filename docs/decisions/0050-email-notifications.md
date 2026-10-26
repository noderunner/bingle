# 0050. Email notifications: contacts, seat tokens, TLS-only SMTP

Status: accepted (2026-10-25)

## Context

`docs/history/email-notifications-plan.md` planned turn emails through a Gmail app password and a security design for a server with no accounts. The owner asked for it to be built, with `BINGLE_TLS_EMAIL` (a throwaway proton.me address) as the sender, a note in the profile and the game options telling players to whitelist that address, and the plan's security kept.

## Decision

- **A generic SMTP client** (`internal/mail`), standard library only (`net/smtp` with our own TLS): STARTTLS or implicit TLS is mandatory and checked before AUTH, so the password never crosses the network in the clear; a server certificate that does not verify is refused (a CA file can be given for Proton Mail Bridge). The password comes only from `BINGLE_SMTP_PASSWORD_FILE` (mode 0600 or the server does not start) into `config.Secret`, which prints as `[redacted]` in every format. Free proton.me accounts have no SMTP, so a Proton sender needs a paid plan or another provider (`docs/email.md`).
- **Fixed templates, one validated recipient** (`ValidAddress`: a single bare ASCII address; `Sanitize` for the game name and room code), a fixed header set, `List-Unsubscribe` with one-click POST.
- **Contacts, confirmed once per browser.** An address becomes a contact with a secret held by the browser and a stop token; it is mailed a link to a page whose button POSTs the token (so link scanners cannot confirm). A seat can use a contact only with the seat's token and the contact's secret. This replaces the plan's per-seat confirmation so one confirmation covers every game, while keeping the property that typing a stranger's address makes exactly one rate-limited confirmation mail go to them.
- **Seat tokens** are issued by the create/join answers (once, hashed in the database, keyed by player name) and protect only the notification endpoints; they do not authenticate the rest of the API.
- **Sending**: a tap on the hub's events notices a turn passing to a seat with a confirmed address and the switch on; "every time" mails unless the player's page is connected, "away N minutes" waits and re-checks that the turn is still pending. Limits are counted in SQLite (`notify_sent`); a login failure disables the mailer until restart, a rate error pauses it an hour; a single worker drains a bounded queue.
- **Privacy**: the address is never returned by any endpoint (a masked form only), and the tests fetch the public surfaces and WebSocket frames to check.
- **Off by default** and without a public URL: with no `BINGLE_SMTP_HOST` the feature is absent (endpoints 404, the profile shows a note).

## Consequences

Anyone with the seat token and contact secret in their browser can change the setting, so clearing browser data loses the ability (the setting stays). A seat taken before this release gets its token at the next rejoin by name, first come first served. Pending "away" waits are lost on restart. Delivery depends on the provider; the notes tell players to whitelist the sender.
