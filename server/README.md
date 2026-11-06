# emoserver — a fully local server for the stock robot

Runs the robot's whole cloud side on your own network: time, token, firmware update, weather, speech and conversation.
**It never contacts the robot vendor** (no forwarding; vendor domains are refused even if configured). The only outbound
connections are to the speech/language/weather services *you* configure — local models on your LAN, or an API of your choice.
Derived from emo-libre/Proxy (BSD-3-Clause), see `UPSTREAM.md`. Wire contract: `../spec-clean/ota_wire.md`, `../spec-clean/cloud_api.md`.

## How it works, in one minute
```
 EMO robot ──(its normal cloud calls: time, token, update, weather, speech)──▶  emoserver  (this program, on a machine in your home)
                                                                                  │
     the robot's vendor host name points here (your router, or the built-in DNS helper) - nothing is sent to the vendor
                                                                                  │
 1. the robot records you ──▶ POST /emo/voice/detectintent ──▶ speech-to-text (a program/server you choose: local or online)
 2. the text is matched ──▶ your own commands, then the built-in commands in 11 languages (microseconds, no model)
 3. understood ──▶ a short answer tells the robot what to do (timer, alarm, animation, light, game, weather, ...)
    not understood ──▶ optionally a language model answers, and text-to-speech turns the reply into an MP3 the robot downloads
 4. time, token, firmware updates and weather are answered by the server itself
```
Everything outside the box (speech-to-text, language model, voice, weather, news, drawing) is a plug you choose: a program on your own machine, a
local model server, or an online API with your own key. Each plug is optional; a missing one just switches that feature off. Nothing here talks to the
robot vendor.

**EMO's character.** Whatever language model you plug in, the server tells it to answer as EMO: a small desk robot, one or two short spoken
sentences, never "as an AI" or "as a language model", in the robot's language (`persona.go`; original text written for this project). Set
`"persona"` in `config.json` to your own text to replace it, or to `"off"` to send none. A backend's own `system_prompt` is added after it. With
no `max_tokens` set, OpenAI-compatible models (Ollama included) are capped at 60 tokens so a rambling model cannot produce a minute of speech.

**Follow-ups.** Outside chat mode EMO still remembers the last few exchanges with each robot (`"free_chat_turns"`, default 3; `-1` = none), and
forgets them after 5 minutes of quiet, so "how tall is it?" after "what is the tallest animal?" makes sense. Chat mode keeps its own longer memory
(`"history_turns"`). Saying "chat mode" or "ChatGPT mode" starts chat mode, including common mis-hearings ("chat GPP mode", "Jack Cheepity mode").
Transcripts whisper invents from silence (a lone "you", "thanks for watching") count as nothing heard. With whisper.cpp, English recordings are sent a short word hint ("EMO, ChatGPT."; `"prompt"` under `whispercpp`: your own text or `"off"`).

**Where the time goes.** Every voice request logs one line such as `voice en: stt 812 ms, llm 4301 ms, tts 52 ms; total 5200 ms; conversation reply`
(never the words), the activity page keeps the same numbers, and capture records how long each answer took (`ms`).

**Answer fast, or not at all.** The robot appears to stop waiting about 10 s after it finishes sending your voice, and ignores a later
answer (`spec-clean/cloud_api.md`, still to be confirmed on a robot). So a voice reply must be ready within `"reply_budget_ms"` of the
upload (default 9000; `-1` = the old 25 s). Three settings help it fit:
- `"keep_alive"` under an OpenAI-compatible model (Ollama: `"-1"` = keep it loaded for good, or `"30m"`). The server also loads the model at
  start-up (skipped with `no_prewarm`). A cold model load takes longer than the robot waits.
- `"tts_fallback"`: a fast second voice (for example `{"type": "espeak"}`). When the main voice would finish too late (it gets the budget
  minus 1 s), the fallback speaks that reply instead. Fallback audio is never cached as the main voice.
