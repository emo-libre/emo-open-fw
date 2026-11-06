# K210 link — wire specification (clean layer)

Tags: **[C]** confirmed against captures/bytes by the owner's private analysis, **[I]** inferred,
**[V]** must be verified on hardware before relying on it. This file contains wire facts only.

**Source status (see `SOURCES.md`):** the framing, baud, pins, checksum and ack rules in §1–§4 are *single-sourced*
from the owner's private analysis. The public `emo-libre/Documentation` lists the UART parameters and framing as
unconfirmed. Only the media-path strings and the one-clip playlist JSON in §6 are corroborated publicly. Treat §1–§5 as
hypotheses until the on-hardware handshake passes.

## 1. UART (commands, small payloads)
- UART1, **1,000,000 baud, 8N1**, ESP32 TX = GPIO 12, RX = GPIO 4, no flow control. [C]
- Byte-oriented on both ends. The K210 receives in an interrupt, one byte at a time; the stock
  sender pauses ~10 ms after every frame. We keep that gap (`gap_ms`). [C/I]

## 2. Frames (identical in both directions) [C]
| frame | bytes |
|---|---|
| short | `55 AA cmd b1 b2 b3 sum`, `sum = (0x55+0xAA+cmd+b1+b2+b3) & 0xFF` |
| payload header | `55 AA FF len_lo len_hi sub sum` — a short frame with `cmd=0xFF, b1=len_lo, b2=len_hi, b3=sub`; same `sum` rule |
| payload body | `len` bytes, then one byte **CRC-8/MAXIM** of the body (poly 0x31 reflected = 0x8C, init 0, no xorout; check value for ASCII "123456789" = 0xA1) |

Multi-byte arguments inside `b1..b3`: `v16 = b1 | b2<<8`, `v24 = b1 | b2<<8 | b3<<16`.

## 3. Acknowledgement (ESP→K210 payload frames only) [C]
After the CRC byte the K210 answers with a short frame **cmd 254 (0xFE)**, `b1 = 0xAA` good or `0x55` bad.
Sender waits up to **500 ms**. `0x55` means the K210 rejected the CRC and reset its receiver.
Short frames are not acknowledged. [C]
Open: whether the ESP must acknowledge K210→ESP payload frames. Stock behaviour not established; we do not. [V]

## 4. Bulk SPI link (files, audio, camera frames) [C]
- ESP32 is SPI master, host 2/"HSPI": MOSI 15, MISO 2, SCLK 14, CS = GPIO 13 driven in software
  (asserted per transaction), 8 MHz, mode 1.
- Transfer = 8-byte header `a, b, len_lo, len_hi, mode, sum, 0, 0` where `sum = (a+b+len_lo+len_hi+mode) & 0xFF`;
  ≥ 500 µs pause; then the body padded with zeros to a multiple of 64 bytes. The K210 raises an
  interrupt from the header and arms a DMA for the body. Block size on file paths: 1,024 bytes.
- Per-block integrity on the bulk path is an 8-bit additive sum, not CRC-8.
- `mode` values used by us: 0 audio PCM to speaker ring, 2 camera/face frame (receive),
  7/8/9 file data, 10 raw copy. `a`,`b` are 0 in all uses we know. [I]
- Mode 6 (K210 image to flash) is deliberately not implemented.

## 5. Commands implemented in Phase 1 (ESP→K210)
| cmd | name | arguments | effect |
|---|---|---|---|
| 52 | reset | 0,0,0 | K210 soft-resets. Stock sends it once at boot, then waits ~0.5 s + ~1 s + ~2.5 s for the K210 to return. [C] |
| 60 | firmware number | 0,0,0 | K210 replies with a **K210→ESP short frame cmd 60** whose bytes carry its firmware number. Byte layout of the number: [V] (we expose `v24`). Stock polls up to 3×, 200 ms apart. [C] |
| 105 | version text | 0,0,0 | K210 replies with **payload sub 105** carrying a short NUL-terminated text, e.g. "3.2.0". [C] |
| 23 | volume | v16 | `v16 ≤ 60` sets the level. Other values (101–106) are step/preset/lock codes — not used. [C] |
| 56 | eye/display mode | v16 | 1 default, 2 vigilant, 4 idle, 5 game; 3 and 6 exist, meaning unknown. [C/I] |

## 6. Media playback (ESP→K210 payload frames)
| sub | meaning | body |
|---|---|---|
| 24 | eye video once | path string + NUL |
| 26 | eye video loop | path string + NUL |
| 25 | sound once | path string + NUL |
| 27 | sound loop | path string + NUL |
| **2** | **media playlist** (full animation: video + timed sounds) | JSON text + NUL (below) |

Path string: the stock form is `/spiffs/avi/<name>.avi` or `/spiffs/mp3/<name>.mp3`. The K210
resolves it to a hashed file on its SD card; it never stores names. A path beginning `/test`
is the plain-path ("developer") route: the K210 skips decryption and opens the plain file. [V]
Whether the string must include the NUL: the stock sender includes it for the name commands we
know of; we include it. [I]

### Playlist JSON (sub 2) [C]
Compact JSON array, no spaces:

`[loop,[[avi,[[t,mp3],[t,mp3],...]],[avi,[...]],...]]`

- `loop`: integer, `0` = once, `1` = loop. Other values: unknown. [I]
- each clip: `[<avi path string>, [ [<t>, <mp3 path string>], ... ]]`; `t` is an integer time offset
  for that sound within the clip (units unverified; stock uses 0 for a simple clip). [I]
- The K210 copies the sub-2 body into a fixed ~1 KiB buffer **without bounds checking**. Keep the
  JSON (including NUL) ≤ 1000 bytes. [C]
- Side effect on the K210: receiving sub 2 resets its media state before parsing. [C]
- The K210 reports "media finished" back to the ESP. The frame carrying it: [V — not needed in Phase 1].

## 7. K210→ESP traffic we parse in Phase 1
| frame | meaning |
|---|---|
| short cmd 254 | ack (above) |
| short cmd 60 | firmware number |
| payload sub 105 | version text |
All other frames are delivered to the registered event callback and otherwise ignored.
