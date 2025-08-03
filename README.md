# MediaFlow - Configurable Media Processing API

A lightweight Go service for processing and serving images with YAML-configurable storage options and S3 integration.

## Features

- **Original Image Serving**: `/photos/{image_path}` - serve original images directly from storage
- **Thumbnail Generation**: `/thumb/photos/{image_path}` - on-demand thumbnail generation
- **YAML Configuration**: Different processing rules per image type (avatar, photo, banner)
- **Multiple Formats**: Convert images to WebP, JPEG, PNG with configurable quality
- **S3 Integration**: Fetch and store images in AWS S3 with folder organization
- **CDN-Optimized**: Cache-Control and ETag headers for optimal CDN performance
- **Graceful Shutdown**: Production-ready server lifecycle management

## API Endpoints

### Original Images
```
GET /photos/{image_path}
```
Serves original images directly from storage.

### Thumbnails
```
GET /thumb/photos/{image_path}?width=256&quality=75
```

**Parameters:**
- `width`: Image width in pixels (1-2048, default: 256)
- `quality`: Image quality (1-100, default: 75)

### Health Check
```
GET /health
```

## Configuration

### Storage Configuration (storage-config.yaml)

MediaFlow uses YAML configuration to define processing rules per image type:

```yaml
storage_options:
  avatar:
    origin_folder: "originals/avatars"
    thumb_folder: "thumbnails/avatars"
    sizes: ["128", "256"]
    quality: 90
    convert_to: "webp"
  
  products:
    origin_folder: "originals/photos"
    thumb_folder: "thumbnails/photos"
    sizes: ["256", "512", "1024"]
    quality: 90
    convert_to: "webp"
  
  default:
    origin_folder: "originals"
    thumb_folder: "thumbnails"
    sizes: ["256", "512"]
    quality: 90
    convert_to: "webp"
```

### Environment Variables

Create a `.env` file for local development:

```bash
# Required
S3_BUCKET=your-bucket-name
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key

# Optional
S3_REGION=us-east-1
PORT=8080
CACHE_MAX_AGE=86400
STORAGE_CONFIG_PATH=storage-config.yaml
```

## Development

### Prerequisites
- Go 1.24.5+
- [Air](https://github.com/cosmtrek/air) for hot reloading (optional)

### Local Development

```bash
# Clone and setup
git clone <repo-url>
cd mediaflow
cp .env.example .env  # Edit with your AWS credentials

# Install dependencies
go mod download

# Development with hot reload
make run-air

# Or build and run
make run
```

### Make Targets

- `make run` - Build and run the server
- `make run-air` - Run with Air for hot reloading during development
- `make build` - Build the binary
- `make clean` - Remove built binary

## Docker Deployment

```bash
# Build image
docker build -t mediaflow .

# Run container
docker run -p 8080:8080 \
  -e S3_BUCKET=your-bucket-name \
  -e AWS_ACCESS_KEY_ID=your-key \
  -e AWS_SECRET_ACCESS_KEY=your-secret \
  -v $(pwd)/storage-config.yaml:/app/storage-config.yaml \
  mediaflow
```

## Production Deployment

1. Set environment variables for your deployment platform
2. Ensure `storage-config.yaml` is available to the container
3. Configure your load balancer to forward requests to the service
4. Set up appropriate S3 bucket policies and IAM roles

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

MIT License