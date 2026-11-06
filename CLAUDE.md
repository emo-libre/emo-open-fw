# CLAUDE.md — emo-open-fw

Open ESP32 firmware for the EMO desktop robot. Clean-room, publishable (Modes 1 and 2).

## Resume pointers (read these first, in order)
1. `RESUME.md` — current state, what to run, what is next. Update it at the end of every work session.
2. `PLAN.md` — the plan of record (modes, scope, phases, locked decisions).
3. `docs/deviations.md` — every deliberate difference from stock behaviour. Add a row for each new one.
4. `spec-clean/` — the ONLY specification this repo may be written from.

## Hard rules (do not relax without the owner saying so)
- Write code only from `spec-clean/`. Never open or copy from the private reverse-engineering
  tree (`../emo-firmware`: `dirty/`, `ghidra_*`, `inputs/`, `quarantine/`, stock binaries).
- `spec-clean/` states wire formats, pin maps, timings and observable behaviour. It contains no
  firmware addresses, decompiler structure offsets, constants that act as secrets, keys, or
  vendor asset/animation names.
- Repo and build artifacts contain NO living.ai assets, media, keys, credentials, animation
  names or catalogue data. Tests use synthetic names (`demo_a`, ...) and synthetic data.
- We write our OWN routine tables, event tables and strings. Do not transcribe vendor tables.
- Third-party code: list it in `NOTICES` with a licence verified from the upstream project BEFORE it enters the
  tree. Copyleft (GPL/LGPL/AGPL/CC-BY-SA) needs the owner's explicit sign-off first. For code the stock firmware
  modified from an open-source project, start from the genuine upstream and re-apply the documented diffs as
  our own code — never reconstruct from the decompile or spec.
- Reuse first: for anything generic, look for a permissively-licensed upstream before writing code (see RESUME.md).
- Never contact living.ai hosts. The firmware talks to the owner's own server only.
- Run `go run ./tools/emotool denylist-scan` (from `tools/emotool`) before every commit and on build output. It must pass.
- Everything except the ESP32 firmware itself (C, ESP-IDF) is Go with the standard library only: `server/` and `tools/emotool/`.
- Mode 3 (decode/import of the owner's own stock SD content) is personal-use tooling and is
  never added to this repo.

## Build / test
- Host tests (no ESP-IDF needed): `cmake -S tests/host -B build/host && cmake --build build/host && ctest --test-dir build/host --output-on-failure`
- Firmware: ESP-IDF **5.2.x** (pinned; `idf.py build`). Not yet compiled on any machine — see RESUME.md.

## Conventions
- C99, one function (or one tightly-coupled group) per `.c` file, named for what it does.
- Portable protocol code lives in `components/k210_link` and must not include any ESP-IDF header;
  ESP-IDF glue lives in `components/k210_port` and `components/board`.
- Anything inferred rather than confirmed is tagged `[I]` in `spec-clean/` and must be verified
  on hardware before it is relied on.
