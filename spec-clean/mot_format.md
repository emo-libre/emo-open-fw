# `.mot` body-motion files (clean layer)

**Disagreement with the public docs:** `emo-libre/Documentation` guesses a 12-byte frame (marked "Inferred"); this file
follows the private, dump-validated 20-byte layout. Confirm with an owner-run check on a real file before depending on it
(see `SOURCES.md`).

A `.mot` file is a sequence of **20-byte frames**; no header, no trailer. Files are looked up as
`/spiffs/mot/<name>.mot` on the ESP32 flash filesystem (names are truncated at 31 characters on both create and
lookup). If a file is missing the stock firmware fetches it from the K210's SD card over the K210 link.

## Frame layout [C]
| bytes | meaning |
|---|---|
| 0–3 | magic `55 AA 55 AA` |
| 4 | type (2 = body, 0x0E also seen) |
| 5 | sub-type: **0xFE = four-angle move**; 0xFD and others exist; sub-types other than 0xFE/0xFD drive the procedural (sinusoidal) engine |
| 6–10 | five parameter bytes (meaning unknown for move frames; stored, not interpreted) |
| 11–14 | the four servo angles in degrees (neutral 90; shipped data spans roughly 0–180) |
| 15 | always 0 in shipped data |
| 16–17 | duration in milliseconds, u16 little-endian |
| 18 | checksum = sum of bytes 4..17, modulo 256 |
| 19 | terminator `0xED` |

A file is valid only if its length is a non-zero multiple of 20 and every frame has the right magic,
checksum and terminator. Shipped data: every frame in every file satisfies this.

## Playing a 0xFE move frame [C]
- Interpolate linearly from the servos' last position to the target angles.
- If duration ≤ 20 ms: one step. Otherwise `steps = floor(duration / 20)`; each step moves `(target − last)/steps`;
  one update every 20 ms (sleep 20 ms, or wait for the shared 20 ms beat tick).
- Pulse width = `trunc(angle · 2000 / 180 + 500 + 0.5)` µs after adding the per-unit servo trim.
- The remainder of a duration that is not a multiple of 20 ms is dropped.

## Our behaviour
- The whole file is validated before anything moves; an invalid file plays nothing (stock behaviour on invalid data is unknown).
- Angles above 180 are clamped to 180.
- Frames that are not 0xFE moves are passed to a procedural-engine callback if one is installed, otherwise skipped.
- Test data is synthetic. Real `.mot` files are vendor content and are never stored in this repository.
