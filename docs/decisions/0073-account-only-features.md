# 0073. Direct messages, who is here, the global chat and notifications are accounts'

Status: accepted (2026-10-31)

## Context

Stage 8, the last, of [the accounts plan](../history/auth-and-accounts-plan.md) (§7): the features
that cannot be held to anybody without an account move onto accounts, and the documents that said
"there are no accounts" stop saying it. ADR 0006 is retired.

## Decision

- **Who is here** lists signed-in accounts only, by handle, under an address that is a hash of the
  account's principal (eight bytes of SHA-256 of a fixed prefix and its ID): it says nothing about
  the account, and does not change with a rename. A guest is not listed and sees no list
  (`dm` capability). `POST /here` takes no name and no secret.
- **Direct messages and direct calls** are between accounts: the sender is the signed-in account,
  under its handle; `/ws-dm` is authenticated by the session and lost its hello frame. The
  browser's own secret, `bingle.session`, and `session.Address(secret)` are gone.
- **One voice at a time** holds for every principal, guests included: the address of whoever joins
  a game's call or a direct call.
- **The global chat** is written by accounts (`chat_global`), under their handles whatever they
  typed, and read by everybody; the page says so to a guest.
- **Email notifications** are accounts' (`notify`, the state-changing routes only: reading whether
  mail is set up is open). A **confirmed contact is grandfathered**: the settings it has keep their
  turn mail, and nothing is deleted. A guest can no longer change them or make a new contact — but a
  browser holding a confirmed contact can turn it into an account at once: `POST /auth/signup` with
  the contact's `contact_id` and `contact_secret` and a handle and a password, with no second
  confirmation, since the address was proven. The guest becomes the account, as every sign-up does.
- **The capabilities** `dm`, `voice_direct`, `chat_global` and `notify`, seeded account-only in stage
  6, are now enforced; an administrator can still open any of them to guests on `/admin`.
- **Tests.** The direct-message, who-is-here and global-chat suites sign in. The notify suite tests
  how mail works with guests allowed notifications (a test capabilities file); what the defaults
  mean — no contact for a guest, the grandfathered mail, the upgrade — is tested with the defaults.

## Consequences

A guest who used direct messages or the global chat signs up to go on. The browser's
`bingle.session` value is left behind in old browsers, unread. ADR 0006's "re-enter your name"
survives only for seats an anonymous client took.
