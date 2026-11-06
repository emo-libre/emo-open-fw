---
name: EMO open server admin
description: A tabbed manual for one robot; each section owns a full-strength colour tab, and the open section's colour is the board under a milk-white sheet.
colors:
  hue-home: "#f2c230"
  hue-setup: "#8c4a2b"
  hue-try: "#e06a1f"
  hue-commands: "#3d9a4a"
  hue-activity: "#239494"
  hue-help: "#2a3fb0"
  hue-expert: "#6b3fa0"
  on-hue-ink: "#15171a"
  on-hue-white: "#ffffff"
  sheet: "#fbfcfd"
  field: "#ffffff"
  ink: "#15171a"
  ink-2: "#4a4f57"
  edge: "#d3d8df"
  edge-2: "#e9ecf0"
  problem: "#d7261e"
  problem-ink: "#a51d16"
  slip: "#c4231c"
  sheet-dark: "#1b1d21"
  field-dark: "#121417"
  ink-dark: "#eef0f3"
  ink-2-dark: "#b3b9c2"
  edge-dark: "#353a42"
  edge-2-dark: "#2a2e35"
  problem-dark: "#ff5a4e"
  problem-ink-dark: "#ff8a80"
typography:
  display:
    fontFamily: "Univers Condensed, Helvetica Neue Condensed, Arial Narrow, Roboto Condensed, DejaVu Sans Condensed, Liberation Sans Narrow, Nimbus Sans Narrow, system-ui, sans-serif"
    fontSize: "clamp(2.6rem, 4.6vw, 4.2rem)"
    fontWeight: 700
    lineHeight: 0.95
    letterSpacing: "0.01em"
  headline:
    fontFamily: "Univers Condensed, Helvetica Neue Condensed, Arial Narrow, Roboto Condensed, DejaVu Sans Condensed, Liberation Sans Narrow, Nimbus Sans Narrow, system-ui, sans-serif"
    fontSize: "1.45rem"
    fontWeight: 700
    lineHeight: 1
    letterSpacing: "0.03em"
  title:
    fontFamily: "Univers Condensed, Helvetica Neue Condensed, Arial Narrow, Roboto Condensed, DejaVu Sans Condensed, Liberation Sans Narrow, Nimbus Sans Narrow, system-ui, sans-serif"
    fontSize: "1.12rem"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "0.05em"
  subtitle:
    fontFamily: "Univers Condensed, Helvetica Neue Condensed, Arial Narrow, Roboto Condensed, DejaVu Sans Condensed, Liberation Sans Narrow, Nimbus Sans Narrow, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 700
    lineHeight: 1.25
    letterSpacing: "0.04em"
  tab:
    fontFamily: "Univers Condensed, Helvetica Neue Condensed, Arial Narrow, Roboto Condensed, DejaVu Sans Condensed, Liberation Sans Narrow, Nimbus Sans Narrow, system-ui, sans-serif"
    fontSize: "0.98rem"
    fontWeight: 600
    lineHeight: 1.1
    letterSpacing: "0.05em"
  plate:
    fontFamily: "Univers Condensed, Helvetica Neue Condensed, Arial Narrow, Roboto Condensed, DejaVu Sans Condensed, Liberation Sans Narrow, Nimbus Sans Narrow, system-ui, sans-serif"
    fontSize: "1.02rem"
    fontWeight: 700
    lineHeight: 1
    letterSpacing: "0.08em"
  lede:
    fontFamily: "Iowan Old Style, Charter, Bitstream Charter, Sitka Text, Cambria, DejaVu Serif, Georgia, serif"
    fontSize: "1.05rem"
    fontWeight: 400
    lineHeight: 1.55
  book:
    fontFamily: "Iowan Old Style, Charter, Bitstream Charter, Sitka Text, Cambria, DejaVu Serif, Georgia, serif"
    fontSize: "0.95rem"
    fontWeight: 400
    lineHeight: 1.5
  body:
    fontFamily: "system-ui, -apple-system, Segoe UI, Roboto, Noto Sans, sans-serif"
    fontSize: "16px"
    fontWeight: 400
    lineHeight: 1.5
    fontFeature: "tnum"
  label:
    fontFamily: "system-ui, -apple-system, Segoe UI, Roboto, Noto Sans, sans-serif"
    fontSize: "0.86rem"
    fontWeight: 600
  mono:
    fontFamily: "ui-monospace, SF Mono, JetBrains Mono, DejaVu Sans Mono, monospace"
    fontSize: "0.86rem"
    fontWeight: 400
  slip:
    fontFamily: "Univers Condensed, Helvetica Neue Condensed, Arial Narrow, Roboto Condensed, DejaVu Sans Condensed, Liberation Sans Narrow, Nimbus Sans Narrow, system-ui, sans-serif"
    fontSize: "0.78rem"
    fontWeight: 700
    lineHeight: 1
    letterSpacing: "0.06em"
