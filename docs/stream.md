# Cloudflare Stream Delivery

Profiles with `delivery: stream` route uploads directly to Cloudflare Stream. R2 is bypassed entirely for those profiles — no double storage, no transcoding pipeline to operate, native HEVC and MOV support.

## Why Stream

For video profiles, the alternatives are:

| Approach | Trade |
|---|---|
| **R2 + ffprobe + DIY transcode** | Full control, but you operate ffmpeg, store proxies, and handle codec rescue (HEVC, MOV) yourself. |
| **R2 + Cloudflare Media Transformations** | JIT transformation at the edge, but inputs must be H.264 mp4. Anything else needs server-side rescue. |
| **Cloudflare Stream** (this) | One-time transcode at ingest, free encoding, native support for any codec/container clients produce. ~$5/1000 min stored, ~$1/1000 min delivered. |

For short-form creator content (trailers, clips), Stream is the lowest-engineering path that handles iPhone HEVC/MOV uploads without any DIY transcoding.

## Flow

```
client → orchestrator
         └─ POST /v1/uploads/presign { profile: "trailer", ... }
            mediaflow → Stream: POST /accounts/{id}/stream/direct_upload
                                { maxDurationSeconds: <profile.max_duration_seconds> }
            mediaflow ← Stream: { uploadURL, uid }
         ← { object_key: <uid>, upload: { stream: { url, uid, method, expires_at } } }

client → Stream: direct upload to uploadURL (POST ≤200MB, TUS otherwise)

orchestrator → mediaflow: POST /v1/assets/trailer/{uid}/probe
               mediaflow → Stream: GET /stream/{uid}  (read duration/dimensions)
               returns 202 if still encoding, 200 with {ok, reasons[]} once ready

orchestrator → mediaflow: DELETE /v1/assets/trailer/{uid}  (if probe failed or asset removed)
               mediaflow → Stream: DELETE /stream/{uid}
```

## Caps & gates

| Constraint | Where it's enforced |
|---|---|
| `size_max_bytes` | mediaflow at presign time, before calling Stream. |
| `max_duration_seconds` | Passed to Stream as `maxDurationSeconds`; Stream rejects longer uploads at ingest. |
| `min_width` / `min_height` (and others) | Validated post-ingest by the probe endpoint — dimensions aren't known until Stream finishes encoding. |

`allowed_codecs` is **ignored** for Stream profiles. Stream re-encodes everything to H.264 for delivery, so input codec is irrelevant.

## Probe states

The probe endpoint returns:

- **`200` with `ok: true, ready: true`** — video is ready and meets all profile constraints.
- **`200` with `ok: false, ready: true, reasons: [...]`** — video is ready but violates a constraint. Caller should `DELETE` the asset and surface a 422 to the client.
- **`202` with `ok: false, ready: false, state: "queued"|"inprogress"`** — Stream is still encoding. Caller should retry, or use a [Stream webhook](https://developers.cloudflare.com/stream/manage-video-library/using-webhooks/) to get notified.
- **`404`** — UID not found in Stream.
- **`502`** — Stream API error.

## Serving

mediaflow does not serve Stream-delivered videos. Use Stream's URLs directly from your frontend:

| URL | Purpose |
|---|---|
| `https://customer-{code}.cloudflarestream.com/{uid}/iframe` | Stream's bundled player (iframe embed). |
| `https://customer-{code}.cloudflarestream.com/{uid}/manifest/video.m3u8` | HLS manifest — feed to your own player (hls.js, AVPlayer, ExoPlayer). |
| `https://customer-{code}.cloudflarestream.com/{uid}/thumbnails/thumbnail.jpg?time=2s` | Auto-generated poster at any timestamp. |

Custom domains for VOD playback are not supported by Stream. Use your own player against the HLS manifest if branding the URL bar matters; end-users don't typically see CDN hostnames in playback flows.

## Required environment

```bash
STREAM_ACCOUNT_ID=     # Cloudflare account ID
STREAM_API_TOKEN=      # API token with Stream:Edit permission
```

mediaflow only reads these when a profile sets `delivery: stream`. Other profiles work without them.
