# Upstream and what changed

Derived from **emo-libre/Proxy** — https://github.com/emo-libre/Proxy, commit `46ab51c48bc6` (2026-07-22),
BSD-3-Clause, "Copyright (c) 2025, EMO Libre Team". The licence text is kept verbatim in `LICENSE.upstream`.

| From upstream | What happened |
|---|---|
| Config-file loading idea, `/time` handler, request-logging scaffold | kept in spirit, rewritten (`config.go`, `handlers_time.go`, `logging.go`) |
| Forwarding of `/token`, `/emo/*`, `/home/*`, `/download/*`, `/tts/*` and `/` to the vendor's hosts | **removed entirely.** This server makes no outbound connections; `no_forward_test.go` enforces it. |
| TLS verification disabled for outbound calls | removed with the forwarding |
| SQLite request database, `/proxy-api`, ChatGPT "replacements" | removed (not needed; they stored traffic) |
| Logging of every request header (including `Authorization`/`Secret`) and of POST bodies to disk | removed; headers and bodies are never logged or stored |

Bugs fixed relative to upstream:
- the `/time` response struct had the tag `jason:"offset"`, so the field was serialised as `Offset`; the documented key is `offset`.
- `/token/*` returned whatever the vendor returned; tokens are now issued locally.

New code: local token issuing (HS256), the OTA endpoints (`/emo/ota/version`, `/emo/ota/res/<n>`, `/emo/ota/allres/<n>`), firmware
file serving, local stubs for the other robot calls, HTTPS with a generated self-signed certificate, tests.
Wire shapes follow `emo-libre/Documentation` (`Server api.living.ai/*.md`, commit `007a02066241`) and `spec-clean/ota_wire.md`.
