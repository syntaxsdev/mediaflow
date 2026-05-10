# MediaFlow

A lightweight Go service that handles media uploads, validation, and serving. Backed by S3/R2 by default; can route video uploads directly to Cloudflare Stream.

## What it does

- **Presigned uploads** — secure direct-to-S3 (or direct-to-Stream) uploads with profile-driven validation
- **Image processing** — on-demand thumbnails with WebP/JPEG/PNG output
- **Video probe** — validates duration, dimensions, and codec against profile constraints (ffprobe for R2, Stream API for Stream)
- **Cloudflare Stream delivery** — opt-in per profile, bytes go straight to Stream with no R2 round-trip

## Quick start

```bash
git clone https://github.com/syntaxsdev/mediaflow
cd mediaflow
cp .env.example .env  # set S3_BUCKET + AWS / R2 credentials
go mod download
make run            # or `make run-air` for hot reload
```

Or via Docker:

```bash
docker run -p 8080:8080 \
  -e S3_BUCKET=your-bucket-name \
  -e AWS_ACCESS_KEY_ID=your-key \
  -e AWS_SECRET_ACCESS_KEY=your-secret \
  -v $(pwd)/storage-config.yaml:/app/storage-config.yaml \
  syntaxsdev/mediaflow:latest
```

## Documentation

- [API reference](docs/api.md) — endpoints, request/response shapes, error codes
- [Configuration](docs/configuration.md) — `storage-config.yaml` profiles, fields, env vars
- [Cloudflare Stream delivery](docs/stream.md) — opt-in flow for video profiles
- [Deployment](docs/deployment.md) — Docker, ECS, IAM, R2, lifecycle rules

## Make targets

- `make run` — build and run
- `make run-air` — hot reload via [Air](https://github.com/cosmtrek/air)
- `make build` — build the binary
- `make build-image` — build the Docker image
- `make clean` — remove artifacts

## Prerequisites

- Go 1.24.5+
- ffmpeg (only if any profile is `kind: video` and `delivery` is *not* `stream` — the runtime image already includes it)

## Contributing

1. Fork
2. Create a feature branch
3. Make changes (add tests where applicable)
4. Open a PR

## License

MIT