rounded:
  bar: "2px"
  plate: "3px"
  sheet: "6px"
  tab: "8px"
  pill: "999px"
spacing:
  hairline-gap: "4px"
  row-gap: "10px"
  plate-gap: "12px"
  gutter-phone: "16px"
  block-top: "22px"
  sheet-y: "28px"
  sheet-x: "32px"
  column-gap: "40px"
components:
  sheet:
    backgroundColor: "{colors.sheet}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sheet}"
    padding: "28px 32px 36px"
  sheet-phone:
    backgroundColor: "{colors.sheet}"
    padding: "20px 16px 28px"
  button-primary:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.sheet}"
    typography: "{typography.body}"
    rounded: "{rounded.sheet}"
    padding: "9px 12px"
    height: "44px"
  button-plain:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    rounded: "{rounded.sheet}"
    padding: "9px 12px"
    height: "44px"
  button-danger:
    backgroundColor: "transparent"
    textColor: "{colors.problem-ink}"
    rounded: "{rounded.sheet}"
    padding: "9px 12px"
    height: "44px"
  input:
    backgroundColor: "{colors.field}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.sheet}"
    padding: "9px 12px"
    height: "44px"
    width: "600px"
  field-label:
    textColor: "{colors.ink-2}"
    typography: "{typography.label}"
  tab-home:
    backgroundColor: "{colors.hue-home}"
    textColor: "{colors.on-hue-ink}"
    typography: "{typography.tab}"
    rounded: "{rounded.tab}"
    padding: "0 14px 0 28px"
    height: "48px"
  tab-setup:
    backgroundColor: "{colors.hue-setup}"
    textColor: "{colors.on-hue-white}"
  tab-try:
    backgroundColor: "{colors.hue-try}"
    textColor: "{colors.on-hue-ink}"
  tab-commands:
    backgroundColor: "{colors.hue-commands}"
    textColor: "{colors.on-hue-ink}"
  tab-activity:
    backgroundColor: "{colors.hue-activity}"
    textColor: "{colors.on-hue-ink}"
  tab-help:
    backgroundColor: "{colors.hue-help}"
    textColor: "{colors.on-hue-white}"
  tab-expert:
    backgroundColor: "{colors.hue-expert}"
    textColor: "{colors.on-hue-white}"
  tab-phone:
    rounded: "{rounded.tab}"
    padding: "0 13px"
    height: "44px"
  engine-plate:
    backgroundColor: "{colors.field}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sheet}"
    padding: "14px 16px"
  plate-label:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.sheet}"
    typography: "{typography.plate}"
    rounded: "{rounded.plate}"
    padding: "7px 10px 6px"
    width: "7.5rem"
  slip:
    backgroundColor: "{colors.problem}"
    textColor: "{colors.on-hue-white}"
    typography: "{typography.slip}"
    rounded: "{rounded.plate}"
    padding: "4px 7px"
  pill:
    textColor: "{colors.ink}"
    rounded: "{rounded.pill}"
    padding: "2px 8px"
  timing-bar:
    backgroundColor: "{colors.edge-2}"
    rounded: "{rounded.bar}"
    height: "12px"
    width: "560px"
  toast:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.sheet}"
    rounded: "{rounded.sheet}"
    padding: "11px 18px"
  toast-alert:
    backgroundColor: "{colors.problem}"
    textColor: "{colors.on-hue-white}"
