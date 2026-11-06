# Native cloud features vs. the local server

"Native" = everything the stock firmware (and its dock/phone apps) got from the vendor's cloud. Status as of 2026-11-05.
**done** = implemented and unit-tested · **[V]** = built from documentation, unconfirmed on a real robot (run capture mode to confirm) ·
**blocked** = cannot be built without information we do not have · **no** = deliberately not done, with the reason.
Everything below is Go (standard library only) except the ESP32 firmware itself.

## 1. Identity, transport, access
| Feature | Status | Notes |
|---|---|---|
| Time, token (HS256 JWT with `version`/`name` claims), TLS 1.2 self-signed | done, [V] | fixes the upstream `Offset` key bug |
| Plain-HTTP speech download (token-protected) | done, [V] | non-default port in the URL unconfirmed |
| `require_token` (valid token needed on `/emo/*`), `allowed_devices` allow-list | done | off by default until capture confirms every robot call carries a token |
| Admin API (`/admin/status`, `/admin/robots`) | done | off unless `admin_token_env` is set |
| Vendor request signature (`Secret` header) | no | needs a vendor key; never produced or validated |
| Graceful shutdown, example systemd unit | done | the unit is an example |

## 2. Updates and content
| Feature | Status | Notes |
|---|---|---|
| Version check, manifest (`res`, `allres`), ESP32 image download | done, [V] | manifest tolerance unconfirmed |
| Optional K210 image, beta channel | done, [V] | |
| Owner's own media offered to the updater (`content_dir`) | done, [V] | never bundled; names validated; read-only |
| K210 `res.zip` and other archives | blocked | request format unknown (capture) |

## 3. Pushed state and telemetry
| Feature | Status | Notes |
|---|---|---|
| Notices (from a file, live), ownership line, server switch | done | |
| Weather *push* on unrelated replies | no | the robot pulls weather itself |
| Telemetry | done | only battery/Wi-Fi/version-type numbers, in memory, for the admin API; city, time zone, language, age dropped on arrival |

## 4. Where the intelligence can come from (all optional, pick any mix)
| Capability | Local (free) | Online (free tier, check current terms) | Online (paid) |
|---|---|---|---|
| Speech-to-text | any OpenAI-compatible server (faster-whisper based, whisper.cpp server) or a program (`whisper-cli`) | Groq (OpenAI-compatible), Google Cloud Speech-to-Text | OpenAI |
| Chat / commands | Ollama, llama.cpp, vLLM, LocalAI | Groq, Gemini (OpenAI-compatible), OpenRouter free models | **Anthropic**, OpenAI |
| Photo description | Ollama with a vision model | Gemini | **Anthropic**, OpenAI |
| Text-to-speech | espeak-ng + lame (tested, ~50 ms), Piper, Kokoro, any OpenAI-style server | Google Cloud Text-to-Speech, ElevenLabs (free allowance) | OpenAI |
| Weather | (needs the internet) | Open-Meteo (keyless), OpenWeatherMap, WeatherAPI.com | |
| News | RSS/Atom feeds (free) | | |
| Drawing | Stable Diffusion WebUI | Pollinations (keyless, text-to-image only) | OpenAI images (text-to-image only) |
Presets: `server/presets/{local,free-online,anthropic}.json`. A test checks that each loads and builds; it cannot prove a service is reachable.

