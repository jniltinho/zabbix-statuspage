# zabbix-statuspage — Full Documentation

## Table of contents

- [Architecture](#architecture)
- [Requirements](#requirements)
- [Installation](#installation)
  - [From release binary](#from-release-binary)
  - [Build from source](#build-from-source)
  - [Docker](#docker)
- [Configuration](#configuration)
  - [Server](#server)
  - [Zabbix API](#zabbix-api)
  - [Trigger tags](#trigger-tags)
  - [Auto-discovery mode](#auto-discovery-mode)
  - [Manual segments mode](#manual-segments-mode)
  - [External status pages](#external-status-pages)
  - [Environment variables](#environment-variables)
- [Zabbix setup](#zabbix-setup)
- [Views](#views)
- [CLI reference](#cli-reference)
- [Building from source](#building-from-source)
- [Release workflow](#release-workflow)

---

## Architecture

```
Zabbix API
    │
    ▼
zabbix-statuspage (single Go binary)
    ├── reads triggers, events, hosts, maintenances
    ├── caches responses (configurable TTL)
    ├── renders HTML via embedded templates
    └── serves HTTP/HTTPS on configured port
```

No database. No Redis. No external dependencies at runtime. All templates, CSS, and JS are compiled into the binary at build time.

---

## Requirements

- Go 1.26+ (build only)
- TailwindCSS v4 standalone binary (build only)
- Zabbix 6.x or 7.x with API token
- Linux/amd64 (other platforms work but releases target amd64)

---

## Installation

### From release binary

```bash
curl -fsSL https://github.com/jniltinho/zabbix-statuspage/releases/latest/download/zabbix-statuspage_linux_amd64.tar.gz \
  | tar -xz -C /usr/local/bin
```

### Build from source

```bash
# Install TailwindCSS standalone binary
curl -fsSL https://github.com/tailwindlabs/tailwindcss/releases/download/v4.2.0/tailwindcss-linux-x64 \
  -o /usr/local/bin/tailwindcss && chmod +x /usr/local/bin/tailwindcss

# Clone and build
git clone https://github.com/jniltinho/zabbix-statuspage
cd zabbix-statuspage
make deps
make build-prod        # compiles CSS + binary + UPX compression
# binary at: bin/zabbix-statuspage
```

### Docker

```bash
git clone https://github.com/jniltinho/zabbix-statuspage
cd zabbix-statuspage
cp config.toml.example config.toml   # edit with your Zabbix credentials
docker compose up -d
```

The Docker image downloads TailwindCSS and compiles CSS during the build stage. The final image is Alpine-based and contains only the binary.

---

## Configuration

Copy `config.toml.example` to `config.toml` and edit as needed.

### Server

```toml
[server]
addr     = "0.0.0.0"  # bind address
port     = 3000        # HTTP port
tls      = false       # enable HTTPS
tls_cert = ""          # path to certificate file (PEM)
tls_key  = ""          # path to private key file (PEM)
```

### Zabbix API

```toml
[zabbix]
api_url   = "https://zabbix.example.com/api_jsonrpc.php"
api_token = "your-api-token-here"
cache_ttl = 30  # seconds
```

Create an API token in Zabbix under **Administration → API tokens**.

### Trigger tags

Tags used to filter which Zabbix triggers appear on the status page. In auto-discovery mode, the same tags are also used to find which hosts to show.

```toml
[[trigger_tags]]
tag   = "output"
value = "statuspage"
```

Multiple tags can be defined — only triggers/hosts matching **all** tags are included.

### Auto-discovery mode

When **no** `[[segments]]` blocks are defined, the application automatically discovers hosts from Zabbix that carry the tags defined in `[[trigger_tags]]`.

This is the recommended mode. No configuration changes are needed when adding or removing hosts — just tag them in Zabbix.

### Manual segments mode

When `[[segments]]` are defined, auto-discovery is disabled. Only hosts listed under segments are shown, grouped into named sections.

```toml
[[segments]]
name        = "Web Services"
description = "Public-facing applications."

  [[segments.services]]
  zabbix_host  = "www.example.com"   # hostname as configured in Zabbix
  display_host = "Website"           # optional display name override

[[segments]]
name = "Mail Infrastructure"

  [[segments.services]]
  zabbix_host  = "mx01.example.com"
  display_host = "MX01"
```

### External status pages

Links rendered in the "External statuspages" section of the normal view.

```toml
[[external_statuspages]]
name        = "Provider A"
url         = "https://status.provider-a.com/"
description = "Optional description."
```

### Environment variables

All `config.toml` values can be overridden with environment variables using the `STATUSPAGE_` prefix and `_` as separator:

```bash
STATUSPAGE_ZABBIX_API_URL=https://zabbix.example.com/api_jsonrpc.php
STATUSPAGE_ZABBIX_API_TOKEN=mytoken
STATUSPAGE_SERVER_PORT=8080
```

---

## Zabbix setup

### Creating an API token

1. Log in to Zabbix as an admin
2. Go to **Administration → API tokens → Create API token**
3. Assign a user with at least **read** access to the relevant hosts
4. Copy the generated token to `config.toml`

### Tagging hosts for auto-discovery

1. Go to **Configuration → Hosts**
2. Open a host and click the **Tags** tab
3. Add the tag configured in `[[trigger_tags]]` (default: `output = statuspage`)
4. Save — the host will appear on the status page within one cache TTL

### Tagging triggers

For triggers to appear on the status page in manual mode, they must also carry the tags defined in `[[trigger_tags]]`. Add the same tag to each trigger under **Configuration → Hosts → Triggers → Tags**.

---

## Views

The status page offers three views accessible via query parameters:

| URL | Description |
|-----|-------------|
| `/` | **Normal** — one row per service. Shows host name, 24h uptime %, status badge, and 90-slot uptime bar. Past problems listed below. |
| `/?compact=1` | **Compact** — multi-column grid. Useful for many services. Resolved events shown as small badges. |
| `/?micro=1` | **Micro** — minimal icon list. Designed for narrow screens or embedded dashboards. |

### Service row (normal view)

Each row shows:

| Element | Description |
|---------|-------------|
| Icon | Green check (operational) or yellow alert (problem) |
| Name | Host display name |
| Uptime % | Percentage of last 24h without problems |
| Status badge | Operational / Degraded / Outage |
| Uptime bar | 90 slots × ~16 min each. Green = OK, Red = problem period |

**Status badge logic:**
- **Operational** — no active triggers
- **Degraded** — active trigger with priority ≤ 3 (Warning / Average)
- **Outage** — active trigger with priority ≥ 4 (High / Disaster)

The page auto-refreshes every 60 seconds in production mode.

---

## CLI reference

```
zabbix-statuspage [command]

Commands:
  serve     Start the HTTP server
  version   Print version, build date, and git commit

Flags for serve:
  --config string     Config file path (default: config.toml)
  --addr   string     Bind address override
  --port   int        Port override
  --tls               Enable TLS
  --tls-cert string   TLS certificate file
  --tls-key  string   TLS private key file
  --debug             Disable cache and auto-refresh (development mode)
```

---

## Building from source

```bash
make deps        # go mod download
make css         # compile web/tailwindcss/input.css → web/static/css/style.css
make build       # css + go build → bin/zabbix-statuspage
make build-prod  # css + go build + UPX → bin/zabbix-statuspage (smaller)
make run         # build + serve
make test        # go test -race ./...
make lint        # golangci-lint run
make clean       # rm -rf bin/ dist/
make docker      # docker build with VERSION/BUILD_DATE/GIT_COMMIT args
```

The compiled CSS (`web/static/css/style.css`) is generated and gitignored. It must be compiled before building the binary since it is embedded in the final binary via `//go:embed`.

---

## Release workflow

Tagged pushes trigger `.github/workflows/release.yml`:

1. Checks out the repo with full history
2. Sets up Go (version from `go.mod`)
3. Installs UPX via apt
4. Downloads TailwindCSS v4 standalone binary
5. Runs `make deps`, `make css`, `make build-prod`
6. Packages `bin/zabbix-statuspage` as `zabbix-statuspage_<version>_linux_amd64.tar.gz`
7. Creates a GitHub release with the archive attached

To trigger a release:

```bash
git tag v1.0.0
git push origin v1.0.0
```
