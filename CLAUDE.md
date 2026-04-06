# The Box — Agent Handover

## What This Is

A recreation of the 1990s interactive music video TV channel "The Box". Users call a phone number (via Jambonz IVR) or use a web page to enter a 3-digit catalogue code. The requested music video gets queued and plays on a full-screen channel output, with a ticker bar cycling through the catalogue and upcoming requests — like teletext meets MTV.

## Architecture

**Go backend (`boxd`)** — single binary. Embeds the React frontend at compile time via `//go:embed`. Serves API, WebSocket, media files, IVR webhooks, and the channel SPA. SQLite with WAL mode, single writer.

**React frontend (`web/channel/`)** — Vite + React 18. Connects to backend via WebSocket at `/ws`. The playout controller on the backend drives all state — the frontend is a dumb renderer. No client-side routing.

**Two binaries:** `boxd` (server) and `boxctl` (CLI for catalogue management).

## Build

Frontend must be built before backend (assets are embedded):
```
make build   # or: cd web/channel && npm run build && cd ../.. && go build -o boxd ./cmd/boxd
```

**Important:** Vite outputs to `cmd/boxd/static/` with `assetsDir: 'static'` (not the default `assets/`) to avoid a route collision with `/assets/` which serves jingles from disk.

## Key Design Decisions

- **3-digit catalogue codes** (001-999), not 4-digit. Codes auto-increment.
- **Videos start muted then unmute** to satisfy browser autoplay policy. For headless capture pipelines, use `--autoplay-policy=no-user-gesture-required`.
- **Stale request cleanup on startup** — `playing`/`fetching` requests get reset to `queued` when `boxd` starts, so unclean shutdowns don't leave the queue stuck.
- **yt-dlp path** defaults to `"yt-dlp"` (PATH lookup), not a hardcoded absolute path. Override in `configs/config.yaml` if needed.
- **Prefetch worker** runs every 5s, fetches and transcodes the next N videos in the queue ahead of time.

## Project Layout

```
cmd/boxd/main.go          — HTTP server, API handlers, embedded SPA
cmd/boxctl/main.go         — CLI tool for catalogue management
internal/
  catalogue/catalogue.go   — CRUD for video catalogue (SQLite)
  config/config.go         — YAML config with sensible defaults
  db/db.go                 — SQLite setup, auto-migration
  fetcher/fetcher.go       — yt-dlp download, FFmpeg transcode, cache eviction
  ivr/handlers.go          — Jambonz webhook handlers (call, DTMF, status)
  playout/playout.go       — State machine: filler → transition → playing → filler
  queue/queue.go           — Request queue with rate limiting and dedup
  ws/hub.go                — WebSocket hub with last-state replay for new clients
web/channel/               — React frontend (Vite)
configs/config.yaml        — Runtime config
```

## Playout State Machine

`filler` → `transition` → `playing` → back to `filler` (or next in queue)

- **Filler** cycles between ident screen and catalogue scroll. After `filler_random_delay_minutes`, plays a random cached video.
- **Transition** shows "coming up" overlay for `transition_seconds`.
- **Playing** streams video. Safety timer advances queue if renderer doesn't report `video_ended`.

## Common Tasks

**Add a video:** `./boxctl add --youtube <YOUTUBE_ID>` or `POST /api/catalogue` with `{"youtube_id": "..."}`

**Request a video:** `POST /api/queue` with `{"code": "001"}`

**Pre-cache a video:** `POST /api/catalogue/001/cache`

**Skip current:** `POST /api/queue/skip`

## Gotchas

- The frontend embed means you must rebuild both frontend AND backend after any React/CSS change: `cd web/channel && npm run build && cd ../.. && go build -o boxd ./cmd/boxd`
- Browser cache can be aggressive — hard refresh (`Cmd+Shift+R`) after rebuilds.
- SQLite single-writer: `db.SetMaxOpenConns(1)`. Don't try to parallelise writes.
- The `handleRequestPage` in `cmd/boxd/main.go` is inline HTML via `fmt.Fprintf`, not a template. Phone number is injected from config.