## 5. Understanding and conversation
| Feature | Status | Notes |
|---|---|---|
| **Every command in the vendor's published English list (168 phrases, 166 understood)** | done, [V] | rule-based, microseconds. The two exceptions ("Do you know him/it?") are robot-to-robot questions with no known cloud reply |
| Commands: news ("what's new today"), photo recognition, 20+ games (incl. 100-points dice, shooter, paint, weapons, birthday), carry mode, eye colour (10 colours + default), hug/kiss/love/feelings, 16 animals, laser/devil eyes, dance (plain / together / lights / to music), 11 festival greetings, goodbye / good morning…night, AI-painting styles, sleep, home-station update, Bluetooth speaker and its transport controls, timers (set/cancel), alarms (set/check/remove/enable/disable by ordinal, tags gym/eat/sleep/pill, recurrence), lights, volume, weather for any city | done, [V] | animation names beyond the documented ones are best guesses |
| **Languages: English plus German, French, Spanish, Italian, Portuguese, Russian, Japanese, Korean, Chinese (Simplified) and Vietnamese: all 168 commands each, and timers, alarms (time, weekdays, tags), light and eye colours, weather for a city, names, birthdays, tic-tac-toe numbers and the 16 animals** | done | each language has a pairs file in `server/testdata`; every pair must mean exactly the same as its English version (also with accents stripped); a second test uses other values. Non-vendor lines are unreviewed translations |
| More languages | plan | a pack file `<lang>.json` with an optional `_lex` section is all that is needed (see server/README.md) |
| Your own commands (speak, animation, explicit command, local program) | done | `custom_commands_file`; programs need `custom_commands_allow_exec` |
| Admin page: first-run setup, settings and API-key editing, presets, service check, voice test, phrase tester, your-commands editor, activity log, capture report, install guides | done | `/admin/ui` (needs `admin_token_env`; `listen_admin` puts it on its own listener) |
| Built-in DNS helper to point the robot here | done, [V] | `dns_*`; whether the robot honours a DHCP-assigned DNS server is unverified |
| Audio clean-up before speech-to-text | done, [V] | `stt_preprocess`, off by default; untested on a real robot's audio |
| LLM-assisted commands (`llm_commands`) | done | the model can never power off or update the robot |
| Free conversation: chat mode with per-robot memory, goodbye | done, [V] | `listen` semantics unconfirmed |
| Spoken refusal/event lines in en, de, fr, es, it, pt, vi | done | all non-English lines unreviewed by native speakers; override with `prompts_file` |
| Scripted TTS; fixed lines cached and pre-warmed; conversation never cached | done | |
| News as speech, weather, drawing, photo description | done, [V] | |
| Two-robot "dialogue" (kinds 4/5) | **blocked** | the reply's two per-line key names are not in any source we hold; capture will show them |
| Eye customisation image transfer | no | goes phone→robot directly |

## 6. Not served by this server, and why
| Thing | Why |
|---|---|
| Phone-app ↔ vendor app server | the apps use their own hard-coded host with normal certificate checks |
| Phone ↔ robot Bluetooth/LAN, robot ↔ dock mesh, robot ↔ robot UDP | local protocols in firmware |
| Any vendor-hosted content or contact | never |

## 7. What remains
1. Run capture on the owner's robot: pins every [V] (animation names, drawing format, voice-prompt fields) and unblocks dialogue and `res.zip`.
2. Native-speaker review of the non-English prompt lines and of every pack line the vendor lists did not supply (see each `testdata/*_pairs.txt` header); Traditional Chinese and kana variants if wanted.
3. Written licence confirmation from the Vietnamese contributor (see NOTICES).

## 8. Engines the server runs for you
| Engine | Status | Notes |
|---|---|---|
| whisper.cpp (`stt` `whispercpp`): started, watched, restarted and stopped by the server | done, [V] | tested against a stand-in program; the real `whisper-server` request format is from memory |
| espeak-ng (`tts` `espeak`) | done | tested for real where installed; voice follows the language |
| Piper (`tts` `piper`) | done, [V] | fixed command line; not run here (no Piper installed) |

## 8. Engines the server runs for you
| Engine | Status | Notes |
|---|---|---|
| whisper.cpp (`stt` `whispercpp`): started, watched, restarted and stopped by the server | done, [V] | tested against a stand-in program; the real `whisper-server` request format is from memory |
| espeak-ng (`tts` `espeak`) | done | tested for real where installed; voice follows the language |
| Piper (`tts` `piper`) | done, [V] | fixed command line; not run here (no Piper installed) |