- Streaming: an OpenAI-compatible model (Ollama included) must stop 2.5 s before the deadline, leaving time to speak. If it is still
  writing, EMO says the whole sentences written so far instead of nothing (log: `model out of time`).
- `"debug_reply_delay_ms"` (testing only): hold every voice reply until this long after the upload. Say a simple command ("dance") at
  8000, 9000, 10000, 11000 ... to find the moment your robot stops listening, then set `reply_budget_ms` a little below it.

On a CPU-only machine a neural voice server (Kokoro) can take longer than the whole budget, and it keeps working on a reply after the
server has given up on it, which slows the next answer. If the log shows `the fallback voice speaks` on most replies, use the fast voice
as the main one.

## Quick start (first run)
```
cd server && go build -o emoserver .
./emoserver
```
With no config file it writes a starter `config.json` next to it, makes an admin token, and prints:
```
FIRST RUN: wrote a starter config to config.json ...
ADMIN TOKEN (shown once; it is kept in data/admin_token): ...
OPEN THE ADMIN PAGE: http://127.0.0.1:8090/admin/ui
```
Open that address, paste the token, and open **Setup**: its checklist shows what is still open; pick a starting point (everything on your own computer, free online services, or Claude), fill in the boxes,
paste any API keys, **Save**, press **Restart**. **Home** then shows whether Listening, Thinking and Voice work (one click checks them) and lets you try a phrase and hear the voice; **Try it** has the full
tester. The **Help** tab has copy-and-paste steps for pointing the robot here and for installing whisper.cpp, espeak-ng and Ollama; technical options live on **Expert**. Nothing in the starter config
contacts anything outside this computer. A step-by-step beginner guide is in the [main README](../README.md).

If you lose the token it is in the file `data/admin_token`. To reach the page from another computer use an SSH tunnel (`ssh -L 8090:127.0.0.1:8090 user@server`); the page is plain HTTP on the loopback address
on purpose. Without `admin_auto_token` or `admin_token_env` none of the admin pages exist.

## What the admin page can do
| Tab | |
|---|---|
| Home | whether your robot has connected; Listening / Thinking / Voice as name plates showing the engine in use (one click checks each and shows its speed); try a phrase; hear the voice; the last answers with bars showing where the time went against the robot's ~10 s patience |
| Setup | the setup checklist; quick-start presets; this computer's address (found for you); a menu per service (listening, chatting, voice, weather, news, photos, drawing) with only the boxes that service needs; API keys; Save and Restart. An engine set in the config file that the page does not offer is shown "as set in the config file" and kept on Save |
| Try it | try any phrase in any language (with the exact reply), hear the voice, check every engine |
| Your commands | add phrases of your own that speak, play an animation, or replace a built-in command; works at once, no restart |
| Activity | what the robot heard, what was done and how long each answer took (opt-in, memory only, last 100) |
| Help | point the robot at the server, other addresses to open this page at, install guides with copy buttons |
| Expert | for tinkerers: options (chat fallback, model-picked commands, recording clean-up, activity log, capture), the DNS helper, server uptime and request counts, what is installed here, the robot's own request list (capture), loaded languages |

Each tab has its own colour, and the open tab's colour is the background, so you can always see where you are.

**Editing is opt-in and bounded.** Only with `"admin_allow_edit": true` (the starter config sets it; otherwise the page is read-only). The page can change the services, the address, the options and the DNS helper, your own
commands (not programs), and API keys. It cannot change listen addresses, the admin token, access control, or choose the generic "run any program" provider: a leaked admin token therefore cannot make the server run
a program. Every save is checked exactly as at start-up, written atomically, and the previous file is kept as `config.json.bak`. Keys you paste are stored in `data/secrets.json` (mode 0600), exported under the key
names the config uses, and never sent back to any page; an environment variable of the same name overrides the file.

