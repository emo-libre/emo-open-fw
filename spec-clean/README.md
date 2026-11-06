# spec-clean

The only specification this repository's code may be written from.

Rules for adding to it:
- Wire formats, pin maps, timings, observable behaviour, state-machine shapes. Nothing else.
- No firmware addresses. No decompiler-derived structure offsets. No keys, credentials or
  constants that act as secrets. No vendor asset, animation, scene or file names.
- Every non-obvious fact carries a tag: **[C]** confirmed, **[I]** inferred, **[V]** verify on hardware.
- Provenance is kept in the owner's private tree, not here.

Contents: `hardware_map.md`, `k210_link.md`, `upstream_deltas.md`, `motion_oscillator.md`, `mot_format.md`, `ota_wire.md`, `cloud_api.md`, `SOURCES.md` (which facts are corroborated publicly and which are single-sourced).