---

# Design System: EMO open server admin

## Overview

**Creative North Star: "The Tabbed Manual"**

The admin page is a well-made manual for one desk robot. Seven sections each own a colour tab at full strength; the tabs hang off the fore edge of a single milk-white sheet, and whichever section is open lends its colour to the whole board the sheet lies on. You always know where you are because the room itself has changed colour. Everything on the sheet is ink: black text, hairline rules, drawn state marks. Vermilion is the one other colour allowed on the sheet, and it means something is wrong.

Density is that of a printed reference card, not a dashboard. Sections on the sheet are separated by rules rather than boxed, explanations are set in a book serif so they read as prose, headings and labels are condensed grotesk capitals so they read as signage, and machine values (timings, codes, commands) are set in mono. The page refuses the white-sidebar SaaS dashboard and the cartoon-robot page; it should feel like equipment documentation that someone cared about.

The page is one self-contained file served under a strict Content Security Policy: inline CSS and JS only, no web fonts, no images, no external hosts. The type stacks are therefore system stacks by design and every icon is inline SVG drawn in the page's own stroke.

**Key Characteristics:**
- One colour per section, at full strength, used for the tab and the board; never inside the sheet's content.
- Milk-white sheet (never cream) with a hairline edge, lifted off the board by a soft two-layer shadow.
- Ink-only content: state is shown by drawn marks (filled check, hollow ring, vermilion cross), not by traffic-light colour.
- Three voices of type: condensed caps for heads, book serif for explanations, mono for machine values.
- Engines wear fixed name plates; risky buttons stand alone at the far end of their row.
- Section changes hinge in a 90 ms two-step with no easing.

## Colors

Seven saturated section hues around a strictly neutral ink-on-milk core, with vermilion held back as the only alarm.

### Primary
- **Chrome Yellow** (`hue-home`): Home tab and the board while Home is open; ink text on it.
- **Burnt Sienna** (`hue-setup`): Setup tab and board; white text on it.
- **Oxide Orange** (`hue-try`): Try it tab and board; ink text on it.
- **Grass Green** (`hue-commands`): Your commands tab and board; ink text on it.
- **Deep Teal** (`hue-activity`): Activity tab and board; ink text on it.
- **Ultramarine** (`hue-help`): Help tab and board; white text on it.
- **Violet** (`hue-expert`): Expert tab and board; white text on it. Expert is set apart from the other six by an extra gap in the rail.

The active hue is exposed as `--hue` (and `--board`, `--on-hue`) by switching `data-s` on the root element. Inside the sheet the hue appears only in text selection; the caret and focus rings are ink, because a light hue (Home yellow) on the white sheet is too faint to see.

### Tertiary
- **Vermilion** (`problem`): the overrun segment of a timing bar and the cross mark. The "Problem" / "too slow" slip and the error toast use the deeper `slip` red, so white text on it stays at least 4.5:1 in both themes.
- **Vermilion Ink** (`problem-ink`): text-weight vermilion for error messages, `.bad` marks and the outline of destructive buttons.

### Neutral
- **Milk Sheet** (`sheet`): the sheet surface; also the text colour on ink fills.
- **Field White** (`field`): inputs, engine plates and code blocks, one step brighter than the sheet.
- **Ink** (`ink`): all body text, primary buttons, plate labels, the timing-bar stages and the default focus outline.
- **Graphite** (`ink-2`): secondary text: ledes, notes, field labels, table heads, meta lines, the waiting mark.
- **Hairline** (`edge`): sheet edge, section rules, input and plate borders.
- **Faint Rule** (`edge-2`): row dividers inside lists and tables, and the empty track of the timing bar.

