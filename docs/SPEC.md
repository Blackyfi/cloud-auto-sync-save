# `cass` — Cloud Auto Sync Save: Specification (v1)

> Working name: **cass**. Scope: v1, single household, single laptop, LAN-only.
> Items deferred to later versions live in [`UPGRADE.md`](./UPGRADE.md).

---

## 1. Goal & non-goals

**Goal.** Replace the manual 2-week / 3-SSD ritual with an automated, encrypted, versioned backup system. A Windows laptop pushes selected folders to a home-lab server while on LAN; the server is the source of truth and feeds 3 external SSDs on rotation.

**Non-goals (v1).**
- Real-time file sync — this is **backup**, not Dropbox (see §3).
- Off-LAN / internet / VPN access.
- macOS or Linux desktop clients.
- Mobile clients.
- Multi-user collaboration / shared folders.

## 2. Core requirements

| # | Requirement | Why |
|---|---|---|
| R1 | Immutable point-in-time **snapshots**, not file mirroring | Ransomware on the laptop can't propagate; deletions are recoverable |
| R2 | **Client-side encryption** (AES-256-GCM); server never sees plaintext | Server compromise ≠ data compromise. Same for the SSDs |
| R3 | **Content-defined chunking + per-repo HMAC-keyed dedup** (FastCDC) | Editing a 4 GB video doesn't store 4 GB again. **No cross-repo correlation** (see §5.2) |
| R4 | **Append-only protocol** for client→server | Compromised laptop can't wipe the backup |
| R5 | **Retention policies** (e.g. hourly×24, daily×30, monthly×12) | Bounded storage growth, real version history |
| R6 | **Multi-destination replication** to the 3 SSDs | Preserves the existing 3-2-1 strategy, automates rotation |
| R7 | **Integrity verification + automated restore tests** | Untested backups don't exist |
| R8 | **Atomic snapshots** | No half-saved files in the backup |
| R9 | **Bandwidth/CPU throttling on the client** | Backup mid-day shouldn't tank the laptop |
| R10 | **Observability**: per-device last-success time, repo size, error log, **email alerts** | You'll know within hours if something breaks |
| R11 | **In-browser WASM restore** in admin UI | Browse, decrypt, and download any file from any browser without installing the CLI |

## 3. Backup vs sync — the model

The system stores **snapshots**. A snapshot is an immutable, encrypted, deduplicated tree of all selected files at a point in time. Snapshots are written, never modified. Old snapshots are deleted only by the **retention policy**, never by client action. This is what makes it a backup and not a sync tool.

## 4. Architecture

```
┌──────────────────┐                     ┌─────────────────────────────────────┐
│  Laptop (Win 10+)│   HTTPS over LAN    │   Home Lab Server (Docker host)     │
│                  │  ◀────────────────▶ │                                     │
│  ┌────────────┐  │                     │   ┌───────────────────────────┐     │
│  │ cass-agent │  │                     │   │  cass-server (Go daemon)  │     │
│  │ (Tauri app)│  │                     │   │  - HTTP API                │     │
│  │            │  │                     │   │  - Auth: device tokens     │     │
│  │ - folder   │  │                     │   │  - Append-only repo writer │     │
│  │   picker   │  │                     │   │  - Retention / GC scheduler│     │
│  │ - schedule │  │                     │   │  - Replicator → SSDs       │     │
│  │ - tray     │  │                     │   │  - Verifier / restore-test │     │
│  │ - status   │  │                     │   │  - Email notifier (SMTP)   │     │
│  └─────┬──────┘  │                     │   └───────────┬───────────────┘     │
│        │         │                     │               │                     │
│  ┌─────▼──────┐  │                     │   ┌───────────▼───────────────┐     │
│  │ cass-cli   │  │                     │   │  cass-admin (HTMX + WASM) │     │
│  │ (Go binary)│  │                     │   │  - users / device tokens   │     │
│  │ chunker +  │  │                     │   │  - repos, policies         │     │
│  │ encryptor  │  │                     │   │  - snapshot browser        │     │
│  └────────────┘  │                     │   │  - in-browser restore (WASM)│    │
└──────────────────┘                     │   │  - SSD rotation status     │     │
                                         │   └───────────────────────────┘     │
                                         │                                     │
                                         │  /var/cass/repo  (primary repo)     │
                                         │  /mnt/ssd-A,B,C  (rotated mirrors)  │
                                         └─────────────────────────────────────┘
```

### Components

