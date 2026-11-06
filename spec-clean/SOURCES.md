# Sources and corroboration for spec-clean/

Two kinds of source:
- **Public**: `emo-libre/Documentation` (BSD-3-Clause, © Holger Wolff and contributors), commit `007a02066241` (2026-10-04).
  Cited as `Documentation/<path>`. Mostly string-level analysis; its own evidence legend marks what is confirmed.
- **Private**: the owner's own analysis of their own unit (not published, not in this repository).

A fact that only the private analysis supports is **single-sourced**: treat it as a hypothesis until it is confirmed on
hardware or by a public source. Tags `[C]/[I]/[V]` inside the other files keep their existing meaning.

| Fact | Public corroboration | Status |
|---|---|---|
| SPIFFS partition at 0xa20000, 1 MB, page 256 B, block 4 KiB, type data/spiffs | `Documentation/Firmware/extraction.md` ("Input" tables) | **corroborated** |
| Other partitions (fctry, phy_init, otadata, ota_0, ota_1, nvs): offsets and sizes | none (only file names `ota_0_out.bin`, `ota_1_out.bin`, `nvs_out.bin`) | single-sourced; check with `esptool read_flash 0x8000 0x1000` on the unit |
| BLE service 0xFFE0 / characteristic 0xFFE1 (read, write) | `Documentation/Bluetooth/README.md` (GATT dump) | **corroborated** |
| BLE frame formats (`BB AA`, `DD CC`, raw `55 AA 55 AA`) | none | single-sourced |
| Media path forms `/spiffs/avi/<n>.avi`, `/spiffs/mp3/<n>.mp3`, `/test/<n>.avi`, `/test/<n>.mp3` | `Documentation/Firmware/animation_system.md` ("Face command to the K210": string-confirmed) | **corroborated** (the strings exist; whether `/test` plays plain files is still [V]) |
| Playlist JSON shape `[0,[["<avi>",[[0,"<mp3>"]]]]]` | same page (same strings) | **corroborated** for the one-clip form; multi-clip/loop semantics single-sourced |
| That the playlist travels as payload sub-type 2, and the NUL question | none | single-sourced [V] |
| `.mot` file path `/spiffs/mot/<n>.mot` | `Documentation/Firmware/animation_system.md` | **corroborated** |
| **`.mot` frame layout** | public doc *guesses* 12-byte frames and marks it "Inferred ... needs checking" | **DISAGREEMENT.** We follow the private 20-byte layout (reported validated on all frames of an actual dump). Resolve with an owner-run check on a real file (`emotool motcheck`); no vendor file is ever committed. |
| ESP32↔K210 UART: port, pins, baud, framing, checksum, ack | `Documentation/Firmware/k210_protocol.md` states port/pins/baud are **not confirmed** and sketches a different, inferred frame | **single-sourced.** First thing to prove on hardware. |
| K210 command numbers and payload sub-types | none | single-sourced |
| GPIO map, I²C addresses | none (public docs say pins are unconfirmed) | single-sourced |
| Servo pulse map 500–2500 µs, 50 Hz | `Documentation/Firmware/animation_system.md` ("PWM pulse width 500–2500 µs, 1500 = center") | **corroborated** (as a possibility they list; consistent with private) |
| Cloud endpoints `/time`, `/token/<id>`, `/emo/ota/version`, `/emo/ota/res/<n>` and their JSON shapes | `Documentation/Server api.living.ai/*.md` | **corroborated** (used for the local server; see `ota_wire.md`) |
| Voice upload layout (framing byte 0x22, PCM s16le 16 kHz mono, framing byte 0x22; chunked; 3 s cap in chat), intent-reply shape, `index` echo, chat-start / TTS reply shapes, weather reply shape | `Documentation/Server api.living.ai/emo-voice-detectintent.md`, `emo-chat-start.md`, `emo-speech-tts.md`, `emo-weather-forecast.md` | **corroborated** (see `cloud_api.md`) |
| How long the robot waits for a `detectintent` reply (~10 s after activity stops; later replies ignored) | none | single-sourced [I]; on the owner's robot replies up to 8.0 s after the upload were accepted and the window counts from the upload's end; the cut-off itself is unmeasured |
| Which request kind needs which reply fields, the `index` staleness rule, voice-prompt reply fields, ending chat with `chatgpt_end`, weather phrase table / classes | none | single-sourced (private); pin with capture mode |

When a public page later confirms a single-sourced fact, move it to "corroborated" and add the path and commit.
