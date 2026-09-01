# Nimbus

Personal cloud storage backed by **your Telegram account**. Always-on web/API server — no desktop app required after login.

Synthesizes the best of three open-source models:

| Source | Taken | Left behind |
|--------|--------|-------------|
| [Teldrive](https://github.com/tgdrive/teldrive) | Always-on server, staging uploads → commit, channel storage | God-service, JSONB parts, spoofed Telegram app IDs |
| [Telegram-Drive](https://github.com/caamer20/Telegram-Drive) | User-account MTProto login, channel-as-folder mindset | Desktop-only loopback API |
| [Pentaract](https://github.com/Dominux/Pentaract) | Layered API, chunk index, Docker shape | Bot-only blob store, whole-file-in-RAM |

## Architecture

```
Presentation (HTTP) → Application (use cases) → Domain
                              ↓
              Infrastructure (SQLite/Postgres ports, Telegram MTProto)
```

- **Metadata** lives in SQLite (default) or Postgres — files, folders, `file_parts`
- **Bytes** live in a Telegram channel owned by your account
- **BlobStore** is a port: Telegram today; swap later without rewriting the drive

## Requirements

- Go 1.22+
- Telegram `api_id` + `api_hash` from [my.telegram.org](https://my.telegram.org) (**your own** — never share/spoof)
- Node 20+ (UI only)

## Quick start

```bash
# 1) Server (JWT secret auto-creates; Telegram API optional in .env)
go run ./cmd/nimbus

# 2) UI
cd web && npm install && npm run dev
```

Open http://localhost:5173

1. Paste **api_id** / **api_hash** in the web form (from [my.telegram.org/apps](https://my.telegram.org/apps))
2. Sign in with your phone number
3. Upload files

You do **not** need to put Telegram credentials in `.env` anymore.

## API (v1)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/send-code` | Start phone login |
| POST | `/api/v1/auth/sign-in` | Submit code (and 2FA if needed) |
| GET | `/api/v1/auth/me` | Current user |
| POST | `/api/v1/auth/logout` | Revoke session |
| GET | `/api/v1/files?parent_id=` | List folder |
| POST | `/api/v1/folders` | Create folder |
| POST | `/api/v1/files/upload` | Upload file (multipart) |
| GET | `/api/v1/files/{id}/download` | Download file |
| DELETE | `/api/v1/files/{id}` | Delete file or folder |
| GET | `/health` | Health check |

## Config

See [`.env.example`](.env.example). Data directory defaults to `./data` (DB + Telegram session).

## Security notes

- Protect the server like a password vault: it holds your Telegram session (`data/telegram.session`)
- Every `/api/v1/*` call requires header `X-Nimbus-Access` matching `data/access.secret` (or `NIMBUS_ACCESS_SECRET`)
- Put TLS + network auth in front if exposed beyond localhost
- Respect [Telegram ToS](https://core.telegram.org/api/terms); this is for personal use

## License

MIT — not affiliated with Telegram, AWS, or Oracle.
