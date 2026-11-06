# RESUME — emo-open-fw

Last updated: 2026-11-05 (Phase 1 code, original oscillator, seam + Mode 1 renderer, .mot player).

## State in one paragraph
Phase 1 code is written. The **portable protocol core** (`components/k210_link`) is built and tested on
the host (framing, CRC-8/MAXIM, frame parser, ack/retry link layer, tier 0–1 commands, sub-2 playlist
builder, SPI header) against a simulated K210. The **ESP-IDF glue** (`components/board`,
`components/k210_port`, `main/`) is written but has **never been compiled** (no ESP-IDF on the dev
machine) and **never run on hardware**. So: the firmware has NOT yet talked to a real K210.

## Run the checks (no ESP-IDF needed)
```
cmake -S tests/host -B build/host -G Ninja
cmake --build build/host
ctest --test-dir build/host --output-on-failure      # 14 tests (C under ASan+UBSan, Go server)
(cd server && go test ./...)                         # also run by ctest as server_go
(cd tools/emotool && go run . denylist-scan)         # must say CLEAN
(cd tools/emotool && go run . denylist-scan ../../build/host)   # scan build output too
```

## Server (pivot) — state
`server/` is complete as a unit-tested Go program (`go test -race ./...`, plus an end-to-end smoke run of the real binary over
TLS 1.2 / plain HTTP with mock backends). It has NOT been used with a real robot, DNS rewrite, or real STT/LLM/TTS servers.
Next for the server: (1) run it with `capture` on against the owner's robot and use `emotool summarize` (tools/emotool) to pin the [V]
items in `spec-clean/cloud_api.md` (voice-prompt fields, plain-HTTP speech download incl. port, `listen` semantics, kinds 4/5 and
0xC/0xD, anything under `/home` or `/app`); (2) try real backends (see `server/config.example.json`); (3) confirm the stock updater
accepts our manifest (empty `sd`/`model`, no `k210`).

## Languages and providers (2026-11-05, latest)
Go-only tooling (`tools/emotool`; Python removed). Embedded packs en/de/vi with a lexicon engine (`server/nlu_lex.go`): Vietnamese timers, alarms, colours,
cities, names, birthdays work. Test `TestVietnameseMeansTheSameAsEnglish` checks all 168 contributed pairs. New providers and presets: see `server/presets/`.
2026-11-05 (later): nine more language packs (de fr es it pt ru ja ko zh; zh/ja/ko use `_chars`), each with a lexicon and `testdata/<lang>_pairs.txt`;
tests `multilang_test.go` and `multilang_params_test.go`. Engine additions in `server/nlu_lex.go`: character tokens, CJK/Korean numerals, "at"-word and day-part clock
times, number joiners, date suffix style, city-before-weather-word, name suffixes, city forms. New: custom commands (`custom.go`), admin page and phrase tester
(`admin_ui.go`, `ui/admin.html`), `stt_preprocess` (`cleanPCM`), prompts for ru/ja/ko/zh. wire-pod studied for ideas only: `docs/wirepod_comparison.md`.
TO DO BEFORE PUBLISHING: written licence confirmation from the Vietnamese contributor (NOTICES); native-speaker review of non-English lines (every pairs-file header says
which are unreviewed); consider crediting ROBOTEKA; owner/counsel to accept shipping the vendor's short command phrases as test data.

## Server completeness (2026-11-05, later)
Added: drawing (SD-WebUI/OpenAI/command/mock backends), news (RSS), beta channel, own-content packs, admin API + telemetry whitelist,
`require_token`/`allowed_devices`, language packs (`server/packs/de.json`), LLM-assisted commands, prompts in en/de/fr/es/it/pt, graceful
shutdown, systemd example. 67 Go test functions (`go test -race ./...`; several are tables, e.g. 103 utterances), mutation-checked, plus an end-to-end run of the binary with everything on.
Still blocked: dialogue (key names unknown), `res.zip`. Non-English prompts and the German pack are not reviewed by native speakers.

## Server features (2026-11-05)
Intent engine (`server/nlu.go`, table-tested with 103 utterances + 30 non-command sentences), TTS disk cache + start-up pre-warm,
notices, permission. Plan and status per feature: `docs/lai_feature_matrix.md`. Measured here: fresh short line 51 ms
(espeak-ng → lame), cached 0.5 ms, 9 fixed prompts pre-warmed in 0.7 s. Not yet run against a real robot or a real STT/LLM server.

## Toolchain
ESP-IDF 5.2.8 is installed at `~/esp/esp-idf-v5.2.8` (`export PYENV_VERSION=3.10.21; . ~/esp/esp-idf-v5.2.8/export.sh`).
The firmware build was started and then paused at the owner's request — `idf.py build` has NOT been run yet.