## Engines the server runs for you
| Config `type` | What it is | You install |
|---|---|---|
| `stt` `whispercpp` | the server starts `whisper-server` (whisper.cpp) on a loopback port, keeps the model loaded, restarts it if it stops, stops it on shutdown | whisper.cpp and one `ggml-*.bin` model |
| `tts` `espeak` | espeak-ng + lame with a fixed command line; the voice follows the robot's language | `espeak-ng`, `lame` |
| `tts` `piper` | Piper + lame, a small natural voice | `piper`, a `.onnx` voice, `lame` |
The server itself does not contain a speech recogniser (that needs a model file of 75 MB or more); it launches and watches the one you install. The `whispercpp` request format (`/inference`) is written from
memory of whisper.cpp's server and tested only against a stand-in program: [V] until run against the real thing.

## Quick start (first run)
```
cd server && go build -o emoserver .
./emoserver
```
With no config file it writes a starter `config.json` next to it, makes an admin token, and prints:
```
FIRST RUN: wrote a starter config to config.json ...
ADMIN TOKEN (shown once; it is kept in data/admin_token): ...
OPEN THE ADMIN PAGE from this computer or any device on your network:
    http://127.0.0.1:8090/admin/ui          http://192.168.1.10:8090/admin/ui
    https://192.168.1.10:8444/admin/ui      (and one line per network address of this computer)
```
Open one of those addresses on this computer, **or on your phone or any other device on your home network**, paste the token, and follow the **Home** checklist and the **Setup** tab: pick a starting point
(everything on your own computer, free online services, or Claude), fill in the boxes, paste any API keys, **Save**, press **Restart**. The **Test** tab then checks every service with one click and lets you hear
the robot's voice. The **Help** tab has copy-and-paste steps for pointing the robot here and for installing whisper.cpp, espeak-ng and Ollama. Nothing in the starter config contacts anything outside this computer.

**Who can open it.** The starter config opens the page to your whole local network (`listen_admin: ":8090"`, plus the same page over HTTPS on `:8444` with the certificate the server made for itself). Anyone who can reach it still
needs the token; eight wrong tokens from one address in a minute lock that address out for a minute; the token travels in a request header (so another website cannot use it), and the robot-facing port carries none of the admin
pages. The plain `http://` address is not encrypted: fine on a home network you trust, otherwise use the `https://` address (your browser warns about the self-signed certificate; accept it) or set `"listen_admin": "127.0.0.1:8090"`
and `"listen_admin_tls": ""` to keep the page on this computer only (then reach it with `ssh -L 8090:127.0.0.1:8090 user@server`). If you lose the token it is in `data/admin_token`. Without `admin_auto_token` or
`admin_token_env` none of the admin pages exist.

## What the admin page can do
| Tab | |
|---|---|
| Home | whether your robot has connected; Listening / Thinking / Voice as name plates showing the engine in use (one click checks each and shows its speed); try a phrase; hear the voice; the last answers with bars showing where the time went against the robot's ~10 s patience |
| Setup | the setup checklist; quick-start presets; this computer's address (found for you); a menu per service (listening, chatting, voice, weather, news, photos, drawing) with only the boxes that service needs; API keys; Save and Restart. An engine set in the config file that the page does not offer is shown "as set in the config file" and kept on Save |
| Try it | try any phrase in any language (with the exact reply), hear the voice, check every engine |
| Your commands | add phrases of your own that speak, play an animation, or replace a built-in command; works at once, no restart |
| Activity | what the robot heard, what was done and how long each answer took (opt-in, memory only, last 100) |
| Help | point the robot at the server, other addresses to open this page at, install guides with copy buttons |
| Expert | for tinkerers: options (chat fallback, model-picked commands, recording clean-up, activity log, capture), the DNS helper, server uptime and request counts, what is installed here, the robot's own request list (capture), loaded languages |

Each tab has its own colour, and the open tab's colour is the background, so you can always see where you are.

