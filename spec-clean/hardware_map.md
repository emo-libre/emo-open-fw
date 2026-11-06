# Hardware map (clean layer)

Source status (see `SOURCES.md`): only the SPIFFS partition geometry is corroborated by the public docs; the rest of the
partition table, the GPIO map and the I²C addresses are single-sourced and need on-device confirmation.

## Flash (16 MB, DIO, 40 MHz). No secure boot, no flash encryption on the reference unit.
| label | type/subtype | offset | size |
|---|---|---|---|
| fctry | data/nvs | 0x009000 | 0x006000 |
| phy_init | data/phy | 0x00f000 | 0x001000 |
| otadata | data/ota | 0x010000 | 0x002000 |
| ota_0 | app/ota_0 | 0x020000 | 0x500000 |
| ota_1 | app/ota_1 | 0x520000 | 0x500000 |
| storage | data/spiffs | 0xa20000 | 0x100000 |
| nvs | data/nvs | 0xb20000 | 0x040000 |

Partition table at 0x8000 (with MD5 entry). Bootloader at 0x1000 is not ours and is never written
by OTA. SPIFFS is mounted at `/spiffs`, `max_files 100`; names are limited to 31 characters and
longer names are truncated on both create and lookup. otadata slot = (seq − 1) mod 2.

## GPIO / buses
| function | pin(s) | note |
|---|---|---|
| K210 UART1 | TX 12, RX 4 | 1 Mbaud. GPIO12 is a strapping pin (MTDI): do not drive high externally at reset. |
| K210 SPI bulk (HSPI) | MOSI 15, MISO 2, SCLK 14, CS 13 (software) | 8 MHz, mode 1 |
| I²C | SDA 19, SCL 18 | 100 kHz. Pull-up arrangement [V] |
| K210 reset | 22 | idle high; low pulse ≈ 1 ms resets the K210 [I] |
| power hold | 23 | output, driven high at start [I] |
| servo signal 0 | 21 (new boards) / 0 (old boards) | detection method in later phase |
| servo signals 1–3 | 25, 5, 26 | later phase |
| touch pads | GPIO 27, 33, 32 (pads 7, 8, 9) | later phase [I] |
| presence-chip interrupt | 35 | input |
| charger present | 36 | input |
| battery sense | ADC1 channel 3 (GPIO 39), 11 dB class attenuation, 12 bit | divider ratio [V] |
| servo-board update UART2 | pins unknown | later phase |
| I²S audio | pins unknown | later phase |

## I²C devices (7-bit addresses)
presence chip 0x71 · light/proximity 0x23 · time-of-flight 0x29 · magnetometer 0x30 ·
IMU 0x68 / 0x6A / 0x6B (one of three chips, auto-detected) · floor-IR board at address 0.