## Next actions, in order
1. **Install ESP-IDF 5.2.x (pinned)** and run `idf.py set-target esp32 && idf.py build`.
   Expect to fix small API/component-name mismatches in `board` and `k210_port` — they were written without a compiler.
2. **Check partition table** with `idf.py partition-table`; `emotool partitions partitions.csv` guards the CSV.
3. **Spike 0a (bench, no robot damage risk):** flash only the app to a spare unit's `ota_1` (never the bootloader or
   partition table), set `otadata`, confirm the stock 2nd-stage bootloader boots an IDF 5 image.
   Then repeat through the stock OTA client from a LAN server (needs the stock-wire server — not written yet).
4. **Spike 0b — the K210 handshake (this is the "talks to the K210" milestone):** boot `main.c`, watch the log for
   `K210 TALKING: firmware number word=...` and `K210 version text`. Record the real byte layout of the cmd-60
   reply into `spec-clean/k210_link.md` (currently exposed as a raw 24-bit value).
5. Spike 0c: play something. Set `EMO_DEMO_AVI` / `EMO_DEMO_MP3` (menuconfig) to names of the owner's own content,
   confirm subs 24/25; then a sub-2 playlist; then the `/test` plain-path bypass with an open (unencrypted) AVI/MP3 on the SD.
