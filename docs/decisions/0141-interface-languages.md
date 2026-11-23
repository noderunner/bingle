# 0141. The interface in ten languages

Status: accepted (2026-11-22), for issue #251. Its first stage is built: the catalogs, the setting, the
language of every page and right-to-left layout. The page's modules are translated one by one after it, then the
mail and the changelog.

## Context

Bingle's interface was English only. Issue #251 settled ten languages (English, Spanish, French, German,
Portuguese (Brazil), Chinese (Simplified), Japanese, Korean, Russian and Arabic), Arabic's right-to-left layout in
the first release, the server's messages left in English, and mail in the recipient's language. How to build it
was left to an ADR. The page has no build step, and the binary has eleven direct dependencies.

## Decision

- **A catalog per language, JSON, embedded** (`internal/i18n/catalogs/<code>.json`): a key's words with `{name}`
  where a value goes in; an object of plural forms (`one`, `other`, … as `Intl.PluralRules` names them) for a key
  given a count; markup only in keys ending `_html`, which only a template may use. English is complete, and any
  key another catalog lacks is read in English. The same catalogs serve the page, the templates and, later, the mail.
- **The codes are BCP 47 tags**: `en`, `es`, `fr`, `de`, `pt-BR`, `zh-Hans`, `ja`, `ko`, `ru`, `ar`. Chinese is
  `zh-Hans`, by its script, since that is what differs; any Portuguese is offered `pt-BR`, and Chinese in traditional
  characters is offered nothing (the next language of `Accept-Language`, or English).
- **The server decides the page's language**: the account's choice, else the browser's (the `bingle_lang`
  cookie), else the first language of `Accept-Language` it has, else English. It writes it into `<html lang dir>`,
  so a right-to-left page is laid out so from its first paint, and translates the templates with
  `{{t .Lang "key"}}`. The pages read the account from its session cookie without making a guest.
- **The page loads its catalog in `js/i18n.js`**, by a top-level `await` of `/static/i18n/<code>.json` (the
  languages and the catalog with English filled in, sent with an ETag), before any module that imports it runs.
  `t(key, params)` gives the words, and `tParts` the same as a list whose values may be elements, so that a link
  keeps its place in any word order. Numbers in values, plurals, dates and the like go through the browser's `Intl`.
- **The choice is `POST /auth/language`**: an account keeps it in `accounts.language` (`""` for the browser's
  own), so that it follows the account and its mail can be in it; the browser keeps it in the cookie either way, for
  a guest, and for after signing out. `/auth/me` reports `language` and `language_choice` (API 1.9.0).
- **The Language setting** is in the profile window and on the profile page: the browser's own language first,
  then each language in its own name. Choosing one loads the page again.
- **Right to left by logical properties**: the stylesheet says `margin-inline-start`, `inset-inline-end`,
  `text-align: start` and so on, so that Arabic mirrors the top bar, the panels and the dialogs. A board, the cards on
  a table, the move list and every picture (`svg`) stay left to right. The phone layout's pages run from the right,
  as the bar below them does.
- **What stays as it is**: the brand, handles, game names, notation, the API's words (ADR 0114) and the
  server's messages (`auth: …`, `api: …`), which are shown as they come, the words the page puts around them
  translated.
- **Translations are drafted by machine, marked, and reviewed by a speaker before release**: a catalog no
  speaker has reviewed says so in `_review`. `TestCatalogs_Report` lists what each catalog lacks.
- **A pseudo-locale, `en-XA`**, made from English (accented, about 40% longer, in brackets), finds layouts that
  break with longer words and words left out of the catalogs. It is not listed; a developer sets the cookie.

## Left for its stage

- **Plurals in mail**: hand-written rules for the ten languages, or `golang.org/x/text`, which is one more direct
  dependency. Until then the server knows English's plural forms only.
- **The changelog** in ten languages (`docs/changelog/<code>.md`, the help window showing the reader's or
  English), with its step in CLAUDE.md's changelog rule and a test that each language's newest entry has English's
  version.
- **A glossary per language** for the platform's words (impulse, muster, practice batch, badge).

## Consequences

- Every new word of the interface goes through `t()` with its key in `en.json`, and a test fails on a key the page
  uses that English lacks. Until a module is translated, its words are English in every language, and English in a
  right-to-left page puts its full stop on the wrong side.
- The page waits for one small request before its modules run. The catalog is asked for every time, and is
  usually answered `304 Not Modified`.
- Browser tests keep running in English, and select by id, which no language changes.
