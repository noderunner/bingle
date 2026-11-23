# The interface's languages

Bingle's interface speaks ten languages: English, Spanish, French, German, Portuguese (Brazil), Chinese
(Simplified), Japanese, Korean, Russian and Arabic, which is written right to left. [ADR
0141](decisions/0141-interface-languages.md) says why it is built as it is; this page is how to work with it.

Only the interface's own words are translated. The brand, players' handles, game names, notation, the API's words and
the server's messages (`auth: …`, `api: …`) stay as they are. A game's own content (its rules guide, tutorials and
badge names) is issue #253, and chat is #252.

## Which language a page is in

The server decides, for every page, and writes it into `<html lang="…" dir="…">`:

1. the language the signed-in account chose (`accounts.language`);
2. else the one this browser chose, in the `bingle_lang` cookie;
3. else the first language of the browser's `Accept-Language` that the interface has (`es-MX` is `es`, any
   Portuguese is `pt-BR`, Chinese is `zh-Hans` unless it asks for traditional characters);
4. else English.

The **Language** setting, in the profile window and on the profile page, calls `POST /auth/language` and loads the
page again. Its first choice, *Browser (…)*, is the browser's own language (`""`): it takes the cookie and the
account's choice away. `/auth/me` reports `language` and `language_choice`, for a native client.

## Words in the code

Every word the interface shows has a key in `internal/i18n/catalogs/en.json`, and each other language's file has
the same key translated. Keys are grouped by where they are used (`profile.…`, `settings.…`), with `common.…` for
words used in many places.

In a module:

```js
import { t, tParts } from "./i18n.js";

h("button", {}, t("common.save"));
h("p", {}, t("profile.write_to", { name: p.name }));              // "Write to {name}"
h("p", {}, ...tParts("settings.guest", { signin: link }));      // an element keeps its place in the sentence
t("games.count", { n: rows.length });                            // {"one": "{n} game", "other": "{n} games"}
```

In a template, `{{t .Lang "key"}}`, with values after the key: `{{t .Lang "settings.email_note" "brand" brandName}}`.
A key ending in `_html` may hold markup (a link, `<em>`), and only a template may use it; the values put into it are
escaped.

- **Use the browser's `Intl`** for what changes with the language: `Intl.PluralRules` (which `t` uses for a plural
  key), `Intl.DateTimeFormat` and `toLocaleString` for dates, `Intl.RelativeTimeFormat` for *3 min ago*,
  `Intl.ListFormat` for lists. A number put into words is formatted by `t` already. `i18n.js` exports `lang`.
- **Whole sentences, not pieces**: word order differs. Put a link or a strong name into a sentence with `tParts`, not
  by joining translated pieces.
- **Right to left**: use logical properties in CSS (`margin-inline-start`, `padding-inline-end`, `inset-inline-end`,
  `text-align: start`), never `left` or `right`, except inside a board, a card table, the move list or a picture,
  which stay left to right (`[dir="rtl"] :is(#board, .card-table, .zone, .move-list, svg)` in `style.css`). Code that
  reads `scrollLeft` or places things by hand checks `dir` from `i18n.js`.

## Translating

Add the key to `en.json` and draft it in the nine other catalogs in the same pull request. A machine may draft
them: a catalog that no speaker of its language has reviewed since says so in its `_review` entry. Each language
needs a reviewer before release.

`go test ./internal/i18n -run Report -v` lists the keys each catalog lacks. A key missing from a catalog is shown in
English; a key missing from `en.json` fails `TestCatalog_HasEveryKeyThePageUses`. `TestCatalogs_MatchEnglish` checks
that each translation puts in the same values and has the same markup as English's.

## Testing a layout

- **Longer words**: set the pseudo-locale, `en-XA`, which is English accented, about 40% longer and in brackets,
  so that a word left out of the catalogs shows as plain English. It is not in the setting's list: in the browser's
  console, `document.cookie = "bingle_lang=en-XA; path=/"`, and load the page again.
- **Right to left**: choose العربية in the setting. `TestWebLanguages_*` checks that the page is mirrored and the
  board is not.
- The browser tests run in English and select by id.

## What is translated so far

- The profile window and the profile page.
- The top bar and its windows: Active Games, Join Game, Friends (with direct messages and calls), the call's panel,
  Help, and moving a guest to another device; the turn-mail settings; "3 min ago"; the footer of every page.

Next, the main page, the game page and the remaining pages (account, administration, tournaments, batches, a player's
page), then the mail and the calendar invitations, then the changelog in the help window (ADR 0141, *Left for its
stage*). `TestWebLanguages_TheTopBarIsTranslated` opens every window of the top bar in the pseudo-locale and fails on
any plain English word in it: a translated area gets a test like it.
