---
version: 1
slug: "server-ui-admin-html"
primary_target: "server/ui/admin.html"
related_targets: []
---

# Surface brief: admin page (server/ui/admin.html)

Scope: the self-hosted EMO server's admin page, every view. Visitor mode: Operate.
Audience: EMO owners (guided path) and tinkerers (expert page, one step away). Laptop and phone about equally.
Job: see that EMO is healthy, try a phrase or the voice, read what EMO heard and how long it took; set up engines; keep power settings on a separate Expert page.
Constraints: one self-contained file; CSP allows inline CSS/JS only (no fonts, images or hosts: system faces, inline SVG); every existing API call, form and label keeps working; the test needs "EMO open server" on the page; name undecided.
Chosen: Tabbed Manual (challenger rw-manual-acetate-tab-board, chosen by the owner: "fun, easy to locate, not generic").
Memorable moment: the coloured tab rail; you always know where you are because the open section's colour is the board the page sits on.
Unresolved: product name.

## Direction contract
THESIS: The admin page is a well-made manual for one robot: each section owns a colour tab, and the open section's colour becomes the board under a clear sheet. Refuses the white-sidebar SaaS dashboard and the cartoon-robot page.
OWN-WORLD: Section hues at full strength (chrome yellow Home, sienna Setup, oxide orange Try it, grass Commands, teal Activity, ultramarine Help, violet Expert); milk-white sheet with hairline edge, never cream; ink black text; vermilion held out for problems only; condensed grotesk caps for heads, book serif for explanations, mono for machine values. Raises: one ink, one meaning (from orienteering); every engine carries a fixed name plate (from character catalog); risky buttons stand alone in empty space (from developer console).
STORY: An owner opens it, sees at once whether EMO is fine, tries a phrase, and only goes to Expert on purpose.
FIRST VIEWPORT: Hue board with the name bar on top; the clear sheet fills the left; the stepped tab rail hangs on the sheet's right edge (a strip across the top on phones). Home sheet: robot line, engine name plates with state marks and a Check now action, try-a-phrase and hear-the-voice, then recent activity with a stage-timing bar against the robot's listening window.
FORM: Tabbed manual, challenger (position: competitive alternate, chosen over the rolled Field Notebook). seed: b6c78ec722fcdc7d
SIGNATURE: the stepped tab rail and board recolour; a section change hinges in a 90 ms two-step, no easing. Stage-timing bars show where each reply's time went.
FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
