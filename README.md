# MediaCDN - Go Image Processing API

A lightweight, production-ready Go service for processing and serving images from S3 with CDN-friendly caching headers.

## Features

- HTTP API for image thumbnails: `/thumb/photos/{image_id}`
- Query parameters for width and quality control
- S3 integration for fetching original images
- Efficient image processing with resize and compression
- CDN-optimized headers (Cache-Control, ETag)
- Docker-ready single binary deployment
- Graceful shutdown handling

## API Usage

```
GET /thumb/photos/product123.jpg?width=256&quality=75
```

### Parameters
- `width`: Image width in pixels (1-2048, default: 256)
- `quality`: JPEG quality (1-100, default: 75)

## Environment Variables

```bash
PORT=8080                    # Server port
S3_BUCKET=your-bucket-name   # S3 bucket containing images
S3_REGION=us-east-1         # AWS region
AWS_ACCESS_KEY_ID=your-key   # AWS credentials (optional if using IAM)
AWS_SECRET_ACCESS_KEY=secret # AWS credentials (optional if using IAM)
CACHE_MAX_AGE=86400         # Cache duration in seconds
```

## Running Locally

```bash
# Set environment variables
export S3_BUCKET=your-bucket-name
export AWS_ACCESS_KEY_ID=your-key
export AWS_SECRET_ACCESS_KEY=your-secret

# Build and run
go build -o mediacdn .
./mediacdn
```

## Docker Deployment

```bash
# Build image
docker build -t mediacdn .

# Run container
docker run -p 8080:8080 \
  -e S3_BUCKET=your-bucket-name \
  -e AWS_ACCESS_KEY_ID=your-key \
  -e AWS_SECRET_ACCESS_KEY=your-secret \
  mediacdn
```

## Health Check

```
GET /health
```

Returns `200 OK` when service is healthy.
