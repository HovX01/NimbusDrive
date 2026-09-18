# Nimbus API Reference (v1)

Base URL: `http://localhost:9090` (or your deployed host)

Nimbus is a self-hosted backend:

- **Storage API** — files on **Telegram** (upload, import, download, public URLs)
- **Data API** — app data in **SQLite** (collections, CRUD, filters)
- **Drive API** — folders, trash, search (web UI + API)

---

## Authentication

### API key (recommended for apps)

Use for Storage API, Data API, and file operations from your backend or demo apps.

```http
X-Nimbus-Key: <your-key>
```

Key location: `data/api.key` (auto-created on first run) or `NIMBUS_API_KEY` in `.env`.

Alternative:

```http
Authorization: Bearer <your-api-key>
```

### Access key (deployment gate)

When `NIMBUS_PUBLIC=false`, also send:

```http
X-Nimbus-Access: <access-secret>
```

From `data/access.secret` or `NIMBUS_ACCESS_SECRET`.

When `NIMBUS_PUBLIC=true`, this is not required.

### Telegram session (web UI)

The drive UI uses Telegram OTP login → JWT:

```http
Authorization: Bearer <jwt>
```

Obtain via `POST /api/v1/auth/sign-in` or `POST /api/v1/auth/resume`.

JWT is required for `/auth/me`, `/auth/logout`, `/settings/storage`.

---

## Errors

All errors return JSON:

```json
{
  "error": {
    "code": "validation_error",
    "message": "human-readable message"
  }
}
```

| HTTP | Code | Meaning |
|------|------|---------|
| 400 | `validation_error` | Bad input |
| 401 | `unauthorized` | Missing/invalid auth |
| 404 | `not_found` | Resource missing |
| 409 | `conflict` | Name conflict, etc. |
| 428 | `not_configured` | Telegram not set up |

---

## Storage API

Upload and serve files. Bytes are stored in your Telegram channel.

### Upload file

```http
POST /api/v1/files/upload
Content-Type: multipart/form-data
X-Nimbus-Key: <key>

file=<binary>
parent_id=root          (optional)
public=true             (optional; API key responses include url by default)
```

**Response `201`:**

```json
{
  "id": "uuid",
  "name": "photo.jpg",
  "mime_type": "image/jpeg",
  "size": 12345,
  "url": "http://localhost:9090/api/v1/share/TOKEN/download"
}
```

### Import from URL

Download any `https://` file and store it (Cloudinary-style fetch).

```http
POST /api/v1/files/import
Content-Type: application/json
X-Nimbus-Key: <key>

{
  "url": "https://example.com/image.jpg",
  "parent_id": "folder-uuid",
  "name": "optional.jpg",
  "public": true
}
```

### Download file

```http
GET /api/v1/files/{id}/download
X-Nimbus-Key: <key>
```

Supports `Range` header for partial content.

### Thumbnail

```http
GET /api/v1/files/{id}/thumb
X-Nimbus-Key: <key>
```

Returns JPEG for images.

### Public share (no auth)

```http
GET /api/v1/share/{token}/download
```

Use the `url` from upload/import responses in `<img src="...">`.

---

## Data API

Store and query JSON documents in named **collections** (like Supabase tables).

Collection names: lowercase letters, numbers, underscore; must start with a letter (`products`, `users`, `orders`).

### List collections

```http
GET /api/v1/data/collections
X-Nimbus-Key: <key>
```

### Query rows

```http
GET /api/v1/data/{collection}?price=gte.10&name=like.*shirt*&order=price.desc&limit=20&offset=0
X-Nimbus-Key: <key>
```

**Query parameters:**

| Param | Example | Meaning |
|-------|---------|---------|
| `{field}=eq.{value}` | `status=eq.active` | Equals |
| `{field}=gt.{value}` | `price=gt.10` | Greater than |
| `{field}=gte.{value}` | `price=gte.10` | Greater or equal |
| `{field}=lt.{value}` | `price=lt.100` | Less than |
| `{field}=lte.{value}` | `price=lte.100` | Less or equal |
| `{field}=like.{pattern}` | `name=like.*shirt*` | LIKE (`*` → `%`) |
| `order` | `order=created_at.desc` | Sort |
| `limit` | `limit=50` | Max rows (default 50, max 100) |
| `offset` | `offset=0` | Pagination |

