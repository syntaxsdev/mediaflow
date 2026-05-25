# API Reference

All write endpoints (`POST` / `PUT` / `PATCH` / `DELETE`) require the `API_KEY` env var when set; pass it as `Authorization: Bearer <key>`.

## Presigned Uploads

```
POST /v1/uploads/presign
```

Generates a presigned URL for direct upload — to S3/R2 by default, or directly to Cloudflare Stream when the profile has `delivery: stream`.

**Request body:**
```json
{
  "key_base": "unique-file-id",
  "ext": "jpg",
  "mime": "image/jpeg",
  "size_bytes": 1024000,
  "kind": "image",
  "profile": "avatar",
  "multipart": "auto"
}
```

| Field | Notes |
|---|---|
| `key_base` | Unique identifier. Ignored for `delivery: stream` profiles — Stream assigns the UID. |
| `ext` | File extension (used in `storage_path` template). |
| `mime` | Validated against profile `allowed_mimes`. |
| `size_bytes` | Validated against profile `size_max_bytes`. For Stream, also chooses POST (≤200MB) vs TUS. |
| `kind` | `image`, `video`, or `file`. Must match the profile's `kind`. |
| `profile` | Name from `storage-config.yaml`. |
| `multipart` | `auto` / `force` / `off`. Ignored for Stream delivery. |

**Response — single PUT (R2):**
```json
{
  "object_key": "originals/avatars/ab/unique-file-id.jpg",
  "upload": {
    "single": {
      "method": "PUT",
      "url": "https://presigned-s3-url",
      "headers": { "Content-Type": "image/jpeg", "If-None-Match": "*" },
      "expires_at": "2024-01-01T12:00:00Z"
    }
  }
}
```

**Response — multipart (R2):**
```json
{
  "object_key": "originals/avatars/ab/unique-file-id.jpg",
  "upload": {
    "multipart": {
      "upload_id": "abc123xyz",
      "part_size": 8388608,
      "parts": [
        {
          "part_number": 1,
          "method": "PUT",
          "url": "https://presigned-s3-part-url-1",
          "headers": { "Content-Type": "image/jpeg" },
          "expires_at": "2024-01-01T12:00:00Z"
        }
      ],
      "complete": {
        "method": "POST",
        "url": "https://your-api/v1/uploads/.../complete/abc123xyz",
        "headers": { "Content-Type": "application/json" },
        "expires_at": "2024-01-01T12:00:00Z"
      },
      "abort": {
        "method": "DELETE",
        "url": "https://your-api/v1/uploads/.../abort/abc123xyz",
        "headers": {},
        "expires_at": "2024-01-01T12:00:00Z"
      }
    }
  }
}
```

**Response — Cloudflare Stream:**
```json
{
  "object_key": "8f2a3c4d5e6f7890abcdef1234567890",
  "upload": {
    "stream": {
      "method": "POST",
      "url": "https://upload.videodelivery.net/...",
      "uid": "8f2a3c4d5e6f7890abcdef1234567890",
      "expires_at": "2024-01-01T12:00:00Z"
    }
  }
}
```

For `delivery: stream`, `object_key` is the Stream video UID — use it as the asset's stable handle for probe + delete. `method` is `"POST"` for ≤200MB, `"TUS"` for larger.

## Multipart Upload Completion

```
POST /v1/uploads/{object_key}/complete/{upload_id}
```

```json
{
  "parts": [
    { "part_number": 1, "etag": "\"d41d8cd98f00b204e9800998ecf8427e\"" },
    { "part_number": 2, "etag": "\"098f6bcd4621d373cade4e832627b4f6\"" }
  ]
}
```

Response: `{ "status": "completed", "object_key": "..." }`

## Multipart Upload Abort

```
DELETE /v1/uploads/{object_key}/abort/{upload_id}
```

Response: `{ "status": "aborted", "upload_id": "..." }`

## Probe Asset (Video)

```
POST /v1/assets/{profile}/{key_base}/probe
```

Validates an uploaded video against the profile's constraint fields. Required: profile must be `kind: video`. Returns 200 for both pass and fail — `ok` is the gate, not the status code.

- **R2 profiles** — mediaflow runs `ffprobe` over a presigned GET URL.
- **Stream profiles** — `key_base` is the Stream UID; mediaflow reads metadata from `GET /accounts/{id}/stream/{uid}`. Returns `202` with `{ok: false, ready: false, state: "queued"|"inprogress"}` if Stream is still encoding.

**Pass:**
```json
{
  "ok": true,
  "ready": true,
  "video": { "duration_seconds": 42.18, "width": 1920, "height": 1080, "codec": "h264" },
  "reasons": []
}
```

**Fail:**
```json
{
  "ok": false,
  "video": { "duration_seconds": 67.4, "width": 854, "height": 480 },
  "reasons": [
    { "code": "duration_exceeded", "limit": 45, "actual": 67.4 },
    { "code": "width_too_low",     "limit": 1280, "actual": 854 }
  ]
}
```

**Reason codes:** `duration_exceeded`, `width_too_low`, `height_too_low`, `width_too_high`, `height_too_high`, `codec_not_allowed`, `no_video_stream`.

**Other status codes:** `404` (asset/UID not found), `422` (profile is not `kind: video`), `502` (ffprobe crash or Stream API error).

## Delete Asset

```
DELETE /v1/assets/{profile}/{key_base}
```

R2 profiles: deletes the original + every object under `thumb_folder/{key_base}*`.
Stream profiles: `key_base` is the UID; mediaflow calls Stream's `DELETE /stream/{uid}`.

**R2 response:**
```json
{ "status": "deleted", "profile": "avatar", "key_base": "abc123", "objects_deleted": 4 }
```

**Stream response:**
```json
{ "status": "deleted", "profile": "trailer", "uid": "8f2a3c4d5e6f7890..." }
```

## Thumbnails

```
GET  /thumb/{type}/{image_id}?width=512
POST /thumb/{type}/{image_id}
```

Generates and serves thumbnails. POST requires `API_KEY` auth.

**GET parameters:**
- `type`: profile name (avatar, photo, banner, …)
- `image_id`: unique identifier
- `width`: pixels — defaults to the profile's `default_size`

## Original Images

```
GET /originals/{type}/{image_id}
```

Serves the original from storage.

## Download URL (Private Files)

```
GET /v1/downloads/presign?profile={profile}&key_base={key_base}&filename={filename}
```

Returns a short-lived presigned **GET** URL for a private `kind: file` asset. Unlike `/thumb` and `/originals` (which serve public reads), this endpoint **requires `API_KEY`** even though it is a GET — it gates private files, so the calling service is expected to authorize the request before asking for a URL. The `file`-kind profile should live in a bucket with no public domain (see [Multiple buckets](configuration.md#multiple-buckets)).

**Query parameters:**

| Param | Notes |
|---|---|
| `profile` | Must be a `kind: file` profile (else `400`). |
| `key_base` | Asset identifier, resolved through the profile's `storage_path`. |
| `filename` | *Optional.* Sets `Content-Disposition` so the browser saves the file under this name. |

**Response:**
```json
{
  "object_key": "downloads/unique-file-id",
  "url": "https://presigned-s3-url?...&response-content-disposition=attachment%3B%20filename%3D...",
  "expires_at": "2024-01-01T12:15:00Z"
}
```

The client then issues a plain `GET` to `url` to download the bytes; the URL is valid for the profile's `token_ttl_seconds`.

**Status codes:** `400` (missing params or non-`file` profile), `404` (object not found), `405` (non-GET method).

## Health Check

```
GET /health
```

Returns `OK`.
