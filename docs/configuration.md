# Configuration

## storage-config.yaml

Profile-based config combining upload, processing, probe, and delivery rules. Loaded from the path in `STORAGE_CONFIG_PATH` (defaults to `examples/storage-config.yaml`). Can also be loaded from S3 with an `s3://bucket/key` path.

```yaml
profiles:
  avatar:
    kind: "image"
    allowed_mimes: ["image/jpeg", "image/png", "image/webp"]
    size_max_bytes: 5242880  # 5MB
    multipart_threshold_mb: 15
    part_size_mb: 8
    token_ttl_seconds: 900
    storage_path: "originals/avatars/{shard?}/{key_base}"
    enable_sharding: true
    thumb_folder: "thumbnails/avatars"
    sizes: ["128", "256"]
    default_size: "256"
    quality: 90
    convert_to: "webp"

  video:
    # R2-delivered video — mediaflow runs ffprobe at attach
    kind: "video"
    allowed_mimes: ["video/mp4", "video/quicktime", "video/webm"]
    size_max_bytes: 104857600  # 100MB
    multipart_threshold_mb: 15
    part_size_mb: 8
    token_ttl_seconds: 1800
    storage_path: "originals/videos/{shard?}/{key_base}"
    enable_sharding: true
    max_duration_seconds: 600
    min_width: 640
    min_height: 360
    allowed_codecs: ["h264", "hevc"]

  trailer:
    # Stream-delivered video — bytes go directly to Cloudflare Stream
    kind: "video"
    delivery: "stream"
    allowed_mimes: ["video/mp4", "video/quicktime"]
    size_max_bytes: 78643200  # 75MB; Stream caps plain POST at 200MB, TUS at 30GB
    token_ttl_seconds: 1800
    max_duration_seconds: 45
    min_width: 1280
    min_height: 720
```

## Field reference

### Upload

| Field | Notes |
|---|---|
| `kind` | `image` or `video`. |
| `delivery` | `""` / `"r2"` (default — presigned R2 PUT) or `"stream"` (Cloudflare Stream Direct Creator Upload). Stream requires `STREAM_ACCOUNT_ID` + `STREAM_API_TOKEN`. |
| `allowed_mimes` | Whitelist of MIME types accepted at presign. |
| `size_max_bytes` | Hard cap on upload size, enforced at presign. |
| `multipart_threshold_mb` | Files above this trigger multipart (R2 only). |
| `part_size_mb` | Multipart chunk size (R2 only). |
| `token_ttl_seconds` | Presigned URL expiration. |
| `storage_path` | Template for R2 key. Required for R2 profiles, omitted for `delivery: stream`. Supports `{key_base}`, `{ext}`, `{shard}`, `{shard?}`. |
| `enable_sharding` | When true, mediaflow generates a 2-char shard from `key_base` hash. |

### Image processing

| Field | Notes |
|---|---|
| `thumb_folder` | Where thumbnails are written. |
| `sizes` | Available widths (e.g. `["128", "256"]`). |
| `default_size` | Width used when client doesn't specify. |
| `quality` | 1–100. |
| `convert_to` | Output format (`webp`, `jpeg`, etc.). |

### Video probe constraints

All optional. Unset fields are skipped during probe; only set fields are enforced.

| Field | Notes |
|---|---|
| `max_duration_seconds` | Reject longer videos. For `delivery: stream`, also passed to Stream's `direct_upload` API as `maxDurationSeconds`. |
| `min_width` / `min_height` | Resolution floors. |
| `max_width` / `max_height` | Resolution ceilings. |
| `allowed_codecs` | Whitelist (e.g. `["h264", "hevc"]`). Stream profiles ignore this — Stream re-encodes to H.264 regardless. |

## Storage path templates

`storage_path` is a string template:

| Placeholder | Meaning |
|---|---|
| `{key_base}` | Unique file identifier from the request. |
| `{ext}` | File extension. |
| `{shard}` | Always replaced. Use when `enable_sharding: true`. |
| `{shard?}` | Removed (along with surrounding `/`) when `enable_sharding: false`. |

**Auto-sharding** (`enable_sharding: true`):
- `"originals/{shard?}/{key_base}"` → `originals/ab/my-file`
- Shards auto-generated from a SHA1 prefix of `key_base`.
- Clients can optionally pass `shard` in the request to override.

**Fixed organization** (`enable_sharding: false`):
- `"originals/user123/{key_base}"` → `originals/user123/my-file`
- `{shard?}` placeholders are stripped.
- `shard` field in requests is ignored.

## Environment variables

```bash
# Required
S3_BUCKET=your-bucket-name

# AWS / R2 credentials — pick one
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key
# or use IAM role (ECS/EC2)

# Optional
S3_REGION=us-east-1
S3_ENDPOINT=                           # Override for non-AWS S3 (R2, MinIO, etc.)
PUBLIC_S3_ENDPOINT=                    # Host used in presigned URLs (defaults to S3_ENDPOINT)
PORT=8080
CACHE_MAX_AGE=86400
STORAGE_CONFIG_PATH=storage-config.yaml  # Or s3://bucket/path/storage-config.yaml

# API authentication (write endpoints reject if unset on a write)
API_KEY=

# Cloudflare Stream — required if any profile uses delivery: stream
STREAM_ACCOUNT_ID=
STREAM_API_TOKEN=                      # Token with Stream:Edit on the account
```
