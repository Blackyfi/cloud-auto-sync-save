# `cass` — Deferred Work / Future Upgrades

Items intentionally **out of scope for v1** (see [`SPEC.md`](./SPEC.md)). Listed here so they aren't forgotten and so v1 design choices stay forward-compatible.

---

## U1. macOS desktop client

**Status:** likely needed in the medium term.
**Scope:** build & ship `cass-agent` for macOS (Apple Silicon + Intel).
**Effort:** small — Tauri already supports macOS; mainly:
- Add `aarch64-apple-darwin` and `x86_64-apple-darwin` build targets.
- Replace Windows Credential Manager calls with **macOS Keychain**.
- Replace Windows Task Scheduler integration with **launchd** for scheduled backups.
- Code-sign + notarize the `.app` bundle (Apple Developer account required).
- Universal binary (`lipo`) for distribution.

**Forward-compat note in v1:** keep all OS-specific code (credential storage, scheduler) behind a small interface so adding macOS is a new implementation, not a rewrite.

---

## U2. Linux desktop client

**Status:** lower priority than macOS.
**Scope:** build & ship `cass-agent` for major Linux distros.
**Effort:** small — Tauri supports Linux; mainly:
- AppImage and `.deb` / `.rpm` packaging.
- Replace credential store with **Secret Service API** (libsecret / GNOME Keyring / KWallet).
- Replace scheduler with **systemd user timers**.

---

## U3. Proper CA / certificate management

**v1:** TOFU self-signed cert, fingerprint pinned by clients on first connect.

**Upgrade options (in increasing complexity):**
1. **`mkcert`** for an internal CA — generate a local root, install on each device, issue server cert from it. Simple, no internet dependency. Good for LAN-only.
2. **`step-ca`** (Smallstep) — full internal ACME-capable CA, automated cert rotation, supports SSH certs too. Right answer once there are 3+ devices.
3. **Let's Encrypt (DNS-01 challenge)** — issues publicly-trusted certs for an internal hostname using a DNS provider's API. No port forwarding needed. Right answer once the server is reachable from outside (see U4).

**Migration from TOFU:**
- Admin UI: "rotate cert" flow — uploads new cert, computes new fingerprint, **broadcasts notice to all clients** (via email + a banner in `cass-agent`).
- Each client prompts the user to confirm the new fingerprint on next connect, then re-pins.

---

## U4. Off-LAN access (VPN / internet)

**v1:** LAN-only. Server bound to internal network; agent only attempts backup when the laptop is on the home LAN.

**Upgrade options:**
1. **Tailscale** (recommended) — zero-config WireGuard mesh. Server and laptop join the same tailnet. No port forwarding, no public DNS, MagicDNS gives you a stable hostname. Works behind any NAT/firewall. Effort: ~1 evening.
2. **Self-hosted WireGuard** — more control, requires a public IP or a relay, and manual key management.
3. **Public HTTPS endpoint** — reverse proxy (Caddy / Traefik) + Let's Encrypt (U3 option 3) + port-forward 443. Highest blast radius if misconfigured; least recommended.

**Forward-compat note in v1:** the server's HTTP API is identical regardless of transport. The agent stores a list of "endpoints to try" — adding a Tailscale URL is just a new entry.

**Bandwidth considerations for off-LAN:**
- Throttling (R9 in SPEC) becomes mandatory, not optional.
- Add a "metered network" detector on the client to skip backup on cellular/hotspot.
- Consider a separate retention policy for off-LAN snapshots if upload windows are short.

---

## U5. Mobile clients (iOS / Android)

**Status:** speculative. Only worth doing if backing up phone photos becomes a goal.

**Scope:** read-only restore first (browse + download a file), then opt-in selective backup of camera roll / specific app folders.

**Tech:** **Capacitor** or **Flutter** wrapping the same WASM crypto bundle would let us reuse the chunker/crypto code. Native (Swift / Kotlin) would mean reimplementing crypto — avoid.

---

## U6. Multi-user / shared folders

**v1:** single user (you), multiple devices fine.

**Upgrade scope:** multiple users, each with their own repo + independent retention. Optional: shared folders (e.g. household photos) accessible by multiple users.

**Hard part:** shared folders + client-side encryption is non-trivial (key sharing, key rotation when someone leaves, audit). Don't tackle until there's a real need; see how Cryptomator / Tresorit handle it for prior art.

---

## U7. Notification channels beyond email

**v1:** email only.

**Add later (in roughly this order of value):**
1. **ntfy** — self-hostable push notifications to phone. Cheap to add (HTTP POST). Best for "backup overdue" wake-ups.
2. **Discord webhook** — handy if you already have a personal Discord server.
3. **Slack webhook** — same idea.
4. **Generic webhook** — lets users wire it into anything (Home Assistant, n8n, etc).

The notifier in v1 should already be behind a small interface so adding channels is additive, not a refactor.

---

## U8. Snapshot search

**Scope:** "find all snapshots containing `tax_2025.pdf`" or full-text search across documents.

**Filename search** is easy: index manifest entries in SQLite (FTS5).

**Full-text** is harder because content is encrypted. Options:
- Index on the client during backup (encrypted index uploaded alongside the manifest).
- Index in the WASM restore UI on demand (slow but no extra storage).

Defer until there's a real "I can't find a file" pain point.

---

## U9. Bare-metal / system-image backup

**v1:** file-level backup of selected folders only.

**Upgrade scope:** whole-disk image backup of the laptop (for fast bare-metal recovery after a dead drive). This is a different problem class — likely solved by a dedicated tool (Veeam Agent, Macrium Reflect, or `wbadmin` on Windows), not by extending `cass`. Document the recommended companion tool rather than building it.

---

## U10. Storage backends beyond local disk

**v1:** server stores the repo on a local filesystem; SSDs are filesystem mirrors.

**Add later:**
- **S3-compatible** (MinIO, Backblaze B2, Wasabi, AWS S3) — for true offsite copy.
- **rclone-compatible remotes** as a generic backend.

Keep the storage layer behind an interface (`type Backend interface { Get/Put/Has/Delete/List }`) in v1 so adding new backends is a new file, not a refactor.

---

## U11. Hardware-token-protected repo password

**Scope:** unlock the repo with a YubiKey (FIDO2 hmac-secret) instead of (or in addition to) a typed password. Eliminates the "what if my password manager is gone" risk.

**Forward-compat note in v1:** key-wrapping format should support multiple wrapped copies of `repo_key` / `dedup_key` (one per unlock method). Then "add YubiKey" is just adding a new wrapped copy.

---

## U12. Snapshot signing / tamper-evidence

**Scope:** sign each snapshot manifest with a per-device signing key so the admin can verify a snapshot was created by a legitimate device, not injected by a compromised server.

**Why deferred:** the append-only API + admin audit log already cover the realistic threat model for a single household. Worth adding once there are multiple users / less trusted operators.

---

## U13. Better dedup window across very large libraries

**v1:** FastCDC with avg 1 MiB chunks is a good general default.

**If repo grows past ~10 TB:** consider:
- Sub-file similarity hashing (e.g. ssdeep) for near-duplicate detection on top of exact-chunk dedup.
- Tunable chunk size per directory (smaller chunks for code, larger for video).

Profile first; don't tune blind.
