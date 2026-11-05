# 0110. An instance can have a name and an icon of its own

Status: accepted (2026-11-05)

## Context

The maintainer asked that another instance of the platform can show its own name and icon in place of
Bingle's, throughout the UI. The name stays Bingle in the code.

## Decision

- **It is set by the operator, at start:** `BINGLE_BRAND_NAME` and `BINGLE_BRAND_ICON` (a file), like the
  rest of an instance's configuration, not on the administration page. A brand does not change while
  a server is running, and a mistake stops the start rather than appearing to players.
- **A player sees the name everywhere they see the platform's:** pages, the top bar, the footer, the help window, mail
  (subject, sender and body), calendar files, authenticator apps, the rules and the changelog. The server replaces the word
  in what it sends, so none of the text had to be rewritten and a new mail template is covered.
- **It stays Bingle** where a player is not reading it: code, `BINGLE_*`, `X-Bingle-Session`, cookies, `localStorage`
  keys, the API's and MCP's descriptions for programs, and the engine's own logs. The help window keeps a link to the software's source, as *Source on GitHub*, and has no *Report Issues*: the issues of
  another instance are not this project's to answer.
- **The name is limited** to 40 letters, digits, spaces and `. ' _ -`, as it goes into pages, mail headers and calendar files.
- **The icon is served with the sandbox** used for guide assets, since an SVG opened by itself could run a script.

## Consequences

- An authenticator app already set up keeps the name it was given.
- Text about the platform written in a game's own definition is replaced like the rest.