**Accessibility.** The page is plain HTML with no outside scripts or fonts, works with a keyboard only (skip link, visible focus, focus moves to the section title when you change tab), names every field and table for screen readers,
announces saves and errors, keeps status in words as well as colour, follows the system light/dark setting, respects "reduce motion", has 44-pixel touch targets and lays out down to a 375-pixel phone screen. The redesign (2026-11) was checked in
a headless browser (every tab, light and dark, 1440 and 390 pixels wide, focus reached with the Tab key, no sideways scrolling, no script errors); text contrast was measured for the earlier design and not again since, and a person using a real screen reader has not tried it.

**Editing is opt-in and bounded.** Only with `"admin_allow_edit": true` (the starter config sets it; otherwise the page is read-only). The page can change the services, the address, the options and the DNS helper, your own
commands (not programs), and API keys. It cannot change listen addresses, the admin token, access control, or choose the generic "run any program" provider: a leaked admin token therefore cannot make the server run
a program. Every save is checked exactly as at start-up, written atomically, and the previous file is kept as `config.json.bak`. Keys you paste are stored in `data/secrets.json` (mode 0600), exported under the key
names the config uses, and never sent back to any page; an environment variable of the same name overrides the file.

## Engines the server runs for you
| Config `type` | What it is | You install |
|---|---|---|
| `stt` `whispercpp` | the server starts `whisper-server` (whisper.cpp) on a loopback port, keeps the model loaded, restarts it if it stops, stops it on shutdown | whisper.cpp and one `ggml-*.bin` model |
| `tts` `espeak` | espeak-ng + lame with a fixed command line; the voice follows the robot's language | `espeak-ng`, `lame` |
| `tts` `piper` | Piper + lame, a small natural voice | `piper`, a `.onnx` voice, `lame` |

The server itself does not contain a speech recogniser (that needs a model file of 75 MB or more); it launches and watches the one you install. The `whispercpp` request format (`/inference`) is written from
memory of whisper.cpp's server and tested only against a stand-in program: [V] until run against the real thing.

## Open the admin page without the first-run flow
1. Put a secret in an environment variable and name it in the config: `"admin_token_env": "EMO_ADMIN_TOKEN"`, then start with `EMO_ADMIN_TOKEN='pick-a-long-secret' ./emoserver -c config.json`.
2. Add `"listen_admin": "127.0.0.1:8090"` (this computer only) or `":8090"` (the whole network) and optionally `"listen_admin_tls": ":8444"`.
3. With neither, the page is on the robot-facing HTTPS port: `https://<server>:8443/admin/ui` (self-signed certificate: accept the warning).

## Pointing the robot here: the built-in DNS helper
Set `dns_listen` (usually `":53"`), `dns_names` (the host name the robot asks for), `dns_answer_ip` (this machine's address) and optionally `dns_upstream` (your router, `192.168.1.1:53`).
Then set **the robot's DNS server** (a DHCP reservation, or the router's DNS setting for that device) to this machine. The helper answers those names with `dns_answer_ip`
and passes every other name to the upstream (or refuses them if you give none, which suits a resolver used only by the robot). Port 53 needs root or
`setcap cap_net_bind_service=+ep emoserver`. Only UDP, standard queries. Using your router's own DNS override instead is just as good if it has one.

