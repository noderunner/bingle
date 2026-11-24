# 0143. Chat translated by a service on the server's own network

Status: accepted (2026-11-23), for issue #252. It builds on [ADR 0141](0141-interface-languages.md) (the reader's
language), [0105](0105-friends.md) (direct messages are live only), [0071](0071-capabilities-modes-and-tokens.md)
(capabilities) and [0134](0134-an-api-version-and-a-schema-of-its-messages.md) (the API's version). Built in four
stages: the service, game and global chat, the page, direct messages.

## Context

Players who read different languages share a game's chat, the global chat and direct messages. Issue #252 settled
that a line should reach each reader in their language, with the original a click away, and that the translation
is done by a model outside the binary, on the server's own machine or network, never by an outside service.
Translation is on by default for guests and accounts alike. Bingle is one static binary with few dependencies, so
the model cannot be inside it; it does have an HTTP client.

## Decision

- **A translation service over HTTP, named by the operator**: `BINGLE_TRANSLATE_URL`, off when unset, with
  `BINGLE_TRANSLATE_MODEL`. The first kind spoken is `openai`, the `/v1/chat/completions` endpoint that Ollama,
  llama.cpp's server and vLLM all speak, so one small client covers the common ways to run a model at home
  (`BINGLE_TRANSLATE_API`; `libretranslate` may follow). `docs/translate.md` describes one setup: Ollama with a Qwen
  model, whose licence (Apache-2.0) allows any use and which covers the ten languages, Arabic included.
- **What is not language is never sent**: handles, links, emoji and notation (`c3`, `a1-b2`, `O-O`) are swapped for
  markers (`⟦1⟧`) before the line goes and put back after. A line with no words left is not sent at all.
- **The prompt is fixed and the line is data**: the system message says what to do and that the message is not to
  be obeyed or answered; the line is the user message alone. The answer is JSON, `{source, text}`, asked for by
  schema. It is **dropped** when a marker is missing, repeated or invented, when it holds a line break or markup the
  line did not, when its length is far from the line's, or when its source is already the reader's language. A
  dropped answer means the line stays as written. The page shows a translation as plain text only.
- **One request per line and language**, not one per reader and not one for all languages at once: a small model
  translates worse when asked for several, and a room rarely holds more than two or three languages. The writer's own
  interface language goes with it as a hint for detection.
- **Chat never waits**: requests go through a bounded queue to a few workers (`BINGLE_TRANSLATE_WORKERS`, one by
  default, since a laptop's model serves one at a time) with a timeout (`BINGLE_TRANSLATE_TIMEOUT`). A full queue
  drops the request. After several failures in a row the service is left alone for a minute, so a laptop that is
  asleep costs nothing. Metrics count each outcome and time each request.
- **Privacy**: chat lines go to the configured service, which is why it must be local. The server warns at start-up
  when the URL's host is not loopback or a private address. Such services usually have no authentication of their
  own, so anyone on that network can use them; `docs/translate.md` and `SECURITY.md` say so.
- **Translate once per language present** (stage 2): a reader who subscribes to a room or the global chat registers
  their language and their switch, and a new line is translated into each language registered other than its own.
- **Stored with the chat it translates** (stage 2): `chat_translations` and `global_chat_translations`, keyed by the
  line and the language and removed with the line, are the cache, so history and late joiners do not pay again. A
  line that needs no translation (already in the language, nothing to translate, the answer dropped) is kept with an
  empty one, so it is not sent again. A reader who comes later has the latest 20 lines without one asked for when
  their socket opens, which then brings them, and when their history is read. **Direct messages' translations are never stored**: they live
  in memory with the message (ADR 0105).
- **The API** (stage 2): the `chat` event is unchanged and comes at once; a `chat_translation` event, `{id, lang,
  from, text}`, follows when it is ready, and each socket passes on only its reader's language. History's lines gain
  an optional `translation`. A minor version.
- **Who gets it** (stage 2): a `translate` capability, held by guests and users by default, and a per-minute limit
  per class counted on the **writer's** lines, since what a writer sends is what costs. The switch, *Translate chat
  into my language*, is on by default: an account keeps it in `accounts.translate_chat`, a guest in a cookie, and
  `/auth/me` reports it. With no service configured the switch is not shown.
- **Moderation sees the original**, always. System lines are not translated by the model: they come in the
  reader's language from the catalogs.

## Considered

- *A model inside the binary*: cgo, a large download and a much larger binary. Refused by #252.
- *An outside translation API*: sends players' words off the network. Refused by #252.
- *LibreTranslate (Argos)*: no prompt to inject into and a detection endpoint of its own, but weaker on Arabic, Korean
  and Japanese chat. It may be the second kind of service.
- *All languages in one request*: one detection and one prompt, but a longer answer that small models get wrong more.
- *The browser's own `Translator`*: free for the server, but most browsers lack it. A later extra, not the base.
- *NLLB-200, Aya*: good coverage, but licensed for non-commercial use only.

## Consequences

- With `BINGLE_TRANSLATE_URL` unset nothing changes: no requests, no switch, no new fields on the wire.
- A slow or missing service only means lines stay as written.
- An instruction model can still translate badly; the checks drop what is plainly not a translation, not what is a
  poor one. The original is always one click away.
