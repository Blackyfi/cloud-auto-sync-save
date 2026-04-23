# cass — Cloud Auto Sync Save

Self-hosted, encrypted, versioned backup for laptop → home-lab server → external SSDs.

- [`docs/SPEC.md`](docs/SPEC.md) — full v1 design
- [`docs/UPGRADE.md`](docs/UPGRADE.md) — deferred work (macOS client, proper CA, off-LAN access, etc.)

## Status

**Phase 0** — server skeleton with admin UI. Spin it up, create the admin account, see the empty dashboard.

Subsequent phases (per SPEC.md §13):
- Phase 1 — backup MVP (CLI)
- Phase 2 — restore + retention
- Phase 3 — Windows desktop app + scheduling
- Phase 4 — SSD replication + email + dashboard wiring
- Phase 5 — hardening (verify, restore tests, throttling)
- Phase 6 — in-browser WASM restore

## Phase 0 quickstart

Requires Docker.

```sh
cd deploy
docker compose up --build
```

Open <https://localhost:8443>. The browser will warn about the self-signed cert — that's expected (TOFU model, see SPEC §6).

The TLS fingerprint is printed to the container logs on first start:

```
TLS fingerprint (SHA-256): AB:CD:EF:...
```

Save it — future clients (`cass-cli`, `cass-agent`) will pin this fingerprint on their first connect.

On first visit you'll be redirected to `/setup` to create the admin account. After that, sign in at `/login`.

### Stop / reset

```sh
docker compose down              # stop, keep data
docker compose down -v           # stop, wipe the volume (admin user, TLS cert, session key, DB)
```

## Repo layout

```
server/
  cmd/cass-server/        # entrypoint
  internal/
    config/               # env-var config
    db/                   # SQLite + embedded migrations
    auth/                 # Argon2id passwords, signed-cookie sessions
    tlsutil/              # auto-generated self-signed cert (TOFU)
    web/                  # HTTP server, handlers, HTML templates
deploy/
  Dockerfile.server
  docker-compose.yml
docs/
  SPEC.md
  UPGRADE.md
```