## What it serves
| Robot calls | Served by |
|---|---|
| `GET /time`, `GET /token/<id>` | built in (tokens signed with a key generated on first start) |
| `GET /emo/ota/version`, `/emo/ota/res/<n>`, firmware file | built in; serves only the files named in the config |
| `GET /emo/weather/forecast` | `backends.weather` (`none`, `static`, or `open-meteo` — opt-in, the city name is sent to it) |
| `GET /emo/speech/tts`, `/emo/chat/start`, `POST /emo/voice/voiceprompt` | `backends.tts` (+ our own spoken lines, `prompts_file` to override) |
| `POST /emo/voice/detectintent` | `backends.stt` → native-style intent engine (`nlu.go`: 56 intents — time/date/battery, timers, alarms, lights, volume, games, music, movement, names/birthdays, weather, photos, …) → otherwise `backends.llm` → `backends.tts` |
| `POST /emo/ai/imgrecog` | `backends.vision` (a vision-capable chat model); 501 if not configured |
| notices, permission, server list, telemetry | notices from `notices_file` (edit live), `permission` config, server switch, telemetry discarded |
| `POST /emo/ai/draw/txttoimg`, `imgtoimg` | `backends.image` (`sdwebui`, `openai`, `command`, `mock`); sized/encoded by `draw` |
| news request ("what's the news") | `backends.news` (RSS/Atom feeds) → one spoken clip per headline |
| `res` manifest extras | beta channel, and your own media from `content_dir` |
| `/emo/dialogue/*` | 501 — blocked: the reply's key names are unknown until a capture shows them |
| `/admin/*` (page, status, robots, setup, self-test, activity, phrase tester, ...) | only if `admin_token_env` is set; telemetry kept is a short whitelist, in memory |

Generated speech is stored in memory for an hour and downloaded by the robot over **plain HTTP** from `listen_audio`
(the URL is built from `audio_base_url`); each file needs its unguessable token.

## Backends (pluggable, all optional, local or online)
Each of `stt`, `llm`, `vision`, `tts`, `image`, `news`, `weather` takes a `type`. Keys are read from environment variables whose NAMES you put
in `api_key_env`; they are never in the config file, never logged, and never part of an error message. Every provider is bound to its own host
and the robot vendor's domains are refused.

