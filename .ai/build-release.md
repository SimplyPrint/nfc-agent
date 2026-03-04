# Build, CI/CD & Release

## Prerequisites

```bash
# macOS — PC/SC built-in, no extra deps

# Linux
sudo apt install libpcsclite-dev      # Debian/Ubuntu
sudo dnf install pcsc-lite-devel      # Fedora/RHEL

# Windows — PC/SC (WinSCard) built-in
```

Go 1.22+ required. CGO must be enabled (it is by default).

---

## Build & Run

```bash
# Normal build + run (with system tray)
go build -o nfc-agent ./cmd/nfc-agent && ./nfc-agent

# Headless only — use for CI, servers, or environments without a display
./nfc-agent --no-tray

# Install auto-start service (XDG autostart on Linux, LaunchAgent on macOS, Task Scheduler on Windows)
./nfc-agent install
./nfc-agent uninstall

# Show version
./nfc-agent version
```

**Always test after building:**
```bash
go build -o nfc-agent ./cmd/nfc-agent && ./nfc-agent &
sleep 1
curl http://127.0.0.1:32145/v1/health
python3 scripts/debug_api.py
```

---

## Run Tests

```bash
# Full test suite with race detector
go test -v -race ./...

# With coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Tests run on Ubuntu, macOS, and Windows in CI via `.github/workflows/ci.yml`.

---

## Version Injection

Version info is injected at build time via ldflags:

```bash
go build \
  -ldflags="-X 'github.com/SimplyPrint/nfc-agent/internal/api.Version=1.2.3' \
             -X 'github.com/SimplyPrint/nfc-agent/internal/api.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)' \
             -X 'github.com/SimplyPrint/nfc-agent/internal/api.GitCommit=$(git rev-parse --short HEAD)'" \
  -o nfc-agent ./cmd/nfc-agent
```

Without ldflags (dev builds), the version is automatically set to `dev-<short-commit>` or `dev-<short-commit>-dirty` using Go's build info (`debug.ReadBuildInfo()`).

---

## Release Process

Releases are triggered by pushing a `v*` git tag:

```bash
git tag v1.2.3
git push origin v1.2.3
```

This triggers `.github/workflows/build.yml` which:
1. Runs tests on Ubuntu
2. Builds for all platforms in parallel
3. Creates platform-specific packages
4. Uploads to GitHub Releases
5. Publishes SDKs (if applicable)

### Platform Build Details

**macOS** (two separate runners):
- `macos-15-intel` → Intel binary (`darwin/amd64`)
- `macos-latest` (Apple Silicon) → ARM binary (`darwin/arm64`)
- Combined into a universal binary via `lipo`
- Packaged as `.dmg` with drag-to-Applications layout
- Code-signed with Apple certificate (if `APPLE_CERT_*` secrets are present)
- Notarized with Apple's notarization service (if `APPLE_*` secrets present)

**Windows** (`windows-latest`):
- `goversioninfo` generates Windows resource file (icon, version info)
- Built as `.exe` with embedded version metadata
- Packaged as Inno Setup installer (`.exe`)

**Linux** (`ubuntu-latest` via GoReleaser):
- `.tar.gz` archive
- `.deb` package (includes systemd user service, desktop entry, kernel module blacklist)
- `.rpm` package
- Post-install script sets up PC/SC daemon and kernel module loading

### SDK Publishing

**JavaScript SDK** (`sdk/javascript/`):
- Published to GitHub Packages (npm registry: `@simplyprint/nfc-agent`)
- Only published if: `sdk/javascript/` directory changed since previous tag AND tag is NOT pre-release
- Workflow: `.github/workflows/sdk-release.yml`

**Python SDK** (`sdk/python/`):
- Published to PyPI as `nfc-agent`
- Same conditions as JS SDK
- Workflow: `.github/workflows/sdk-python-release.yml`

**Homebrew Tap:**
- SimplyPrint tap automatically updated with new SHA256 and version

### Pre-release Tags

Tags containing `-alpha`, `-beta`, `-rc` (e.g. `v1.3.0-beta.1`) are marked as pre-release on GitHub:
- Still builds all platform packages
- Does NOT publish SDKs to npm/PyPI
- Does NOT update Homebrew tap

---

## Configuration (Environment Variables)

| Variable | Default | Description |
|----------|---------|-------------|
| `NFC_AGENT_PORT` | `32145` | HTTP/WS server port |
| `NFC_AGENT_HOST` | `127.0.0.1` | Bind address (localhost only by default) |
| `NFC_AGENT_PROXMARK3` | `0` | Set to `1` to enable Proxmark3 reader support |
| `NFC_AGENT_PM3_PATH` | `pm3` | Path to Proxmark3 `pm3` binary |
| `NFC_AGENT_PM3_PORT` | (auto) | Serial port for Proxmark3 |
| `NFC_AGENT_PM3_PERSISTENT` | `1` | Keep pm3 subprocess alive between commands |
| `NFC_AGENT_PM3_IDLE_TIMEOUT` | `60s` | pm3 subprocess idle timeout |

---

## API Change Checklist

**Whenever any HTTP or WebSocket API changes** (new endpoint, changed request/response, new WS message type), update ALL of:

1. `README.md` — API Overview table and relevant sections
2. `sdk/javascript/README.md` — JS SDK docs
3. `sdk/python/README.md` — Python SDK docs
4. `.ai/api.md` — AI documentation

There is no auto-sync. Always do a project-wide search for the old endpoint name/path when renaming or changing a route.

---

## Key Files

| File | Purpose |
|------|---------|
| `.goreleaser.yaml` | Linux packaging (deb, rpm, tar.gz) |
| `.github/workflows/build.yml` | Full release CI (all platforms + SDK publish) |
| `.github/workflows/ci.yml` | PR/push CI (tests + lint + security scan) |
| `.github/workflows/sdk-release.yml` | JS SDK publish |
| `.github/workflows/sdk-python-release.yml` | Python SDK publish |
| `cmd/nfc-agent/main.go` | Entry point + CLI flag handling |
| `internal/api/http.go` | HTTP endpoint registration (version ldflags live here) |
| `build/darwin/` | macOS app bundle, DMG config, codesign scripts |
| `build/linux/` | systemd service, desktop entry, post-install script |
| `build/windows/` | Inno Setup config, icon resources |