1. **`cass-server`** — Go daemon shipped as a Docker image. HTTP/JSON API. Stateless re: data plane (state lives in the repo + a small SQLite DB for users/tokens/jobs).
2. **`cass-admin`** — Web UI bundled with the server. Server-rendered (Go templates + HTMX + Tailwind) + a WASM module for in-browser restore (§8).
3. **`cass-cli`** — Go binary on the laptop. Heavy lifting (chunking, encryption, upload). Headless, scriptable.
4. **`cass-agent`** — Windows desktop app. Tauri (Rust shell + web UI). Wraps `cass-cli`, adds folder picker, schedule, system tray, progress. ~10 MB binary.

### Stack

| Choice | Reasoning |
|---|---|
| **Go** for server + CLI | Best-in-class for backup daemons. Single static binary. Strong crypto stdlib. Cross-compiles to Windows trivially |
| **Tauri** for desktop app | ~10 MB binary vs Electron's 100+ MB; native webview; trivial to add macOS/Linux later |
| **HTMX + Tailwind** for admin | Server-rendered, no SPA build, fast to ship solo |
| **WASM** for in-browser restore | Reuses the Go chunker/crypto code via `GOOS=js GOARCH=wasm` — no second crypto implementation to maintain |
| **SQLite** for server metadata | Single file, no separate DB to operate |
| **HTTP + bearer tokens over TLS (TOFU cert)** | Simple, debuggable. Proper CA deferred (UPGRADE.md) |

## 5. Data model

### 5.1 Repository layout (server-side)

```
/var/cass/repo/
├── config.json            # repo version, chunker params, KDF params (NOT the keys)
├── chunks/
│   └── ab/cd/abcdef...    # content-addressable, sharded by first 4 hex chars
│                          # filename = HMAC-SHA256(plaintext, dedup_key)
│                          # content  = nonce || AES-256-GCM(plaintext, repo_key)
├── snapshots/
│   └── <device-id>/
│       └── <snapshot-id>.json.enc   # encrypted manifest
├── keys/
│   └── <key-id>.json      # wrapped(repo_key, dedup_key) under user-derived KEK
└── locks/                 # advisory locks for GC
```

### 5.2 Chunking & dedup — **no convergent encryption leak**

- **FastCDC** with avg 1 MiB chunks (min 256 KiB, max 4 MiB). Edits to large files re-upload only the changed chunks.
- Each chunk's **filename** = `HMAC-SHA256(plaintext, dedup_key)` where `dedup_key` is a 256-bit random secret generated at repo init and wrapped under the user-derived KEK (Argon2id over the repo password).
- Each chunk's **content** = `nonce || AES-256-GCM(plaintext, key=repo_key, aad=hmac_hash)`.
- **Property:** within one repo, identical plaintext → identical filename → automatic dedup. Across two different repos (different `dedup_key`), identical plaintext → different filenames. **No cross-repo correlation, no "you have the same file as user X" leak.**
- The trade-off vs. plain SHA-256 dedup is just one HMAC per chunk on backup — negligible.

### 5.3 Snapshot manifest (plaintext shape, before encryption)

```json
{
  "id": "snap_2026-04-23T10-15-00Z_abc123",
  "device_id": "laptop-nicolas",
  "started_at": "2026-04-23T10:15:00Z",
  "completed_at": "2026-04-23T10:18:42Z",
  "root_paths": ["C:\\Users\\nicolas\\Documents", "D:\\Photos"],
  "stats": { "files": 12453, "bytes_logical": 89234567890, "bytes_new": 124578901 },
  "tree": [
    {
      "path": "Documents/taxes/2025.pdf",
      "mode": "0644",
      "mtime": "2026-04-12T09:11:00Z",
      "size": 234567,
      "chunks": ["hmac:abcd...", "hmac:ef01..."]
    }
  ]
}
```

Manifest is encrypted client-side and stored as `<snapshot-id>.json.enc`.

### 5.4 Server SQLite (metadata only — never file content)

Tables: `users`, `devices`, `device_tokens`, `policies`, `snapshots_index` (denormalized: device_id, id, started_at, byte counts), `replication_targets`, `replication_runs`, `restore_tests`, `email_settings`, `email_outbox`, `audit_log`.

## 6. Security model

- **Repo password** is set at repo init, lives only in the Windows Credential Manager (via `cass-agent`) and the user's password manager. Server never sees it.
- **KDF**: Argon2id (m=256 MiB, t=3, p=1) → KEK → unwraps random `repo_key` and `dedup_key`.
- **Device tokens**: created in admin UI, scoped to one device, append-only permission to one repo. Revocable. Sent as `Authorization: Bearer <token>`.
- **Append-only** server API: only `PUT /chunks/<hash>` (idempotent), `POST /snapshots`, `GET /chunks/<hash>`, `GET /snapshots/...`. **No DELETE for clients.** Deletion only via admin UI / scheduled GC.
- **Transport**: HTTPS with **self-signed cert (TOFU)** in v1. Admin UI shows the server cert fingerprint; client pins it on first connect. (Proper CA: UPGRADE.md.)
- **Admin auth**: username + Argon2id-hashed password + optional TOTP. Sessions in signed cookies.
- **WASM restore session**: the repo password is taken in a browser session, used to derive keys *in the browser*, and **never sent to the server**. The browser fetches encrypted blobs and decrypts locally.