### Dark theme
Under `prefers-color-scheme: dark` only the neutrals and vermilion change (`sheet-dark`, `field-dark`, `ink-dark`, `ink-2-dark`, `edge-dark`, `edge-2-dark`, `problem-dark`, `problem-ink-dark`). The sheet becomes charcoal, fields go darker than the sheet, and ink becomes near-white. The seven section hues and their on-hue text colours are the same in both themes, so the board looks identical in light and dark; only the sheet inverts.

### Named Rules
**The Board Rule.** A section's hue belongs to its tab and to the board. Content on the sheet never uses a section hue for meaning or decoration.

**The One Alarm Rule.** Vermilion means a problem or a destructive action, and nothing else. Healthy and waiting states are ink and graphite.

## Typography

**Display Font:** a condensed grotesk system stack (Univers Condensed, then Helvetica Neue Condensed, Arial Narrow, Roboto Condensed and other narrow faces, falling back to system-ui), always uppercase with `font-stretch: condensed`.
**Body Font:** the system UI stack (system-ui, -apple-system, Segoe UI, Roboto, Noto Sans) for controls and plain text, with tabular numerals on by default.
**Book Font:** a book serif stack (Iowan Old Style, Charter, Bitstream Charter, Sitka Text, Cambria, DejaVu Serif, Georgia) for explanations and for anything EMO heard or said.
**Mono Font:** ui-monospace, SF Mono, JetBrains Mono, DejaVu Sans Mono for codes, commands, timings and textareas.

**Character:** condensed capitals do the signage, the serif does the explaining, mono does the machine talk. Which face appears tells you what kind of text it is.

**Constraint:** the CSP forbids web fonts, so every face is a system stack and rendering varies by platform (on Linux the condensed role usually lands on Roboto Condensed or DejaVu Sans Condensed). A self-hosted condensed display face is an open follow-up; it would need its licence verified and the owner's sign-off before entering the tree, and it would have to be inlined to satisfy the CSP.

### Hierarchy
- **Display** (700, `clamp(2.6rem, 4.6vw, 4.2rem)`, line-height 0.95, caps): the view title at the top of the sheet. Fixed at 2.6rem on phones.
- **Headline** (700, 1.45rem, caps, 0.03em): the product name in the name bar on the board.
- **Title** (700, 1.12rem, caps, 0.05em): section heads on the sheet.
- **Subtitle** (700, 1rem, caps, 0.04em): sub-heads inside a section (presets, engines).
- **Tab** (600, 0.98rem, caps, 0.05em; 700 when current): the tab rail.
- **Plate** (700, 1.02rem, caps, 0.08em): the engraved engine label.
- **Lede** (book serif, 1.05rem/1.55, max 68ch): the one-line explanation under the view title.
- **Book** (book serif, 0.95rem): notes, plate details, results; capped at 68ch.
- **Body** (system UI, 16px/1.5, tabular numerals): controls and default text.
- **Label** (system UI 600, 0.86rem, graphite): field labels above inputs.
- **Mono** (0.86rem; textareas 0.88rem, timings 0.84rem): machine values.
- **Slip / table head** (condensed caps 700, 0.78rem and 0.82rem): the vermilion slip and table column heads.

### Named Rules
**The Three Voices Rule.** Condensed caps for anything that names, serif for anything that explains, mono for anything a machine produced. Never set a sentence of explanation in condensed caps.

## Layout

A centred desk, max 1200px wide (padding 14px 20px 48px), with the name bar sitting directly on the board above a two-column grid: the sheet (`minmax(0,1fr)`) and a 172px tab rail to its right. The sheet's top-right corner is square so the tabs appear to come out of its fore edge; tabs tuck 12px under the sheet. The sheet has a minimum height of 72vh so the board always frames it.

