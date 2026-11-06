# Install / update wire contract (clean layer)

How the stock firmware's update client talks to a server, as far as it matters for installing our image from the
owner's own LAN server. Sources: public `emo-libre/Documentation` (Server pages, commit `007a02066241`) for the JSON shapes;
the owner's private analysis for the sequence and download behaviour (single-sourced, tagged below).

## Sequence (robot → server)
1. Wi-Fi up. Resolve the API host name. **The default host name is the vendor's public API name; resolve it on your own LAN
   DNS to the local server** (the manifest's `host` can then point anywhere). [C private]
2. `GET /time?tz=<IANA zone>` → `{"time": <unix>, "offset": <seconds east of UTC>}`. Firmware 3.x may receive only `time`. [corroborated]
3. `GET /token/<device id>?version=<n>&version_name=<text>`; device id = 12 lower-case hex characters (the Bluetooth MAC).
   → `{"access_token": <JWT HS256>, "expire_in": <s>, "type": "Bearer"}`. Claims: `sub`, `iat`, `nbf`, `exp`, and (3.x) `version`, `name`.
   The robot stores and replays the token; it does not verify it. [corroborated shapes]
4. `GET /emo/ota/version?type=<0|1>&version_num=<running>` with `Authorization: Bearer <token>` and a `Secret:` signature header
   → `{"version-name": "...", "version-num": N}`. A larger `version-num` than the running one means "update available". [corroborated]
   The `Secret` header is a request signature made with a key embedded in the stock firmware. **This project does not have,
   use or validate it.**
5. `GET /emo/ota/res/<running version>` (or `/emo/ota/allres/<n>` in the full-update mode) → manifest:
   `{"version-num","version-name","updates":{"host","esp32","k210"?,"sd":{...},"model":{...}}}`. [corroborated; sample in Documentation]
6. Download: the client builds the URL from `https://` + `updates.host` + (`/`?) + `updates.esp32` [V: exact joining] and issues a
   plain `GET` (HTTP/1.0 style). The image is written straight to the *other* OTA slot; it is a bare ESP-IDF app image (no wrapper,
   no signature; the reference unit has no secure boot). [C private]
7. After the image is written the client switches the boot partition and reboots. What happens next is decided by the *new*
   image — which is ours.

## TLS
The client's TLS is TLS 1.2 (mbedTLS, old), **certificate verification optional** (a self-signed server certificate works). Port 443.
Our server therefore needs: TLS 1.2, an RSA certificate, and either port 443 or a `host:port` the client accepts in the URL [V].

## Manifest details we emit
- `esp32`: path relative form without a leading slash, e.g. `firmware/esp32/<file>`. The server also accepts `//firmware/...`. [V]
- `sd` and `model`: always present as well-formed groups with only the directory entry `"0"` and no files, so the client finds
  nothing to fetch. Whether omitting them would also be tolerated is unknown [V].
- `k210`: present only when a K210 image is configured. Whether the client skips the K210 step when the key is absent is unknown [V].
  The K210 and ESP32 images of one release are expected to match; we never change the K210.
- `esp_sha256` is informational (the stock client does not read it).

## What the server must NOT do
Contact the vendor, store request headers or bodies, or validate/produce the vendor's request signature.

## Verify on hardware
URL joining of `host` and `esp32`; accepted `host:port`; tolerance of a manifest without `k210` / with empty `sd`,`model`;
whether the client follows redirects; behaviour on a TLS 1.2 ECDHE-RSA handshake with our certificate.
