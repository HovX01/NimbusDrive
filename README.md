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

## API documentation

Full reference: **[docs/API.md](docs/API.md)** · Browser: open **[docs/index.html](docs/index.html)**

## API (v1) quick reference

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/send-code` | Start phone login |
| POST | `/api/v1/auth/sign-in` | Submit code (and 2FA if needed) |
| GET | `/api/v1/auth/me` | Current user |
| POST | `/api/v1/auth/logout` | Revoke session |
| GET | `/api/v1/files?parent_id=` | List folder |
| POST | `/api/v1/folders` | Create folder |
| POST | `/api/v1/files/upload` | Upload file (multipart) |
| POST | `/api/v1/files/import` | Import file from remote URL |
| GET | `/api/v1/files/{id}/download` | Download file |
| GET | `/api/v1/data/{collection}` | Query rows (filters) |
| POST | `/api/v1/data/{collection}` | Insert row |
| PATCH | `/api/v1/data/{collection}/{id}` | Update row |
| DELETE | `/api/v1/data/{collection}/{id}` | Delete row |
| GET | `/api/v1/data/collections` | List collection names |
| DELETE | `/api/v1/files/{id}` | Move file/folder to trash |
| GET | `/health` | Health check |

### Storage API (for your apps)

Use `X-Nimbus-Key` (see `data/api.key`) for programmatic access. See [docs/API.md](docs/API.md) for examples.

### Media fetch engine

Nimbus keeps its existing native extractors and configured API fallbacks. When those cannot resolve a link, it can fall back to local `yt-dlp` for broad site support and high-quality video/audio merging via `ffmpeg`. The Docker image includes both `yt-dlp` and `ffmpeg`; bare-metal installs can set `NIMBUS_YTDLP_PATH`.

## Config

See [`.env.example`](.env.example). Data directory defaults to `./data` (DB + Telegram session).

### TikTok / Instagram / Facebook connections

Settings includes account switching for TikTok, Instagram, and Facebook. Without a developer account, export a Netscape-format `cookies.txt` from a browser where you are logged in, then use **Connect browser session** in Settings. Nimbus stores that session on the server and uses the active account for media fetches.

Official provider-app OAuth is only an advanced optional path for deployments that own TikTok/Meta developer apps. Create developer apps with each provider, add the callback URLs below, then save setup in Settings or set the matching environment variables from [`.env.example`](.env.example):

```text
https://your-domain.example/api/v1/social/oauth/tiktok/callback
https://your-domain.example/api/v1/social/oauth/instagram/callback
https://your-domain.example/api/v1/social/oauth/facebook/callback
```

OAuth providers generally require a public HTTPS callback. If Nimbus is only at `http://192.168.100.49:9090` on your LAN, put it behind a reverse proxy/tunnel with TLS and set `NIMBUS_PUBLIC_BASE_URL` to that public URL.

You can connect multiple accounts for the same provider. Use **Connect browser session** or **Add account** to add another login, then **Switch** in Settings to choose the active account Nimbus uses for provider-aware fetches.

## Security notes

- Protect the server like a password vault: it holds your Telegram session (`data/telegram.session`)
- OAuth access/refresh tokens for connected media accounts are stored in the local SQLite database.
- Every `/api/v1/*` call requires header `X-Nimbus-Access` matching `data/access.secret` (or `NIMBUS_ACCESS_SECRET`)
- Put TLS + network auth in front if exposed beyond localhost
- Respect [Telegram ToS](https://core.telegram.org/api/terms); this is for personal use

## License

MIT — not affiliated with Telegram, AWS, or Oracle.
