# NFC Agent — AI Guide

## What This Is

NFC Agent is a **local HTTP/WebSocket bridge** (inspired by DYMO Connect JS SDK and Zebra BrowserPrint) that lets browser-based and desktop apps communicate with USB NFC readers. Built by SimplyPrint for use on simplyprint.io, also open-source for third parties.

**Architecture:**
```
Web App / SimplyPrint ↔ HTTP/WS localhost:32145 ↔ NFC Agent (Go) ↔ PC/SC ↔ USB Reader ↔ NFC Card
```

Cross-platform: macOS, Windows, Linux. Go 1.22+, CGO required (PC/SC).

## Design Philosophy

**NFC Agent does NOT parse brand-specific tag data.** It exposes raw block/page access via API. Creality, Anycubic, Qidi, etc. data is parsed by SimplyPrint's backend or mobile app — not here. This avoids duplicating logic.

**Exception:** OpenPrintTag has full native encode/decode in `internal/openprinttag/` — this is intentional.

## Build & Run

```bash
# Build and run (with system tray, normal mode)
go build -o nfc-agent ./cmd/nfc-agent && ./nfc-agent

# Headless only (for CI/servers without a display)
./nfc-agent --no-tray

# Tests
go test -v -race ./...

# Install auto-start service
./nfc-agent install
```

## Git Rules

- **Never add yourself (AI) as a co-author** in commit messages. No `Co-Authored-By` lines.
- **Never commit, push, or create tags** without being explicitly asked to do so.
- When asked to commit: stage specific files by name, write a concise message, do not push unless told.
- Tagging for release (`git tag v1.x.x && git push origin v1.x.x`) triggers CI builds and SDK publishing — only do this when explicitly instructed.

## ALWAYS Test After Changes

After any code change: **build and call the API to verify.** The agent starts in seconds. It's trivial to test — just curl an endpoint or run a script.

```bash
go build -o nfc-agent ./cmd/nfc-agent && ./nfc-agent &
curl http://127.0.0.1:32145/v1/health
python3 scripts/debug_api.py
```

**API change rule:** When any HTTP/WS API changes → update `README.md`, `sdk/javascript/README.md`, `sdk/python/README.md`, AND `.ai/api.md`.

## API Quick Reference

Base URL: `http://127.0.0.1:32145/v1`
WebSocket: `ws://127.0.0.1:32145/v1/ws`

```bash
curl http://127.0.0.1:32145/v1/health                          # health check
curl http://127.0.0.1:32145/v1/readers                         # list readers
curl http://127.0.0.1:32145/v1/readers/0/card                  # read card (NDEF + meta)
curl "http://127.0.0.1:32145/v1/readers/0/mifare/4?key=FFFFFFFFFFFF&keyType=A"  # read MIFARE block
curl http://127.0.0.1:32145/v1/readers/0/ultralight/4          # read NTAG page
```

Full API reference: [`.ai/api.md`](.ai/api.md)

## Key Scripts

| Script | Purpose |
|--------|---------|
| `scripts/debug_api.py` | API health check — run this first |
| `scripts/inspect_tag.py` | Generic tag inspector (UID, type, NDEF, raw pages) |
| `scripts/read_creality.py` | Read + decrypt a Creality CFS RFID tag |
| `scripts/test_write.py` | Test NTAG page writes + MIFARE block writes |
| `scripts/capture_tag_data.py` | Capture APDU data for regression testing |

## Detailed Docs

| File | Contents |
|------|---------|
| [`.ai/api.md`](.ai/api.md) | Full HTTP + WebSocket API with curl examples |
| [`.ai/standards.md`](.ai/standards.md) | NFC standards: Creality, Anycubic, Qidi, OpenPrintTag, etc. |
| [`.ai/build-release.md`](.ai/build-release.md) | Build, CI/CD, tagging, release process |
| [`.ai/debugging.md`](.ai/debugging.md) | Reader debugging, APDU commands, troubleshooting |

## Key Files

| Path | Purpose |
|------|---------|
| `cmd/nfc-agent/main.go` | Entry point, CLI flags |
| `internal/api/http.go` | HTTP endpoint registration & handlers |
| `internal/api/websocket.go` | WebSocket message handlers |
| `internal/core/card.go` | Card detection, APDU logic, NDEF parsing |
| `internal/core/readers.go` | Reader enumeration (PC/SC + Proxmark3) |
| `internal/openprinttag/` | OpenPrintTag encode/decode (CBOR, UUIDv5) |
| `internal/config/config.go` | Environment variable config |
| `.goreleaser.yaml` | Linux release packaging config |
| `.github/workflows/build.yml` | Full release CI pipeline |
