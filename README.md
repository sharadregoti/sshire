# sshire

Apply to jobs over SSH instead of a web form.

Candidates run `ssh careers.yourcompany.com`. They land in a terminal app: browse open roles, read the description, fill out an apply form. No web page, no `<form>` for a scraper to find, no public URL for an AI apply-bot to crawl or index.

This was inspired by [superlogical.com](https://www.superlogical.com), whose careers link is literally `ssh superlogical.jobs`.

## Why this blocks most automated applications

Connecting without a real terminal gets rejected outright:

```
$ ssh -o BatchMode=yes careers.yourcompany.com
Requires an active PTY
```

Scrapers and mass-apply tools are built to fill out HTML forms or POST to a REST endpoint. There is neither here — just a terminal program that requires an interactive pty to even start. It is not bulletproof against a determined, purpose-built agent, but it filters out the overwhelming majority of automated, spray-and-pray applications.

## The simplest setup: just an email address

You don't need any sinks, webhooks, or an ATS to use this. Set `apply_email` on a job (or `company.apply_email` as a fallback for every job) and the detail screen just tells the candidate where to send their application — same as [superlogical.jobs](https://www.superlogical.com) does it:

```yaml
jobs:
  - id: backend-eng
    title: "Backend Engineer"
    apply_email: "jobs@acme.example"
    description: |
      ...
```

No `apply_email`? That job falls back to the in-TUI apply form below, with submissions routed to whichever sinks you enable.

## Quick start

```sh
cp config.example.yaml config.yaml
# edit config.yaml: company name, jobs, which sinks to enable
go run ./cmd/sshire -config config.yaml
```

In another terminal:

```sh
ssh -p 2222 localhost
```

## Going to production

1. Point a DNS record (e.g. `careers.yourcompany.com`) at your server.
2. Run `sshire` on port 22. Binding port 22 needs either root or, on Linux, `sudo setcap 'cap_net_bind_service=+ep' ./sshire`.
3. Keep `.sshire/host_key` (or wherever `ssh.host_key_path` points) across restarts — losing it changes your host key fingerprint and every returning candidate's client will warn about it.

A `Dockerfile` is included. Mount a volume at `/data` for `config.yaml`, the host key, and the SQLite dashboard database.

## Where applications go

Enable any combination of these in `config.yaml`:

- **webhook** — a plain JSON POST of the application to a URL you own.
- **slack** — the same idea, formatted as a Slack message for an incoming webhook.
- **ats** — pushes into an existing applicant tracking system. Only Greenhouse is implemented (`internal/sink/ats_greenhouse.go`), and it is **not verified against a live Greenhouse account** — check the field names against Greenhouse's current Harvest API docs before relying on it. Use it as the template for Lever, Ashby, or anything else: implement the `sink.Sink` interface and wire it up in `cmd/sshire/main.go`.
- **dashboard** — stores every application in a local SQLite file and serves a small HTTP-Basic-Auth-protected page to review them. No ATS required.

A failure in one sink never blocks the others, and never blocks the candidate's "submitted" confirmation.

## Architecture

- `internal/tui` — the bubbletea app a candidate sees: job list → detail (a scrollable [viewport](https://github.com/charmbracelet/bubbles), for job postings longer than one screen) → either a plain "email us" line or a [huh](https://github.com/charmbracelet/huh) apply form → confirmation.
- `internal/sshserver` — wires up [wish](https://github.com/charmbracelet/wish): guest-only auth (anyone can connect — it's a public careers page, not a login), the `activeterm` middleware that produces the pty-required rejection above, and the bubbletea program per session.
- `internal/sink` — where a submitted application goes; see above.
- `internal/store` / `internal/dashboard` — the built-in SQLite storage and its read-only web UI.
- `internal/config` — loads `config.yaml`.

## Testing

- `internal/tui/model_test.go` scripts the whole apply flow against the bubbletea model directly (via `teatest`), including a real regression test for a pointer-aliasing bug that once made every submission arrive empty.
- `internal/sshserver/server_test.go` drives the same flow over an actual SSH connection with a requested pty, and separately asserts that a connection without one gets rejected — the same "no bots" behavior confirmed live against `superlogical.jobs`.
- `internal/tui/model_test.go` also covers the email-only apply path and detail-screen scrolling for a description longer than the terminal.

```sh
go test ./...
```

## License

MIT — see `LICENSE`.