| type | for | notes |
|---|---|---|
| `openai` | stt, llm, vision, tts | any OpenAI-compatible API: OpenAI, Groq, Gemini (`/v1beta/openai/`), OpenRouter, Ollama, llama.cpp, vLLM, LocalAI, faster-whisper servers |
| `anthropic` | llm, vision | Anthropic Messages API (paid). Photo understanding included. |
| `google` | stt, tts | Google Cloud Speech-to-Text / Text-to-Speech (API key, monthly free allowance) |
| `elevenlabs` | tts | ElevenLabs (free allowance; needs `voice_id`) |
| `command` | stt, tts, image | run a local program, no shell. `{wav}` `{lang}` `{out}` `{in}` `{init}`; TTS text on stdin; `convert_args` makes MP3 |
| `sdwebui` | image | Stable Diffusion WebUI (the robot's drawing parameters are its own) |
| `pollinations` | image | free, keyless, text-to-image only (the prompt is sent to that service) |
| `rss` | news | RSS/Atom feeds |
| `open-meteo`, `openweathermap`, `weatherapi`, `static` | weather | Celsius only; the robot converts for display |
| `mock` | all | offline fakes |

Ready-made setups: `presets/local.json` (everything on your machine), `presets/free-online.json`, `presets/anthropic.json`; see `presets/README.md`.

## Languages
- Robots speak to the server in their configured language (`languagecode`). English has everything built in.
- Ten more languages ship embedded in the binary, each as a pack (`packs/<lang>.json`): **German, French, Spanish, Italian, Portuguese, Russian,
  Japanese, Korean, Chinese (Simplified), Vietnamese**. Every pack covers the whole command list **and** its own lexicon, so timers, alarms (time of day,
  weekdays, tags), light and eye colours, weather for a named city, "my name is…", birthdays, "I choose 5" and the 16 animals work in all of them.
  A `packs_dir` adds or extends packs without rebuilding.
- A pack maps command keys to phrases. `"_fold": true` ignores accents; `"_chars": true` is for languages written without spaces (Chinese, Japanese,
  Korean): each character is a token. `"_lex": {...}` teaches the server how the language writes numbers, time of day, durations, weekdays, colours,
  ordinals, cities, names and dates (see `packs/vi.json`, `packs/de.json` and `packs/zh.json` for three different shapes). Unknown command keys are an error
  at start-up.
- Where the lists came from, and how they are checked: the English side of each `testdata/<lang>_pairs.txt` is the vendor's published command list
  (used as a coverage checklist); the other side comes from the vendor's per-language lists, the Vietnamese contributor, and lines translated for this project
  (those are marked and **not reviewed by native speakers**). A test requires every pair to mean exactly what the English one does, with accents stripped too,
  and a second test checks other numbers, times, weekdays, cities and names written the way a speech-to-text transcript would give them.
- Two vendor lines had typos and the fixtures use corrected wording (Portuguese timer lines said "alarme"; one said "alarque"). Where a vendor list gave one phrase
  to two commands (Russian "Включи музыку"), the phrase goes to the Bluetooth-speaker command.
- Chinese is Simplified only; Traditional characters, Japanese kana/kanji variants for the same word, and regional spellings are matched only where listed.
  Add them in a `packs_dir` file.
- `GET /admin/languages` lists the loaded packs; the admin page (below) tries any phrase in any language.

## Your own commands
`custom_commands_file` points at a JSON file of your own phrases. They are tried before the built-in ones, so they can also replace a built-in phrase, and the file
is re-read whenever it changes (a broken edit keeps the previous commands):
```
{"commands": [
  {"name": "party",  "phrases": ["party mode", "let's party"], "say": "Party time!"},
  {"name": "wave",   "phrases": ["do the wave"], "animation": "Hi"},
  {"name": "dance2", "phrases": ["go dancing"], "intent": "my_dance", "behavior": "dance", "paras": {"dance_type": "to_music"}},
  {"name": "radio",  "lang": "en", "phrases": ["turn on the radio"], "exec": ["/usr/local/bin/radio", "on"], "speak_output": true}
]}
```
Phrases can be in any language (`lang` limits a command to one). A phrase must make up most of what was said, unless the command sets `"contains": true`
(anywhere in a sentence) or `"exact": true`. A command does one of: speak (`say`), play an animation, send an explicit robot command (`intent` + `behavior`
+ `paras`), or run a program. Programs run only if the config sets `custom_commands_allow_exec`: the path must be absolute, there is no shell, the environment is
fixed, `timeout_s` is 1-30 (default 5), at most 2000 bytes of output are kept, and what a program prints is spoken only with `speak_output` and is never cached.

## Admin page
With `admin_token_env` set, `/admin/ui` is a small page (no outside scripts or fonts): enter the token, see status, the loaded languages and your own commands, and
**try a phrase** in any language to see exactly what the robot would be told. The phrase tester calls no online service and nothing reaches the robot. The same
data is available as `GET /admin/parse?lang=..&text=..`, `/admin/languages`, `/admin/commands`.

## Fast and light speech
Speech never has to be fancy to be fast. What the server does for speed:
- **Fixed lines are instant.** Refusal prompts, greetings and the robot's scripted text are synthesised once (pre-warmed at start-up in the
  background) and cached on disk (`tts_cache_mb`, default 128, `-1` = off). Conversation replies and photo descriptions are never cached.
- **Commands never call a model** (rules, microseconds).
- **Small MP3s:** mono, 24 kHz, 32 kbit/s ≈ 4 KB per second of speech.

A tested light chain (no GPU, ~50 ms for a short sentence on a desktop CPU), using `espeak-ng` and `lame`:
```
"tts": { "type": "command", "command": {
  "args": ["espeak-ng", "-v", "en-us", "-s", "165", "-p", "60", "-w", "{out}", "--stdin"],
  "convert_args": ["lame", "--quiet", "-m", "m", "-b", "32", "--resample", "24", "{in}", "{out}"],
  "out_format": "wav" } }
```
It sounds robotic, which suits a robot. For a more natural voice with similar speed use Piper (a small local neural voice): same shape, with
`args` = `["piper", "--model", "<voice>.onnx", "--output_file", "{out}"]` (text on stdin) and the same `convert_args` (not tested here).
Any OpenAI-compatible `/audio/speech` server also works (`"type": "openai"`). MP3 sample rate/bitrate tolerance of the robot's player is unconfirmed.

