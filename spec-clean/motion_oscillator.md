# Procedural (sinusoidal) motion — behavioural specification (clean layer)

The recorded `.mot` poses ("go to these four angles in N ms") are one motion engine. This is the second:
it **computes** repeated movement (head shake, wag, walking sway) as a sine wave per servo. Our implementation
is written from the maths below and from these observed behaviours. It is not derived from any third-party
oscillator library.

## Model
Four servos, index 0..3. Per servo *j* the caller supplies:

| parameter | type | meaning |
|---|---|---|
| amplitude | int, degrees | peak swing |
| centre | int, degrees | middle of the swing, relative to the neutral pose |
| phase offset | radians | start phase of this servo's wave (different phases make a walk or a sway) |
| invert | flag | mirror the servo (output is negated) |
| trim | int, degrees | per-unit calibration, added after the wave (valid ±20) |

Run-wide: `period_ms`, number of `cycles` (may be fractional), a start phase in degrees applied to all servos.

## Frame computation [C]
- Frame time is **20 ms**. `frames_per_cycle = period_ms / 20`; phase step `dphase = 2π / frames_per_cycle`.
- For frame *i* and servo *j*:
  `rel_j = trunc( amplitude_j · sin(phase_offset_j + start_phase + i·dphase) + centre_j )`
  — truncation toward zero (a C double→int conversion), then negated if `invert_j`.
- Absolute servo angle = `rel_j + 90 + trim_j` (90 = neutral), limited to 0..180.
- Pulse width in µs = `trunc(angle · 2000 / 180 + 500 + 0.5)` (0° = 500 µs, 90° = 1500, 180° = 2500).
- Cycles: whole cycles run first, then the fractional remainder; the wave stays phase-continuous throughout.
  A counter of completed whole cycles is kept (the behaviour layer reports it as "steps walked").
- Pacing: one frame every 20 ms, either by sleeping 20 ms or by waiting on the shared 20 ms beat tick.
- A "hold" condition stops the run: if it is set at the start nothing is output; if it becomes set mid-run
  the run ends after the current frame.

## Our deviations (see docs/deviations.md)
- `period_ms` is clamped to at least one frame (20 ms) so `frames_per_cycle ≥ 1` (DV-009).
- Output angle is clamped to 0..180 before the pulse width is computed (DV-010).
- Phase is computed from the frame index rather than accumulated, so long runs do not drift (not visible on the wire).

## Not covered here
The numbered generator library (swing, bow-and-shake, walking gait, head turn, …) is built on top of this
engine; each generator's parameters will get their own entry when it is implemented.
