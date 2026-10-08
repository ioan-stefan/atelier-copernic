# Atelier Copernic

A website for a fictional Geneva atelier that makes bespoke astrolabes and celestial chronometers. It is written in Go with the standard library only and sends **zero JavaScript** to the browser. The Content-Security-Policy header sets `script-src 'none'`, so the browser itself blocks any script.

## Run

Requires Go 1.22 or newer.

```bash
go run .
```

Then open http://localhost:8080. Set `PORT` or `ADDR` to listen elsewhere.

To build a single self-contained binary (templates and CSS are embedded):

```bash
go build -o atelier .
```

To run the test suite (gear solver, validation, every route, the full dossier flow, and a check that no page contains a `<script>` tag):

```bash
go test ./...
```

## Views

| Route | View |
| --- | --- |
| `/` | **The Vault**: an editorial showcase of current editions, with tabbed specifications |
| `/commission` | **Commission Dossier**: a four-section inquiry, then review, confirm and receipt |
| `/ledger` | **The Ephemeris Ledger**: a searchable, sortable archive with certificate popovers |
| `/ledger/{ref}` | One delivered piece: its caliber breakdown, provenance and certificate |

## Architecture

| File | Responsibility |
| --- | --- |
| `main.go` | Configuration, server timeouts, graceful shutdown |
| `handlers.go` | Routes, view models, handlers, security and logging middleware |
| `catalog.go` | Domain types and the in-memory catalogue (mock data) |
| `ledger.go` | Ledger query parsing, search, sorting and sort headers |
| `commission.go` | Dossier input, validation, in-memory store, CSRF |
| `astrolabe.go` | Stereographic projection that draws each plate as an accessible SVG |
| `gears.go` | Two-stage gear-train solver for sidereal, synodic and draconic rates |
| `render.go` | Template sets per page, buffered rendering, gzip, fingerprinted assets |
| `format.go` | Latitude, arc, currency and search-folding helpers |
| `templates/` | Layout, shared partials and one template per page |
| `static/styles.css` | Design tokens, layout, CSS-only state engines, motion |

### Computed, not typed

The technical figures are computed rather than typed in by hand. Every tympan drawing is a true stereographic projection for its latitude: the almucantars, azimuths, horizon, twilight line, ecliptic ring and star positions all come from J2000 coordinates. The ecliptic eccentricity ratio comes from the obliquity. Each gear train is searched tooth by tooth against the astronomical period it keeps, and its drift is shown. The names, provenance, prices and editions are mock data.

### Interactivity without JavaScript

- **Edition tabs**: hidden radio inputs placed as siblings of the tabs and panels, driven by `:checked ~` selectors. They work in every browser and with arrow keys.
- **Dossier sections**: the same radio technique. The server re-opens the section holding the first error.
- **Live dossier summary**: `:has()` reads the selected hemisphere, tier and alloy (including `<option>:checked`). It sits behind `@supports`.
- **Certificates**: `<dialog popover>` opened by `popovertarget`. Browsers without the popover API get a link to the entry page instead.
- **Disclosures**: `<details>`, animated with `::details-content` where supported.
- **Ledger**: GET-form search and filters with header-link sorting, so every view has its own URL. A container query turns table rows into cards in narrow layouts.
- **Validation**: the server is the single source of truth. The form uses `novalidate` because hidden sections cannot show native validation bubbles. `:user-invalid` still styles fields as the visitor types.
