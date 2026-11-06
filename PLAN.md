# PLAN OF RECORD — EMO open firmware

Locked 2026-11-05 by the owner's instruction. Changes go through the owner.

## 1. Idea
EMO's behaviour ("the brain": moods, idle scheduler, daily routine, greetings) is ESP32-side.
The K210 is a peripheral that shows an eye video, plays sound, and reports camera/gesture events;
the ESP32 drives it by name. We write the brain once and put a narrow seam under it:
`AssetProvider` (what does this name resolve to) + `Renderer` (how is it played). The three
modes are three implementations of that seam; nothing above it changes.

## 2. Modes and publication scope
| Mode | K210 | Assets | Status |
|---|---|---|---|
| 1 Hybrid | stock, untouched | the owner's own, already on the device | **published** |
| 2 Open | stock (via the `/test` plain-path bypass, to be spiked) or custom | our open pack | **published** |
| 3 Stock-faithful | custom | owner's own SD, converted by an import tool | **personal-use only; the decode/import tool is never shipped or added to this repo** |

The owner's decision is that Mode 3 stays personal-use. Whether that settles every legal question
for publication is the owner's call; it has not been independently verified here.

## 3. Scope
- ESP32 firmware only. K210 firmware and the four limb MCUs stay stock.
- Install path: the stock OTA client fetches our app image from a server on the owner's LAN.
  Stock partition table and NVS layout are kept exactly (see `partitions.csv`, `spec-clean/hardware_map.md`).
- Stock-compatible fallback: local protocols only (BLE, LAN TCP/UDP, K210 link, OTA wire format).
  Cloud calls go to the owner's server. Nothing is sent to living.ai.

## 4. Locked deltas (2026-11-05)
1. Full animations play via **payload sub 2** = JSON media playlist `[loop,[[avi,[[t,mp3],...]],...]]`
   (builder: `k210_media_playlist.c`). Single avi/mp3 by name use **subs 24/25/26/27**.
2. The **`/test` plain-path bypass** is the Mode 2 open-asset path: files under `/test` play on the
   stock K210 without decryption. UNVERIFIED — must be spiked on hardware.
3. **audio_eq** module: biquad band-filter bank on the ESP32 PCM output path, REST-configurable.
   It cannot touch audio decoded by the stock K210 (Mode 1 faces/sounds); it only applies to PCM the
   ESP32 itself produces/sends.
4. Own routine table and own event/reaction table. No vendor tables, offsets, constants, keys or
   asset names in the repo; `spec-clean/` only.

## 4a. Pivot (2026-11-05): fully local server first
Goal: the stock robot runs entirely against `server/` with zero vendor contact, no firmware change. Done in code: time, token, OTA
(version/res/file), weather, speech TTS, chat start, voice prompt, voice conversation (STT -> rules -> LLM -> TTS) with pluggable
backends, image description (vision), capture mode. Not done: dialogue (kinds 4/5) and drawing (0xC/0xD) — formats unknown, answered 501.
Everything marked [V] in `spec-clean/cloud_api.md` is unconfirmed until the owner runs capture mode on their own robot.

## 4b. Native features (2026-11-05)
`docs/lai_feature_matrix.md` lists every native cloud feature with status. Done: intent engine (56 intents), timers/alarms/lights/volume/
time/date/battery/update/weather/photo/games/music/animals/names/birthdays, chat mode, notices, permission, cached fast TTS.
Open: dialogue + drawing (need capture), news, weather push, beta channel, per-language command packs, own-content pack hosting.

## 4c. Server completeness (2026-11-05)
All items on `docs/lai_feature_matrix.md` that can be built without guessing are built: drawing, news, beta channel, own-content packs,
admin + telemetry whitelist, token enforcement + allow-list, language packs (de), LLM-assisted commands, prompts in six languages,
graceful shutdown. Blocked: dialogue key names, `res.zip`. Everything [V] awaits a capture from the owner's robot.

## 4d. Languages, providers, Go (2026-11-05)
Every command in the vendor's published English list is understood (166 of 168; two are robot-to-robot questions). Vietnamese (contributed by
a native speaker) is covered in full, parameters included, via an embedded pack with a lexicon; German covers commands. Providers added:
Anthropic (text and vision), Google STT/TTS, ElevenLabs, OpenWeatherMap, WeatherAPI, Pollinations; presets for local, free-online and Anthropic setups.
All tooling is Go (`tools/emotool`); the only non-Go code is the ESP32 firmware.

## 5. Phases
0. Spec-clean layer, denylist scan, hardware spikes (IDF 5 image through stock OTA; `/test` bypass; manifest without a K210 entry; missing pins).
1. **Board bring-up + K210 link tiers 0–1** (reset, version, volume, eye mode, play-by-name, sub-2 playlist). Code done; hardware-untested.
2. Seam + Mode 1 renderer.
3. Minimal body and inputs: servo driver (safe pulses first), `.mot` player, touch, cliff, battery.
4. NVS vault (before any per-person memory).
5. Brain: mailbox/dispatch, affect model, idle scheduler, own routine table.
6. Social brain (own strings).
7. Mode 2 pack + renderer.
8. Remaining sensors, second motion engine, Wi-Fi/web/REST, audio_eq.

## 6. Locked earlier decisions (carried from review)
- NVS: stock keys untouched; our data in namespace `oefw`, sealed with application-level AEAD.
  Mode (a) key from owner's server over TLS (pinned); mode (b) passphrase KDF. Whole-partition
  IDF NVS encryption is opt-in only (breaks stock layout compatibility).
- Never write the stock persistent blob key `emo_data` (it embeds code pointers).
- Content key for the stock catalogue: never in this repo or firmware. Mode 1 learns animation
  names from an owner-run host tool or the owner's own manifest; Mode 2 has its own catalogue.
- Tier-3 K210 commands (K210 flash writes/slot flips) are off by default and not implemented.

## 6a. Toolchain pin and licence decisions (2026-11-05)
- ESP-IDF is pinned to **5.2.x** (CMake guard + `main/idf_component.yml`) so cJSON stays in-tree. Glue APIs were checked
  against the `release/v5.2` headers (`i2c_master_probe`, `ADC_ATTEN_DB_12`, line-fitting calibration, `UART_SCLK_DEFAULT`, `gptimer`).
- The sinusoidal motion engine is original code (`components/motion`); the GPL-3.0 Otto library is not used. Firmware stays permissive.
- Third-party policy: see `NOTICES` and `CLAUDE.md`.

## 7. Open questions (need hardware)
See `RESUME.md` "Verify on hardware".

## 4e. Eleven languages, custom commands, admin page (2026-11-05)
German, French, Spanish, Italian, Portuguese, Russian, Japanese, Korean and Chinese join Vietnamese: each covers the whole 168-command list and its own timers,
alarms, colours, cities, names and birthdays (pack + lexicon; zh/ja/ko tokenise by character). Added after studying wire-pod (ideas only, nothing copied;
`docs/wirepod_comparison.md`): custom commands (`custom_commands_file`), an admin page with a phrase tester, optional audio clean-up before speech-to-text.
Open: native-speaker review of every line not taken from the vendor lists; a built-in DNS helper is a candidate to ease setup.
