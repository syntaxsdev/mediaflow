# Deployment

## Docker

### Pre-built image

```bash
docker run -p 8080:8080 \
  -e S3_BUCKET=your-bucket-name \
  -e AWS_ACCESS_KEY_ID=your-key \
  -e AWS_SECRET_ACCESS_KEY=your-secret \
  -v $(pwd)/storage-config.yaml:/app/storage-config.yaml \
  syntaxsdev/mediaflow:latest
```

### Build from source

```bash
make build-image
```

The runtime image ships with `ffmpeg` (for `ffprobe`) plus `libwebp` and `vips` for image processing. ARM64 builds fall back gracefully if `libwebp`/`vips` aren't available.

## ECS

1. Create an IAM role with S3 permissions (see below).
2. Create an ECS task definition using `syntaxsdev/mediaflow:latest`.
3. Attach the IAM role to the task.
4. Set env vars: `S3_BUCKET`, `S3_REGION`, `PORT`, plus `STREAM_ACCOUNT_ID` / `STREAM_API_TOKEN` if any profile uses Stream delivery.
5. Mount `storage-config.yaml` into the container, or set `STORAGE_CONFIG_PATH=s3://bucket/path/storage-config.yaml` to load from S3.
6. Configure your ALB to forward to the ECS service.

### EC2

Same as ECS but attach the IAM role directly to the instance. mediaflow will pick up instance role credentials automatically.

## AWS S3 IAM policy

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::your-bucket-name",
        "arn:aws:s3:::your-bucket-name/*"
      ]
    }
  ]
}
```

When using IAM role auth (ECS or EC2), omit `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` from env — the SDK picks up credentials from instance metadata automatically.

## R2 / non-AWS S3

Set `S3_ENDPOINT` to your provider's endpoint:

```bash
S3_ENDPOINT=https://<account>.r2.cloudflarestorage.com
PUBLIC_S3_ENDPOINT=https://media.your-domain.com   # what clients see in presigned URLs
S3_REGION=auto
```

`PUBLIC_S3_ENDPOINT` is the host stamped into presigned URLs the client receives — set it to your custom CDN domain if you want clients to upload to `media.example.com` rather than the raw R2 endpoint. mediaflow itself uses `S3_ENDPOINT` for internal calls.

## R2 bucket lifecycle rules

For R2-delivered profiles, add these in the Cloudflare dashboard (R2 → bucket → Settings → Object lifecycle rules):

- **`abort-stale-multipart`** — action: *Abort incomplete multipart uploads* after 1 day. Reaps abandoned multipart sessions that never called `/complete`.

For "completed object, never attached" orphans, the right defense is application-side cleanup, not a bucket lifecycle rule (which can't distinguish attached from unattached). Implement a periodic job in your orchestrator that walks unattached records past a TTL and calls `DELETE /v1/assets/...` for each.

Stream-delivered profiles don't need R2 lifecycle rules — bytes never land in R2.

## CDN

For R2-delivered images, put a CDN (CloudFront, Cloudflare) in front of the `/originals/` and `/thumb/` paths. mediaflow sets `Cache-Control` and `ETag` headers appropriately.

For Stream-delivered videos, Stream's edge handles caching — no CDN setup needed.