System fields: `id`, `created_at`, `updated_at`.

**Response:**

```json
{
  "items": [
    {
      "id": "uuid",
      "collection": "products",
      "name": "Blue Shirt",
      "price": 29.99,
      "image_file_id": "file-uuid",
      "image_url": "http://.../share/.../download",
      "created_at": "2026-09-01T12:00:00Z",
      "updated_at": "2026-09-01T12:00:00Z"
    }
  ]
}
```

### Insert row

```http
POST /api/v1/data/{collection}
Content-Type: application/json
X-Nimbus-Key: <key>

{
  "name": "Blue Shirt",
  "price": 29.99,
  "image_file_id": "file-uuid-from-upload"
}
```

### Get one row

```http
GET /api/v1/data/{collection}/{id}
X-Nimbus-Key: <key>
```

### Update row

```http
PATCH /api/v1/data/{collection}/{id}
Content-Type: application/json
X-Nimbus-Key: <key>

{
  "price": 39.99
}
```

Merged into existing JSON fields.

### Delete row

```http
DELETE /api/v1/data/{collection}/{id}
X-Nimbus-Key: <key>
```

---

## Drive API (files & folders)

### List folder

```http
GET /api/v1/files?parent_id=root
X-Nimbus-Key: <key>
```

### Create folder

```http
POST /api/v1/folders
Content-Type: application/json

{ "parent_id": "root", "name": "my-folder" }
```

### Rename

```http
PATCH /api/v1/files/{id}
Content-Type: application/json

{ "name": "new-name.jpg" }
```

### Move

```http
POST /api/v1/files/{id}/move
{ "parent_id": "folder-uuid" }
```

### Delete (trash)

```http
DELETE /api/v1/files/{id}
```

### Search files

```http
GET /api/v1/search?q=photo
```

### Fetch social/video URL (async)

```http
POST /api/v1/files/fetch
{
  "url": "https://youtube.com/watch?v=...",
  "parent_id": "root",
  "mode": "video",
  "max_height": 480
}
```

Poll: `GET /api/v1/files/fetch/{job_id}`

### Share links

```http
POST /api/v1/files/{id}/share
GET  /api/v1/files/{id}/shares
DELETE /api/v1/shares/{shareId}
```

### Trash

```http
GET    /api/v1/trash
DELETE /api/v1/trash/{id}    (permanent purge)
DELETE /api/v1/trash         (empty trash)
```

---

## Auth & setup (Telegram)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/setup/status` | Public — configured, authorized |
| POST | `/api/v1/setup/telegram` | Set api_id / api_hash |
| POST | `/api/v1/auth/send-code` | Start phone login |
| POST | `/api/v1/auth/sign-in` | Submit OTP code |
| POST | `/api/v1/auth/resume` | Session → JWT |
| GET | `/api/v1/auth/me` | Current user (JWT) |
| POST | `/api/v1/auth/logout` | Logout (JWT) |
| GET | `/api/v1/settings/storage` | API key + base URL (JWT) |

---

## Health

```http
GET /health
```

```json
{ "status": "ok", "service": "nimbus" }
```

---

## Full example: ecommerce product

```bash
API=http://localhost:9090
KEY=$(cat data/api.key)

# 1. Upload image → Telegram
UPLOAD=$(curl -s -X POST "$API/api/v1/files/upload" \
  -H "X-Nimbus-Key: $KEY" -F "file=@shirt.jpg")
FILE_ID=$(echo "$UPLOAD" | jq -r .id)
IMAGE_URL=$(echo "$UPLOAD" | jq -r .url)

# 2. Insert product row → SQLite
curl -s -X POST "$API/api/v1/data/products" \
  -H "X-Nimbus-Key: $KEY" -H "Content-Type: application/json" \
  -d "{\"name\":\"Blue Shirt\",\"price\":29.99,\"image_file_id\":\"$FILE_ID\",\"image_url\":\"$IMAGE_URL\"}"

# 3. Query products
curl -s "$API/api/v1/data/products?order=created_at.desc" \
  -H "X-Nimbus-Key: $KEY"
```

---

## Demo app

Interactive examples: [`testAPI/`](../testAPI/) — run `cd testAPI && npm run dev` → http://localhost:5199

Browser docs: open [`docs/index.html`](index.html)
