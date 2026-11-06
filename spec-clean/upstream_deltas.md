# Upstream-modified code: what actually differs from stock Espressif code (clean layer)

Approach: for every place the stock firmware modified open-source code, our firmware is built on the
**genuine upstream project** (its own repository, its own licence, credited in `NOTICES`). We re-apply only
the *behavioural* delta, written from this file as our own code. Nothing is transcribed from the stock
binary or its decompilation. Copyleft upstreams are flagged in `NOTICES` and need the owner's sign-off first.

Upstream locations as of 2026-11-05: the A2DP sink example is `a2dp_sink_stream` (+ `a2dp_utils`) in IDF 5.2;
the BLE-Mesh provisioner example is `examples/bluetooth/esp_ble_mesh/provisioner`; cJSON is `components/json`
in IDF 5.2 and the managed component `espressif/cjson` in newer IDF.

| # | Area | Upstream baseline | Behavioural delta that matters | Our plan | Phase |
|---|---|---|---|---|---|
| 1 | JSON | cJSON allocator hooks | Only extra log lines. The application separately routes all JSON allocations to external RAM. | Use IDF's cJSON (never reimplemented); call `cJSON_InitHooks` once with a PSRAM allocator. | 2 |
| 2 | 20 ms frame tick | IDF timer-group example ISR | Hardware timer, 20 ms period, auto-reload; the ISR posts one small event record to a queue. Servo interpolation and the sine motion engine wait on that queue when "beat-sync" is on, otherwise they sleep 20 ms. The BLE path pauses the timer. | `beat_timer` component on `gptimer`: 20 ms auto-reload alarm, ISR-safe queue post, pause/resume. | 3 |
| 3 | Servo pulse end | (not upstream — vendor code) | A one-shot timer ends each commanded pulse by driving all servo pins low, then pauses itself. | Replaced by RMT/MCPWM pulse generation in the servo module (see PLAN.md). | 3 |
| 4 | Classic-BT speaker (A2DP sink + AVRC) | Espressif A2DP sink example | Advertises a per-unit speaker name built from two bytes of the BT address; registers AVRC controller and target with the volume-change notification; the sink data callback feeds decode/resample then an audio ring (the ring is emptied if it grows past ~256 KiB). Stock auto-accepts numeric-comparison pairing. | Use the example as the base. Name is configurable. **Deviation:** require user confirmation for pairing (DV-008, to be added). PCM produced here is where `audio_eq` can apply. | 8 |
| 5 | GATT server (phone channel) | Espressif GATT-server demo | Single profile, service 0xFFE0 / characteristic 0xFFE1 (write, write-no-response, notify); advertising restarts after raw-data set; the vendor adds a command channel with ~120 commands behind the standard connect/disconnect/write handling. | Base on the demo; command set to be extracted into its own clean spec before implementation. | 8 |
| 6 | BLE-Mesh provisioner (dock nodes) | Espressif BLE-Mesh provisioner example | Config/generic/health client callbacks drive a "welcome a new node" sequence: read capabilities, push the application key, bind it to each model, track on/off state with resend, treat health reports as presence; at most five connected nodes. | Base on the example; behaviour details to be extracted into a clean spec before implementation. | 8 |
| 7 | BT task core | Espressif `bt_app_core` | Boilerplate only. | Use the example unchanged. | 8 |

Items 1 and 7 need no new logic. Items 2 and 3 are small. Items 4–6 are the real work and belong with the
connectivity phase.

## Not copied on purpose
- Auto-accepted pairing (item 4): a security weakness, not a requirement.
- Any unit-specific or vendor-embedded constant mentioned in the private notes.

## Gait / procedural motion engine (not an upstream modification per the analysis)
The analysis classes the sinusoidal motion engine as vendor code, not as a modified upstream. Behaviour:
each servo follows `angle(t) = amplitude * sin(phase + 2*pi*t/period) + centre`, per-servo invert flag and
trim, 20 ms frames, N cycles. The owner believes its design descends from the Otto DIY `Oscillator`
(GPL-3.0). That attribution is unverified here. Decision: original implementation in `components/motion`,
specified in `motion_oscillator.md`; see `NOTICES`.