Inside the sheet, content runs in blocks separated by hairline rules (22px above, 18px below). Home and similar views pair blocks in a `1.15fr / 1fr` two-column grid with a 40px column gap; lists of presets and engines use an auto-fit grid of 240px minimum tracks with a 28px gap. Form rows are flex rows with a 10px gap whose children grow from 220px; fixed-width actions sit at their natural size. Text measure is held at 68ch, inputs at 600px, timing bars at 560px.

Breakpoints: at 860px and below, the desk collapses to one column, the rail becomes a horizontal strip of top-rounded tabs above a full-bleed sheet (no side borders or radius, 16px gutters), the rail scrolls sideways with a fade on its right edge, and every multi-column grid becomes one column. At 600px and below the "signed in" note in the name bar is hidden.

## Elevation & Depth

Depth is physical and limited to the desk metaphor: the sheet lies on the board, and the tabs lie under the sheet. Nothing on the sheet is lifted; content depth is carried by rules, borders and the slightly brighter field white.

### Shadow Vocabulary
- **Sheet lift** (`box-shadow: 0 1px 2px rgba(0,0,0,.14), 0 10px 30px -8px rgba(0,0,0,.28)`): the sheet only.
- **Tab edge** (`box-shadow: 0 2px 1px rgba(0,0,0,.2)`): each rail tab on desktop, a thin card-stock edge.
- **Tab tuck** (`box-shadow: inset 0 -6px 6px -6px rgba(0,0,0,.4)`): phone tabs that are not current, so they read as tucked behind the sheet; the current tab drops it.
- **Toast** (`box-shadow: 0 6px 20px -6px rgba(0,0,0,.45)`): the floating status and error toasts.

### Named Rules
**The Flat Sheet Rule.** Only the sheet, the tabs and the toasts cast shadows. Plates, inputs, buttons and blocks on the sheet are flat.

## Shapes

Small, consistent corners. The sheet, inputs, buttons, plates, code blocks and toasts share a 6px radius; tabs use 8px on their open side only (right side on desktop, top on phones); the engraved plate label and the vermilion slip use a tighter 3px; timing bars and legend swatches 2px; the key-status pill is fully round. Borders are 1px hairlines throughout. The tab's punched hole is a 9px circle drawn as a 1.5px ring, filled for the current tab. State marks are 18px, one 2px stroke weight, drawn as inline SVG.

## Components

### Buttons
Ink blocks, plain and firm.
- **Shape:** gently rounded (6px), minimum 44px tall, 9px 12px padding, weight 600.
- **Primary:** ink fill with sheet-coloured text. Hover brightens by `filter: brightness(1.15)`.
- **Plain:** transparent with an ink label and a hairline border; hover darkens the border to ink.
- **Danger:** transparent with vermilion-ink text and border. Always placed with the `alone` treatment: pushed to the far end of its row with 32px of empty space before it.
- **Disabled:** 50% opacity, no hover.
- **On the board:** the name-bar button is transparent with a `currentColor` border in the on-hue colour.

### Chips
- **Pill:** a round hairline-bordered tag (0.78rem, 600) next to an API-key label, reading "key saved" in ink or "no key yet" in graphite.
- **Slip:** a small deep-red (`slip`) tag in condensed caps with white text ("Problem", "too slow"). The only filled vermilion element besides the error toast.

### Cards / Containers
- **Sheet:** the one container: milk white, hairline edge, 6px radius (square top-right on desktop), 28px 32px 36px padding, sheet-lift shadow.
- **Blocks:** not cards; ruled sections with a 1px hairline top border.
- **Warn box:** a hairline-bordered note in book serif on the sheet colour, shown above the desk when the page is served over plain HTTP.
- **Code block:** field white, hairline border, 6px radius, 12px padding, mono, wraps long lines. A plain "Copy" button sits in its top-right corner.

