# Deviations from stock behaviour

Each row: what stock does (as observed/specified), what we do, whether it is wire-visible, and
whether a compat toggle exists. "Wire-visible" deviations must be switchable to stock behaviour.

| ID | Area | Stock | Ours | Wire-visible | Toggle |
|---|---|---|---|---|---|
| DV-001 | K210 link RX | Payload length is not checked against the receive buffer | Frames longer than our buffer are skipped and counted (`stat_overflow`) | no | none (safety) |
| DV-002 | K210 link TX | Payload > ~1 KiB can overflow the K210's fixed buffer (no bound check there) | `send_payload` rejects bodies over `max_payload` (default 1000) with `K210_ERR_TOO_LONG` | no | `cfg.max_payload` |
| DV-003 | K210 link TX | No retry; caller may retry on NACK/timeout | Up to `max_retries` (default 2) automatic resends for acked payloads | yes (duplicate frames possible after a lost ack) | `cfg.max_retries = 0` |
| DV-004 | K210 link RX | Header checksum failure resets the parser, discarding bytes that may begin a real frame | On failure the five collected bytes are re-scanned for a new sync | no | none |
| DV-005 | K210 link RX | Sync `55 55 AA` loses the frame | A repeated `55` keeps the parser armed | no | none |
| DV-006 | Media playlist | Unbounded copy of names/playlists | Builder rejects playlists over 1000 bytes and names with unsafe characters | no | none (safety) |
| DV-007 | K210 link RX | No inter-byte timeout | Port layer resets a half-received frame after 100 ms of silence | no | none |
| DV-008 | Classic-BT speaker | Numeric-comparison pairing is always accepted | User confirmation required (when the speaker is implemented) | yes (pairing UX) | none (security) |
| DV-009 | Procedural motion | A period shorter than one 20 ms frame gives a fractional frames-per-cycle below 1 | Period is clamped to at least 20 ms | no | none |
| DV-010 | Procedural motion | Angle fed to the pulse-width map is not range-checked here | Absolute angle clamped to 0..180° before conversion | no | none (servo safety) |
| DV-011 | `.mot` playback | Behaviour on a file with a bad frame is unknown | Whole file is validated first; an invalid file plays nothing | no | none |