6. Phase 2 remainder: a manifest-backed AssetProvider (owner's JSON manifest parsed with the in-tree cJSON — `components/json`, REQUIRES `json`),
   written as ESP-IDF glue because cJSON is not on the host test build.
7. Phase 3 remainder: servo pulse driver (RMT/MCPWM, safe-start/unload first), 20 ms `beat_timer` on gptimer, `.mot` loader
   (SPIFFS, 31-char truncation, K210 fetch), generator library on top of `osc_*`, touch/cliff/battery inputs.

## Reuse-first rule (owner's steer, 2026-11-05)
Before writing any generic component (sensor drivers, servo pulse output, OTA/stock-wire server, JSON, timers), search for an
existing permissively-licensed implementation (IDF components, IDF Component Registry, emo-libre projects), verify its licence
from the upstream repo, list it in `NOTICES`, and only then decide. Own code is for EMO-specific logic with no upstream.
Server: start from `emo-libre/Proxy` (BSD-3) rather than writing the stock-wire server from scratch.

## Licence decisions (resolved)
Otto `Oscillator` is GPL-3.0 and is NOT used; `components/motion` is an original implementation (owner chose option A).
ESP-IDF is pinned to 5.2.x. Everything in `NOTICES` is permissive.

## Upstream-modified code
`spec-clean/upstream_deltas.md` lists the 7 places stock modified Espressif code, what really differs, and the phase each
belongs to. Rule: build on current upstream IDF/examples; implement only the behavioural delta.

## Verify on hardware (every [I]/[V] in spec-clean)
- cmd-60 reply byte layout; whether cmd 105's reply text includes the NUL.
- GPIO22 really is the K210 reset; GPIO23 really is power-hold; behaviour if pulsed after the soft reset.
- Whether sub 24–27 bodies want the NUL; whether sub 2 wants it.
- Units of `t` in playlist sounds; meaning of `loop` values other than 0/1.
- Whether the ESP must ack K210→ESP payload frames.
- I²C pull-ups (we enable internal ones); battery divider ratio; PSRAM mode/size.
- What a half-delivered payload does to the K210 parser after a timeout (retry could desync it) — see DV-003.
- `/test` bypass actually plays plain files on the stock K210.
- Whether the stock OTA client tolerates a manifest with no K210 entry.
- Missing pins: servo-board UART2, I²S audio.

## Not done / out of scope this session
- No stock-wire OTA server, no Wi-Fi/web/REST, no servos/`.mot`, no vault, no brain.
- `audio_eq` is specified in PLAN.md only.
- No git commit has been made (repo is initialised; nothing committed).
- No LICENSE file yet — the owner has not chosen one.

## File map
- `spec-clean/` the only spec; `docs/deviations.md` every deviation; `PLAN.md` plan of record.
- `components/motion/` sinusoidal motion engine (`osc_*`); `components/mot/` .mot validate/play; `components/seam/` AssetProvider + Renderer + Mode 1 K210 renderer. All portable and host-tested.
- `components/k210_link/` portable core (no IDF includes). One function or tight group per file.
- `components/k210_port/` UART + SPI transports (IDF). `components/board/` GPIO/I²C/ADC.
- `tests/host/` unit tests + simulated K210 (`sim_k210.h`). `tools/` partition check, denylist scan/gen.

2026-11-05 (latest): `listen_admin` (separate admin listener), `activity_log`, `/admin/setup|selftest|say|activity`, and a built-in DNS helper (`dns.go`, `dns_*` config).
README has "How it works" and "Open the admin page". Real-binary check done: UI 200, no-token 401, admin absent from the robot port, DNS answered, a timer request logged.

2026-11-05 (latest+1): first-run flow (`./emoserver` writes config.json + data/admin_token), full admin UI rewrite (`ui/admin.html`: Home/Setup/Test/Your commands/Activity/Help),
settings + API-key + custom-command editing (`config_edit.go`, `secrets.go`; opt-in `admin_allow_edit`, no generic command provider), managed engines (`backend/local_engines.go`:
whispercpp/espeak/piper), `detect.go` (installed tools, presets, capture report). Browser-tested with headless Chromium (login, wrong token, preset, save, checks, phrase tester, commands, tabs).
A real EMO on the LAN connected to a fresh test server and completed its boot calls (time, token, report, version, notice, weather, permission, server list: all 200). Capture (Setup > options) is ready to run.
NOT verified: the real whisper-server request format, Piper, DHCP-assigned DNS on the robot, Docker (none shipped).

2026-11-05 (latest+1): first-run flow (`./emoserver` writes config.json + data/admin_token), full admin UI rewrite (`ui/admin.html`: Home/Setup/Test/Your commands/Activity/Help), open to the LAN
(`listen_admin` ":8090" + `listen_admin_tls` ":8444", per-address throttle, admin URLs printed and listed in Help), accessibility pass, settings + API-key + custom-command editing (`config_edit.go`, `secrets.go`;
opt-in `admin_allow_edit`, no generic command provider), managed engines (`backend/local_engines.go`: whispercpp/espeak/piper), `detect.go` (installed tools, presets, capture report).
Browser-tested with headless Chromium over CDP (login, wrong token, preset, save, checks, phrase tester, commands, all tabs; 24-view accessibility audit incl. dark mode and 375 px).
A real EMO on the LAN connected to a fresh test server and completed its boot calls (time, token, report, version, notice, weather, permission, server list: all 200).
NOT verified: the real whisper-server request format, Piper, DHCP-assigned DNS on the robot, a real screen reader, Docker (none shipped). Capture (Setup > options) is ready to run.

2026-11-05 (persona, step 1 of the app-fix plan): the capture from the owner's robot showed generic "as a large language model" replies because the model got
no system prompt. New `server/persona.go`: built-in EMO character (original text) sent first on every model call (free chat and `llm_commands`), with the
robot's language named ("Always reply in German."); `"persona"` config key = own text or `"off"`. OpenAI-compatible LLMs default to `max_tokens` 100.
Old one-line `system_prompt` removed from config.example.json and the three presets (it would stack). Tests: `persona_test.go` (mutation-checked).
Checked live against local llama3.2 (3B): in character, 3-12 s per reply on this machine. Steps 2-5 done the same day (below).

2026-11-05 (app-fix plan, steps 2-5, all with tests; `go test -race ./...` + ctest 14/14 + denylist CLEAN):
(2) `isChatModeRequest` (nlu.go): "chat/ChatGPT mode" incl. the robot's real mis-hearings; whisper.cpp gets an English word hint (`whispercpp.prompt`), and a
transcript that only repeats the hint counts as silence. Live whisper base.en: hint does not parrot on silence, but 3 s of silence transcribes as "you".
(3) `tidyReply`: drops *stage directions*, free-standing emoticons, curly quotes; keeps **bold** words, times, numbers.
(4) normal free chat remembers 3 turns per robot, 5 min idle (`free_chat_turns`, -1 = off), separate from chat mode; per-stage timings (`stages.go`) in the log
line, activity log and capture (`ms`). (5) owner's config.json: local ollama `llama3.2:3b` (same id as :latest, a80c4f17acd5), max_tokens 100; openrouter dropped.
.gitignore now covers server/data (keys, tokens, secrets, captures), local config*.json and the binary -- nothing had been committed.
FOUND BY THE NEW TIMINGS (live run, real ollama + Kokoro, not yet fixed): first question after ollama idles fails -- model load 23.7 s + TTS hit the 25 s
backend timeout; Kokoro TTS takes 9-13 s per ~30-word line (slower than the model, ~6 s). Warm totals 15-18 s; the robot's own patience is unknown [V].
Next: decide keep-alive/warm-up for the model and a faster or shorter TTS path; then try on the robot.

2026-11-05 (reply budget; from the owner's interop notes): the robot appears to wait only ~10 s after the voice upload, then ignores the reply
(recorded in spec-clean/cloud_api.md + SOURCES.md as single-sourced [I]; nothing copied). Server changes, all tested (mutation-checked):
`reply_budget_ms` (default 9000, -1 = old 25 s) deadline from the end of the upload; `tts_fallback` fast second voice used when the main voice would
be late (gets budget - 1 s; fallback audio never cached); `keep_alive` for Ollama (whole number sent as a JSON number -- the string "-1" is a 400);
model warm-up at start-up (`warm.go`); `debug_reply_delay_ms` to find the robot's real cut-off; persona "under 20 words", max_tokens default 60.
Owner config: keep_alive -1, max_tokens 60, tts_fallback espeak. Live: cold load handled at start-up (13 s), model shows "Forever"; with Kokoro as main
voice it missed every deadline (fallback spoke) and its abandoned jobs slowed the next reply; espeak-only: 6/6 within budget (3.9-9.0 s, STT not counted).
Laptop: 8 threads, 7 GB RAM, deep in swap, Kokoro is the CPU Docker build. Piper not installed (PyPI piper-tts >= 1.3 is GPL-3.0 piper1-gpl; owner's call).
NEXT (robot session): run ./emoserver, set debug_reply_delay_ms 8000/9000/10000/11000 with "dance" to find the cut-off, then set reply_budget_ms;
decide main voice (espeak now, Piper if approved, Kokoro only on a faster machine); add real STT time to the budget picture.

2026-11-05 (first robot session with the new pipeline): main voice now Piper `en_GB-jenny_dioco-medium` (rhasspy/piper 2023.11.14-2 standalone,
MIT, in ~/.local/opt/piper; voices in ~/.local/share/piper-voices; Jenny's dataset licence: commercial OK, credit the voice as "Jenny (Dioco)").
Chosen by test: 0/31 whisper word errors, ~0.9 s a line (Kokoro 4.9 s, espeak 0.03 s but robotic). Kokoro container stopped (`docker start kokoro-tts`);
frees ~2.5 GB. Robot results: 7 of 8 model replies accepted (robot fetched the mp3 at once), latest 8.0 s after the upload / 12.0 s after its
start -> the window counts from the upload's end (spec-clean updated). One lost: model 7.2 s. Fixes after the session (tested, mutation-checked):
whisper warm-up at start-up (first request had paid 3.8 s engine start); streaming `ChatPartial` -- the model stops 2.5 s before the deadline and
the finished sentences are spoken; whisper silence phantoms ("you", "thanks for watching") read as nothing heard.
NEXT: measure the exact cut-off (`debug_reply_delay_ms`), then tune reply_budget_ms; the server runs from a Claude session in the background --
use `server/emoserver.service` (systemd example) to keep it up for good.

2026-11-06 (admin page redesign, `server/ui/admin.html`): owner chose the "Tabbed Manual" look (Impeccable design skill, code-led): every section has its own
colour tab on a stepped rail; the open section's colour is the board under a milk-white sheet; vermilion only for problems. Home now shows the robot, engine
name plates (Listening / Thinking / Voice with the engine in use), try-a-phrase, hear-the-voice and recent answers with stage-timing bars against the ~10 s
window. Technical settings moved to a new Expert tab (options, DNS helper, server, installed tools, capture, languages). Fixes found on the way: saving Setup
dropped `tts_fallback` (now starts from the saved engines); an engine type the page does not offer (command, mock, static) was shown as Off and saved as
off (now "As set in the config file" and kept). Checked in headless Chromium at 1440 and 390, light and dark, real-keyboard focus; independent finish
review: ship. Product/design records: PRODUCT.md, DESIGN.md, `.impeccable/design.json`, `.impeccable/surfaces/` (commit these); `.impeccable/review/` is
ignored (screenshots show local paths and LAN addresses). Tooling installed for the owner: skills apple-design (ff4ebdd) and impeccable (c8891a9, skill only,
no hook; its telemetry is turned off per command with IMPECCABLE_NO_TELEMETRY=1), Playwright MCP 0.0.83 at user scope (restart Claude Code to use it).
Open: product name (placeholder "EMO open server"); a self-hosted condensed display face needs a licence decision; Go tests do not parse the page's
script (a syntax error broke the page once during this work and only the browser run caught it).

2026-11-06 (later): whisper hints for every command: `WhisperHintEN` now lists the robot's unusual command names (games, features), and the server adds the
owner's own command phrases per language (`stt_hint.go`, capped at 600 chars; `HintedSTT` interface; the `whispercpp.prompt` setting still wins). Silence
guard: a transcript that is the hint or two hint items back to back counts as nothing heard. Live on whisper base.en: "rock paper scissors" went from
"rock paper system" to correct; 3 s of silence now gives [BLANK_AUDIO] instead of "you". "Tic Tac 2" (heard on the robot) is accepted for tic tac toe.
Persona now answers first with the real fact, then one playful line; honest about news/scores/time; under 30 words. Replies cut by the token cap end on
their last whole sentence (`endOnSentence`). Local model temperature 0.5 (owner config, example, local preset): at 0.8 the 3B model mixed itself up
("I have eight legs!"). Live: correct answers in 5.2-7.6 s through the real handler.