### Inputs / Fields
- **Style:** field white, 1px hairline border, 6px radius, 9px 12px padding, 44px minimum height, full width up to 600px. Labels sit above in 0.86rem graphite 600. Placeholders are graphite at 80% opacity.
- **Focus:** border turns ink and a 2px ink outline appears 1px outside it; tabs and buttons show the same 2px ink ring with :focus-visible.
- **Checkboxes:** 1.25rem, `accent-color` ink, inside a full-size label row.
- **Textarea:** mono, 5.5rem minimum height.

### Navigation
- **Tab rail (desktop):** a column of tabs 48px tall, each filled with its section hue and set in condensed caps, with a punched-hole marker. Resting tabs are 84% of the rail width; hover extends to 90%; the current tab reaches the full width, goes bold, and fills its hole. Expert sits below a 22px gap.
- **Tab strip (phone):** the same tabs in a horizontally scrolling row above the sheet, rounded on top, 44px tall; the current tab is 52px tall and loses its tuck shadow so it joins the board edge.
- **Name bar:** product name in headline caps and the sign-out button, in on-hue colour directly on the board.
- **Skip link:** an ink pill that drops in at the top-left on focus.

### Engine name plate (signature)
A field-white row with a hairline border and 6px radius: an engraved ink label (condensed caps, 0.08em tracking, 3px radius, minimum 7.5rem wide) naming the role (listening, thinking, voice), the engine in use in book serif, and a state mark with a short word ("Ready"). An optional explanation spans beneath in book serif. On phones the three parts stack and the label hugs the left.

### Timing bar (signature)
A 12px faint-rule track, max 560px, scaled to 12 s, with a dashed 2px ink line marking the robot's 10 s listening window. Each stage of a reply is drawn in ink with a distinct texture rather than a colour: solid (listening), 135° hatch (thinking), vertical stripes (looking), outlined hollow (voice); a zero-length stage is a 2px notch. Any time past the window is filled vermilion. A legend underneath repeats the textures at 10px.

### Checklist rows
A four-column row (mark, name in condensed caps, detail in book serif, timing in mono) divided by faint rules; on phones the detail drops to its own line.

### Toasts
Centred at the bottom, ink fill with sheet text, 6px radius, toast shadow. The error variant is vermilion with white text and stays up longer.

### Motion
Changing section switches the board colour and nudges the sheet 5px sideways back into place, both in 90 ms using `steps(2)`: a deliberate two-frame hinge with no easing. Reduced-motion preference removes all transitions and animations.

## Do's and Don'ts

### Do:
- **Do** give any new section its own full-strength hue, its own tab, and its own on-hue text colour chosen for contrast (ink or white).
- **Do** keep sheet content in ink and graphite; show state with the drawn marks (filled check, hollow ring, vermilion cross) at 18px.
- **Do** separate sections on the sheet with 1px hairline rules, not boxes.
- **Do** set names and heads in condensed caps, explanations in the book serif (max 68ch), and machine values in mono.
- **Do** give every engine a name plate with the role engraved in an ink label.
- **Do** push destructive buttons to the far end of their row with the `alone` spacing.
- **Do** keep touch targets at 44px minimum and keep both light and dark themes working.
- **Do** draw icons as inline SVG in the page's own stroke weight; the CSP allows nothing else.

### Don't:
- **Don't** use a section hue inside the sheet for status, emphasis or decoration.
- **Don't** use vermilion for anything but problems and destructive actions.
- **Don't** use a cream or warm-tinted sheet; it is milk white.
- **Don't** nest a bordered box inside another bordered box on the sheet.
- **Don't** add shadows to anything on the sheet.
- **Don't** add easing curves or long transitions to section changes; the hinge is 90 ms in two steps.
- **Don't** load web fonts, images or anything from another host.