## Access control, admin, content, languages
- `require_token`: every `/emo/*` call needs a token this server issued. `allowed_devices`: only these robots (12 hex chars) get tokens.
  Turn both on after a capture shows every robot call carries its token.
- `admin_token_env`: name of an environment variable holding a token; enables `GET /admin/status` and `/admin/robots`
  (`Authorization: Bearer <token>`). Telemetry kept: battery, Wi-Fi level, version-type numbers; location, time zone, language, age are dropped.
- `content_dir`: your own `mp3/ avi/ mot/ json/ model/` files; listed in the update manifest and served read-only. Nothing is bundled.
- `packs_dir`: `<lang>.json` command phrases for more languages or extra phrases (see Languages). Keys are validated at start-up, so a typo
  is an error, not a silent no-op.
- `stt_preprocess`: before speech-to-text, remove the recording's DC offset and rumble (80 Hz high-pass) and lift a quiet recording to about 70% of full scale
  (never more than 8x; silence and loud recordings are left alone). Off by default: untested on a real robot's microphone.
- `llm_commands`: one model call that returns either a command from a safe list or a short reply (never power-off or update-install).
- `draw`: `format` png|jpeg|passthrough, `max_dim` shrink, `concurrency`.
- Spoken refusal and event lines exist in en, de, fr, es, it, pt, vi, ru, ja, ko, zh; the non-English ones are unreviewed. Override any line with `prompts_file`.

## Run
```
cd server && go build -o emoserver .
./emoserver                 # first run: makes config.json and an admin token, prints the admin address (see Quick start)
./emoserver -c config.json  # later, or with your own file (config.example.json lists every setting)
mkdir -p firmware && cp <your image>.bin firmware/emo-open-fw.bin   # only needed to update the robot's firmware
```
A self-signed certificate (TLS 1.2, RSA) and the token key are written to `data_dir` on first start.

## Point the robot at it (no firmware change)
1. DNS: on your LAN resolver (dnsmasq, AdGuard Home, router override) make the robot vendor's default API host name resolve to this machine.
2. Port: the robot connects to 443. Either redirect 443→8443 on the host (`nftables`/`iptables`) or use `"listen": ":443"` with
   `setcap cap_net_bind_service=+ep emoserver`.
3. The robot does not verify the certificate, so the self-signed one works.

## Capture — pin the formats from your own robot
Set `"capture": {"dir": "./capture", "bodies": false}` and use the robot normally. `capture.jsonl` records method, path, query,
header *names* (values only for Content-Type/Length/Transfer-Encoding/User-Agent/Host/Connection), body sizes, our status and a short
response preview. **Authorization and Secret values are never written.** Set `"bodies": true` only if you want the raw uploads
(voice recordings, photos) saved, mode 0600. Then:
```
(cd ../tools/emotool && go run . summarize ../../server/capture)
```
Anything it lists as UNHANDLED is a request our server could not answer properly: that is what to compare against
`spec-clean/cloud_api.md` and fix. Capture is off by default.

## Tools (Go, standard library only)
`tools/emotool`: `summarize` (capture summary), `motcheck` (check your own `.mot` files against the documented frame layout), `denylist-scan` / `denylist-gen`
(keeps vendor secrets out of the repository), `partitions` (stock flash layout). Run with `go run .` inside `tools/emotool`.

## Tests
`go test -race ./...` (67 test functions, several table-driven, including a 103-utterance intent table) — includes tests that fail if any outbound-call API appears outside `backend/`, if any backend can reach a
vendor host, or if logs/captures contain credentials.
