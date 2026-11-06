# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- **EMO owners** who want their desk robot to keep working without the vendor's cloud, entirely on their own machine. Comfortable
  following clear steps; not necessarily developers. They need a guided path: get the server running, point the robot at it, choose
  speech/model/voice engines, and see that it works.
- **Tinkerers** (including the project's author) who know the internals. The same page serves them, with technical detail
  (settings, API keys, presets, logs, timings) reachable in one step rather than spread through the newcomer path.

## Product Purpose

The admin page is the control room for the self-hosted EMO server: first-run setup, choosing and checking the plug-in engines
(speech-to-text, language model, voice, weather, news, drawing), testing phrases and speech, the owner's own voice commands, recent
activity, and help. Success: an owner goes from a fresh install to "EMO answers me" without reading source code, and can tell at a
glance whether everything is healthy.

## Positioning

Everything runs on the owner's own hardware and network: no account, no vendor cloud, and nothing sent to the robot's maker. Each
engine is a plug the owner picks (local program, local model server, or an online API with their own key). It is an independent open
project, not affiliated with the robot's vendor.

## Operating Context

- Opened from a laptop or desktop browser (often the machine running the server) and from a phone on the same Wi-Fi, about equally:
  setup tends to happen on the laptop, quick checks and tests from the phone next to the robot.
- Served by the Go server itself on the local network (`listen_admin`, plain HTTP and a self-signed HTTPS variant), behind an admin
  token. Works offline: no CDN, external fonts or scripts.
- The robot speaks with a few-second patience window, so timing and "is it fast enough" are everyday concerns.

## Capabilities and Constraints

- Views today: sign-in (admin token), Home (status), Setup (presets, settings, API keys, managed engines), Test (phrase tester,
  self-test, say a line), Your commands (custom voice commands), Activity (recent requests with per-stage timings), Help.
- Single self-contained `server/ui/admin.html` (inline CSS and JS), served by `admin_ui.go`; covered by Go tests and a headless-browser
  check. Editing settings is opt-in on the server (`admin_allow_edit`).
- Go standard library only on the server side; no front-end build step or framework.
- **Name: undecided.** Use a neutral, descriptive placeholder until the owner picks one.

## Brand Commitments

- Never use the robot vendor's name, logo, artwork, animation names or any of its assets. "EMO" names the robot the server talks to,
  nothing more.

## Evidence on Hand

- No screenshots, testimonials, user counts or press exist. Do not invent any.

## Product Principles

1. First run is a guided path; power settings are one step away, never in the newcomer's way.
2. Health at a glance: every engine shows whether it works and how fast, in plain words.
3. Local and private by default, and the page says so where it matters (keys, captures, online engines).
4. Equally good on a laptop and on a phone held next to the robot.

## Accessibility & Inclusion

- Keep what the page already guarantees: keyboard use with a skip link and visible focus, 44 px touch targets, light and dark themes,
  reduced motion, and a phone-width layout. Non-English owners use the server too (11 language packs), so labels stay plain.
