# Chat translation

Bingle can show a chat line to each reader in their language ([ADR 0143](decisions/0143-chat-translated-by-a-local-service.md),
issue #252). The translation is done by a model that **you** run, on the server's own machine or network. Bingle
does not contain a model and never sends chat to an outside service of its own accord. With no service set up,
nothing changes: lines arrive as written.

> **Built so far:** game and global chat, on the page and in the API. Direct messages follow (#252).

On the page, a translated line is marked *Translated from …* with **Show original**, and the profile has the switch
**Translate chat into my language**, shown only on a server with a service.

## How chat is translated

- A reader whose socket is open in a game or the global chat registers their language there: the interface's (ADR
  0141), if they have not turned translation off (`POST /auth/translate`, on by default) and hold the `translate`
  capability. A line said there is sent **once for each language registered**, not once per reader.
- What it comes to is **kept with the line** (`chat_translations`, `global_chat_translations`) and goes with it, so
  history and later readers do not pay again. A line already in the reader's language, or with nothing to translate,
  is kept as needing none.
- A reader who arrives later has the **latest 20 lines** without one asked for when their socket opens (and when their
  page reads the history).
- **The writer's limit**: a writer's lines sent to the service, per minute, by class (`translate` in the capabilities
  file: 10 for guests, 30 for accounts). A line over it is shown as written.

## Settings

| Variable | Default | What it does |
|---|---|---|
| `BINGLE_TRANSLATE_URL` | *(unset = off)* | The service's base URL, up to the part before `/chat/completions`: `http://<host>:11434/v1` for Ollama |
| `BINGLE_TRANSLATE_MODEL` | *(required with the URL)* | The model to ask for, as the service names it (`qwen2.5:7b`) |
| `BINGLE_TRANSLATE_API` | `openai` | How the service is spoken to. `openai` is the `/chat/completions` endpoint that Ollama, llama.cpp's `llama-server` and vLLM serve |
| `BINGLE_TRANSLATE_WORKERS` | `1` | Requests made at once. A single GPU answers one at a time, so raise it only for a server that batches |
| `BINGLE_TRANSLATE_TIMEOUT` | `20s` | How long one request may take. The first request after the model was unloaded can be slow while it loads |

At start-up the server logs whether translation is on, and **warns when the URL's host is not this machine or a
private address** (10/8, 172.16/12, 192.168/16, fc00::/7, link-local), since chat lines go there.

## A recommended setup: Ollama and Qwen

[Ollama](https://ollama.com) runs on Linux, macOS and Windows and serves the OpenAI-style endpoint. **Qwen2.5 7B
Instruct** (`qwen2.5:7b`) covers all ten of the interface's languages, Arabic included, runs on a GPU with 8 GB, and
is licensed Apache-2.0, which allows any use. Check the licence on the model card of the exact size you pull: not
every size of a family shares one.

Models **not** to use on a server that is run commercially: NLLB-200 and Aya are licensed for non-commercial use
only.

Models that "think" before answering (Gemma 4, Qwen3) are asked not to, with `reasoning_effort: "none"`, which
Ollama honours: left to think, `gemma4:12b` took up to half a minute a line and sometimes ran out of tokens before
answering; asked not to, it answers in under a second. A server that refuses the field is asked again without it.

### On the machine with the GPU

```sh
ollama pull qwen2.5:7b
```

Ollama listens on `127.0.0.1:11434` only. If bingle runs on **the same machine**, that is all:
`BINGLE_TRANSLATE_URL=http://127.0.0.1:11434/v1`.

If it runs on **another machine of the network**, make Ollama listen there and let the port through:

- **Windows**: set the user environment variable `OLLAMA_HOST` to `0.0.0.0:11434` (search the Start menu for *Edit environment
  variables for your account*), and `OLLAMA_KEEP_ALIVE` to `30m` so the model stays loaded
  between lines. Quit Ollama from the tray and start it again. Then allow the port on private networks only, in an
  administrator PowerShell:

  ```powershell
  New-NetFirewallRule -DisplayName "Ollama (LAN)" -Direction Inbound -Protocol TCP -LocalPort 11434 -Profile Private -Action Allow
  ```

  Check that Windows calls the network *Private*, not *Public*.
- **Linux (systemd)**: `systemctl edit ollama` and add `Environment="OLLAMA_HOST=0.0.0.0:11434"` under
  `[Service]`, then restart it and open the port in the firewall for the local network only.

Give the machine a fixed address (a DHCP reservation on the router) so the URL does not change.

### On the bingle server

Check that the service answers, then set the two variables:

```sh
curl -s http://192.168.1.20:11434/v1/models
BINGLE_TRANSLATE_URL=http://192.168.1.20:11434/v1 BINGLE_TRANSLATE_MODEL=qwen2.5:7b ./bingle
```

If the machine with the model sleeps or is switched off, lines stay untranslated: after three failures in a row
the server leaves the service alone for a minute before it tries again, and chat never waits for it.

## What is sent, and what is checked

- **Only the line, and never what is not language in it.** Links, `@handles`, the names of the people present,
  emoji and move notation (`c3`, `a1-b2`, `e2e4`, `Nf3`, `O-O`) are replaced by markers such as `⟦1⟧` before the
  line is sent, and put back after. A line with no words left is not sent.
- **Fixed instructions.** The model is told what to do in a system message that never contains the line; the line is
  the user message alone, and the model is told it is text to translate, not instructions.
- **Answers are checked.** The model answers JSON `{source, text}` (asked for by schema). An answer is dropped, and
  the line shown as written, when a marker is missing, repeated or made up, when it has a line break or markup
  (`<`, `` ` ``, `**`, a Markdown link) the line did not, or when its length is far from the line's.
- **Moderation sees the original**, and the page shows a translation as plain text.

## Privacy and security

- Chat lines go to the service you name. Keep it on this machine or your own network; that is the point.
- **Such services usually have no authentication.** Anyone who can reach Ollama's port can use the model on your
  GPU (though not reach bingle through it). Open the port to the local network only,
  never to the internet.
- The server does not log what a line said, only what became of it.

## Metrics

On the private metrics listener ([observability.md](observability.md)), only when a service is set up:

| Series | What it counts |
|---|---|
| `bingle_translations_total{outcome}` | Lines asked to be put into a reader's language: `translated`, `same` (already in it), `skipped` (nothing to translate), `rejected` (the answer was dropped), `failed`, `dropped` (the queue was full), `paused` (the service was being left alone) |
| `bingle_translate_request_duration_seconds` | How long one request to the service took |

Many `rejected` mean the model is too small or does not keep the markers; many `failed` or `paused`, that the service
is down or the timeout too short.