## 7. Client API (HTTP, JSON)

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/v1/repo/config` | Repo params (chunker config, KDF params, wrapped key blobs) |
| `HEAD` | `/v1/chunks/<hash>` | Dedup check: does this chunk already exist? |
| `PUT` | `/v1/chunks/<hash>` | Upload a chunk (idempotent; rejects on hash mismatch) |
| `GET` | `/v1/chunks/<hash>` | Download a chunk (for restore) |
| `POST` | `/v1/snapshots` | Commit a snapshot manifest (atomic; references must exist) |
| `GET` | `/v1/snapshots?device=...` | List snapshots for restore browsing |
| `GET` | `/v1/snapshots/<id>` | Download manifest |

Client backup flow:
1. Walk selected folders → file list.
2. For each file: chunk with FastCDC, compute `HMAC(chunk, dedup_key)`, `HEAD` to check existence.
3. For missing chunks: encrypt with `AES-256-GCM(chunk, repo_key, aad=hmac_hash)`, `PUT`.
4. Build manifest, encrypt, `POST /snapshots`.
5. Server validates all referenced chunk hashes exist, persists manifest, returns snapshot id.

Resumability: chunks are idempotent and the manifest only commits at the end → an interrupted run resumes cheaply.

## 8. Admin UI surface (v1)

- **Dashboard**: per-device last-snapshot age (green/yellow/red), repo size, dedup ratio, last SSD rotation, last restore test result, last email sent.
- **Devices**: list, create token, revoke token, set policy.
- **Snapshots**: browse by device, drill into tree, **download single file or whole subtree** decrypted in-browser via WASM (see below).
- **Policies**: retention rules (hourly/daily/weekly/monthly counts), schedule, included/excluded patterns.
- **Replication**: configure SSD targets, view rotation log, "sync now" button.
- **Maintenance**: trigger GC, integrity check, restore test on demand.
- **Email settings**: SMTP host, port, username, **app password**, from-address, recipients, alert thresholds.
- **Audit log**: every auth event, every snapshot, every admin action.

### 8.1 In-browser WASM restore

- Same Go chunker/crypto code, compiled to WASM (`GOOS=js GOARCH=wasm`). One implementation, no drift.
- Flow: user enters repo password → browser derives KEK with Argon2id (in WASM) → unwraps `repo_key` + `dedup_key` → fetches the encrypted manifest, decrypts → renders the file tree → on file/subtree download, streams encrypted chunks from `/v1/chunks/...`, decrypts in WASM, assembles, triggers a browser download (using the File System Access API for whole-folder restore, falling back to a zip download).
- Repo password lives only in browser memory for the session; never sent to the server.

## 9. Replication to the 3 SSDs

- Each SSD has a label (`cass-ssd-A`, `B`, `C`). A udev rule (or systemd path unit) on the host fires on mount.
- Replicator copies new chunks + new snapshot manifests to the SSD's repo dir using rsync semantics (the layout is rsync-friendly because chunks are immutable).
- Logs the run (`replication_runs` table). Admin UI shows last-sync timestamp per SSD. Email alert if an SSD hasn't been rotated in N days (configurable).
- SSDs hold a **full mirror**, decryptable with the same repo password from any machine running `cass-cli` or the WASM restore UI pointed at a local copy.

## 10. Maintenance jobs (cron inside the container)

| Job | Frequency | What it does |
|---|---|---|
| Retention | nightly | Apply policies, mark snapshots for deletion |
| GC | weekly | Delete chunks no longer referenced by any snapshot |
| Verify | weekly | Re-hash a sample of chunks, alert on mismatch |
| Restore test | monthly | Pick a random snapshot, restore to scratch dir, diff against manifest, **email** on failure |
| Heartbeat check | hourly | If a device's last snapshot is older than its policy + grace, **email** alert |

## 11. Email notifications

- Single SMTP account (Gmail or any provider). User supplies host, port, username, **app password**, from-address.
- Recipients list (comma-separated).
- Alert types: backup overdue, replication overdue, verify failure, restore-test failure, GC error, auth failures over threshold.
- Per-alert-type cooldown (no spam).
- Outbox in SQLite with retries; admin UI shows last 50 emails sent + delivery status.

## 12. Failure modes

| Failure | Behavior |
|---|---|
| Laptop loses network mid-upload | Resumable: chunks idempotent, manifest commits atomically at end |
| Server crash mid-write | Chunks are write-then-rename (atomic on POSIX). Manifest commit is the only "transaction" |
| **Repo password lost** | **Data is unrecoverable.** Admin UI prints a recovery code at init and warns prominently |
| SSD plugged in but corrupted | Replicator detects via verify pass, alerts via email, refuses to overwrite good chunks with bad |
| Clock skew on laptop | Snapshot ids include server-assigned monotonic counter, not just timestamp |
| Email send fails | Retried with exponential backoff; admin UI surfaces the failure |
| Server cert rotated | Client refuses to connect (TOFU pin mismatch) until user confirms new fingerprint in agent UI |

## 13. Phased delivery

**Phase 0 — Skeleton (week 1)**
- Repo layout, Go module, Docker compose, SQLite schema, admin login + dashboard placeholder.
- Goal: `docker compose up` shows an empty admin UI.

**Phase 1 — Backup MVP (weeks 2–3)**
- `cass-cli backup <path>`: chunking, HMAC-keyed dedup, encryption, upload, manifest commit.
- Server: chunk + snapshot endpoints, append-only enforcement.
- Admin UI: device tokens, snapshot list.
- Goal: backup a folder from the laptop CLI, see it in admin UI.

**Phase 2 — Restore + retention (week 4)**
- `cass-cli restore <snapshot-id> <dest>`.
- Retention + GC jobs.
- Goal: prove round-trip works; old snapshots prune correctly.

**Phase 3 — Windows desktop app + scheduling (weeks 5–6)**
- Tauri app (Windows target only): folder picker, policy editor, schedule, system tray, progress.
- Repo password stored in Windows Credential Manager.
- Client-side scheduling (Windows Task Scheduler integration).
- Goal: install the app, click 3 things, never touch the CLI again.

**Phase 4 — SSD replication + email + dashboard (week 7)**
- Replicator + udev/systemd hook.
- SMTP outbox + alert types.
- Admin dashboard with live status tiles.
- Goal: the system actively tells you when something's wrong.

**Phase 5 — Hardening (week 8)**
- Automated weekly verify + monthly restore test.
- Bandwidth/CPU throttling on client.
- Documentation: setup runbook, recovery runbook, threat model.

**Phase 6 — In-browser WASM restore (week 9)**
- Compile shared chunker/crypto to WASM.
- Browser tree view + selective decrypt-and-download.
- File System Access API for whole-folder restore where supported; zip fallback elsewhere.

## 14. Repo structure

```
cloud-auto-sync-save/
├── server/                # Go daemon + admin UI
│   ├── cmd/cass-server/
│   └── internal/
│       ├── api/           # HTTP API for clients (Phase 1+)
│       ├── auth/          # Argon2id passwords, signed-cookie sessions
│       ├── config/        # env-var config
│       ├── db/            # SQLite + embedded migrations
│       ├── repo/          # repository writer / reader (Phase 1+)
│       ├── replication/   # SSD replication (Phase 4)
│       ├── maintenance/   # GC, verify, restore-test (Phase 5)
│       ├── email/         # SMTP outbox (Phase 4)
│       ├── tlsutil/       # auto-generated self-signed cert (TOFU)
│       └── web/           # HTTP server, handlers, HTML templates (HTMX + Tailwind)
├── cli/                   # Go CLI (shares chunker/crypto with server)
│   └── cmd/cass-cli/
├── shared/                # Shared Go: chunker, crypto, manifest, protocol
├── wasm/                  # WASM build of shared/ for in-browser restore
├── desktop/               # Tauri app (Windows target in v1)
│   ├── src-tauri/
│   └── src/               # Svelte or vanilla JS frontend
├── deploy/
│   ├── docker-compose.yml
│   ├── Dockerfile.server
│   └── systemd/           # SSD-mount triggers
├── docs/
│   ├── SPEC.md            # this document
│   ├── UPGRADE.md         # deferred work
│   ├── THREAT_MODEL.md
│   └── RUNBOOKS/
└── README.md
```

## 15. Decisions locked in for v1

1. Name: **cass**.
2. Client OS: **Windows only**.
3. Encryption model: **HMAC-keyed dedup**, no convergent-encryption leak.
4. Restore UX: **In-browser WASM** (max capability) + CLI for power users.
5. TLS: **TOFU self-signed cert with fingerprint pinning**.
6. Notifications: **Email via SMTP** (user-provided account + app password).
