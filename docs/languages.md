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
- **Words left out**: `TestWebLanguages_*Translated` open a translated area in the pseudo-locale and fail on any
  plain English word in it (`plainEnglish` in `tests/app/languages_ui_test.go`). Names, handles, codes, the brand and
  the games' own words (their names, descriptions, options, seats, tutorials and badges, read from the API) are let
  through, and so is whatever stands in an element marked `translate="no"`: a name a player gave (a tournament's),
  which no language translates.
  A translated area gets a test like it.
- **Text that runs left to right inside a right-to-left page** (a version, "3–5P", a licence's name) goes in
  `<bdi>` or takes `dir="ltr"`, or the bidi algorithm reorders it.

## What is translated so far

- The profile window and the profile page.
- The top bar and its windows: Active Games, Join Game, Friends (with direct messages and calls), the call's panel,
  Help, and moving a guest to another device; the turn-mail settings; "3 min ago"; the footer of every page.
- The main page: the gallery, a game's setup (every option of Create Game, tutorials, deal history, active games),
  the puzzles, and the fun facts (`fact.*`).
- The main page's other tabs (Analysis, Ratings, Tournaments, Global Chat), a tournament's page and the shelf of
  badges. Dates and times follow the page's language (`lang` from `i18n.js`), not the browser's.
- The welcome tour, with the words in its pictures. Lists of words go through `Intl.ListFormat`.
- Around a game's board: the lobby (with musters), the chat and the names of its emoji (`emoji.*`), the rules
  window's own tabs (Controls, Key, search) and the options window's frame. A game's rules guide is its own (#253).
- The play area (`game.*`, `end.*`): whose turn it is and what they are asked, the players' strips, the banners
  (draws, slow turns, votes, hints), the status bar and how a game ended, analysis and its dialogs, and the
  phone's pages. A game's own words in these (its seats, pieces, actions, endings and page script) stay as the
  game has them. A message names its players inside `<bdi translate="no">` (`withSeatWords`), so no language touches them.
- The rest of the game page: the move bar (`move.*`), when to be asked about a reaction (`react.*`), the
  Auto-Play Timeout (`autoplay.*`), the history and the list of moves (`hist.*`), the options window's sections
  (`options.*`) and the tutorial's card. The lines of the log are the game's own, as are a tutorial's lessons and
  a game's contest panel; a notation's name (TPS, PTN) is marked `translate="no"`.
- Boards and cards: what a screen reader hears of a board (`board.*`, with the palette's colours as `color.*`;
  a piece's name stays the game's), the sets of cards (`cards.*`; a joker's name on its picture stays in the
  English lettering), solitaire's table (`sol.*`), the illustrated themes (`theme.*`) and the page layouts.
- The account pages (`account.*`: signing in and up, a forgotten password, confirming an account, the account
  itself with two-step sign-in, sessions and API tokens), a player's page (`player.*`) and the pages behind the
  turn mail's links (`mailpage.*`). Their titles are catalog keys the router passes to the template. Data in a
  sentence (a handle, an address, a browser's name) goes in a `<bdi translate="no">`, and an API scope stays the
  API's word, shown as code.
- The administration page (`admin.*`) and the practice batches' pages (`batch.*`, with the charts' words as
  `chart.*`). A capability, a kind of limit and an audit event's kind are the server's names and are shown as code
  or marked `translate="no"`, as is an event's detail; the server settings they name (`BINGLE_AUTH_MODE`) stay as
  they are. A game's own sections of a report come from its page script, and are its own (#253).

The Health page stays in English: it is served on the metrics listener, off the public site, for whoever runs the
server, and has no language of its own to follow.

The mail and the calendar invitations are in their reader's language too (`mail.*`, left out of what a page
loads): see [email.md](email.md) for which language a mail is in. They need no plural rules: their numbers are
written into whole sentences.

The help window's *Recent changes* is in the reader's language: `docs/changelog/<code>.md` has each release
from 0.124.0 (when the Language setting came) in that language, built into the binary beside `CHANGELOG.md`.
An older release, or one a language lacks, is shown in English, marked `lang="en"`. Each pull request adds its
entry to every language's changelog, as it does its words to every catalog; `TestChangelog_EachLanguageHasTheNewestEntry`
checks the newest entry and that each release keeps English's sections.

The platform's own words have a glossary, [docs/glossary/](glossary/en.md): what each means, the key that shows it,
and each language's word for it, which `TestGlossary_EachLanguageHasEveryTermAsItsCatalogShowsIt` checks against
the catalogs. A new string that uses one of these words uses the glossary's word; a change of a word changes the
glossary and every key that says it together. English's file has a second table, of the words of the code and the
docs that the interface shows nowhere (color, label, paint: ADR 0154), with no key; the other languages' files do
not repeat it, and the test reads only the first.
