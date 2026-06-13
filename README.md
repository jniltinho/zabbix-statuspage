# zabbix-statuspage

**A fast, self-hosted public status page powered by your Zabbix monitoring.**

Turn your existing Zabbix setup into a beautiful status page — no extra database, no extra services. Just point it at your Zabbix API and go.

---

## Why zabbix-statuspage?

- **Zero extra infrastructure** — reads directly from Zabbix API, single binary
- **Auto-discovers hosts** — tag a host in Zabbix and it appears on the page automatically
- **Uptime bars** — 90-slot 24h history bar per service, computed from real Zabbix events
- **Three views** — normal, compact, and micro for any screen size
- **Embedded assets** — templates, CSS, and JS compiled into the binary, no static files needed
- **Docker-ready** — multi-stage build, single image under 20 MB
- **TLS support** — serve HTTPS directly without a reverse proxy

---

## Quick start

```bash
# 1. Get the binary
curl -fsSL https://github.com/jniltinho/zabbix-statuspage/releases/latest/download/zabbix-statuspage_linux_amd64.tar.gz \
  | tar -xz

# 2. Configure
cp config.toml.example config.toml
# edit config.toml — set api_url and api_token

# 3. Run
./zabbix-statuspage serve
```

Open [http://localhost:3000](http://localhost:3000).

---

## Auto-discovery in 30 seconds

No segment configuration needed. Add a single tag to any Zabbix host:

| Tag    | Value       |
|--------|-------------|
| output | statuspage  |

The host appears on the status page on the next refresh. Remove the tag to hide it.

---

## Docker

```bash
# docker-compose.yaml already provided — just add your config.toml
docker compose up -d
```

---

## Views

| URL | Description |
|-----|-------------|
| `/` | Normal — one row per service with uptime bar |
| `/?compact=1` | Compact — grid layout for many services |
| `/?micro=1` | Micro — minimal list, great for dashboards |

---

## Documentation

Full setup, configuration reference, build instructions, and CLI docs:

**[docs/README.md](docs/README.md)**

---

## License

MIT
