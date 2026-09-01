# Nimbus architecture

Clean layered design (see `.cursor` rules).

```
cmd/nimbus          → process wiring
internal/http       → presentation (chi routes, DTOs, status codes)
internal/app        → application use cases
internal/domain     → entities, ports, pure algorithms
internal/store/*    → metadata persistence
internal/telegram   → BlobStore + TelegramAuth (gotd MTProto)
web/                → React UI
```

## Design choices taken from the three references

1. **Teldrive**: always-on server, staging upload → part index → ready status, channel-backed bytes
2. **Telegram-Drive**: user-account MTProto session (not bots), single long-lived client runner
3. **Pentaract**: router → service → repository layering, explicit chunk table

## Explicitly avoided

- Bot-only storage (Pentaract) for the personal-account product
- JSONB part blobs and PL/pgSQL business logic (Teldrive)
- Loopback-only desktop API as the only access path (Telegram-Drive)
- Spoofed Telegram desktop `api_id`/`api_hash`

## Ports

- `BlobStore` — swap Telegram later without rewriting drive logic
- `NodeRepository` / `PartRepository` — SQLite now; Postgres adapter can implement the same interfaces
