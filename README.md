# MediaFlow - Configurable Media Processing API

A lightweight Go service for processing and serving images with YAML-configurable storage options and S3 integration.

## Features

- **Original Image Serving**: `/originals/{type}/{image_id}` - serve original images directly from storage
- **Thumbnail Generation**: `/thumb/{type}/{image_id}` - on-demand thumbnail generation
- **YAML Configuration**: Different processing rules per image type (avatar, photo, banner)
- **Multiple Formats**: Convert images to WebP, JPEG, PNG with configurable quality
- **S3 Integration**: Fetch and store images in AWS S3 with folder organization
- **CDN-Optimized**: Cache-Control and ETag headers for optimal CDN performance
- **Graceful Shutdown**: Production-ready server lifecycle management


## Future Features
- Other media support (currently on images)
- Server level caching of high frequency media

## API Endpoints

### Thumbnails
```
GET /thumb/{type}/{image_id}?width=512
```
Generates and serves thumbnails with configurable dimensions.

**Parameters:**
- `type`: Image category (avatar, photo, banner, or any configured type)
- `image_id`: Unique identifier for the image
- `width`: Image width in pixels (optional, defaults to the type's `default_size` from storage config)

### Original Images
```
GET /originals/{type}/{image_id}
```
Serves original images directly from storage.

**Parameters:**
- `type`: Image category (avatar, photo, banner, or any configured type)
- `image_id`: Unique identifier for the image

### Health Check
```
GET /health
```
Returns service health status.

## Configuration

### Storage Configuration (storage-config.yaml)

MediaFlow uses YAML configuration to define processing rules per image type:

```yaml
storage_options:
  avatar:
    origin_folder: "originals/avatars"
    thumb_folder: "thumbnails/avatars"
    sizes: ["128", "256"]
    default_size: "256"
    quality: 90
    convert_to: "webp"
  
  photo:
    origin_folder: "originals/photos"
    thumb_folder: "thumbnails/photos"
    sizes: ["256", "512", "1024"]
    default_size: "256"
    quality: 90
    convert_to: "webp"
  
  banner:
    origin_folder: "originals/banners"
    thumb_folder: "thumbnails/banners"
    sizes: ["512", "1024", "2048"]
    default_size: "512"
    quality: 95
    convert_to: "webp"
  
  default:
    origin_folder: "originals"
    thumb_folder: "thumbnails"
    sizes: ["256", "512"]
    default_size: "256"
    quality: 90
    convert_to: "webp"
```

### Environment Variables

Create a `.env` file for local development:

```bash
# Required
S3_BUCKET=your-bucket-name

# AWS Credentials (use one of the following methods)
# Method 1: Direct credentials (local development)
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key

# Method 2: IAM Role (recommended for ECS/EC2)
# No credentials needed - uses IAM role attached to ECS task/EC2 instance

# Optional
S3_REGION=us-east-1
PORT=8080
CACHE_MAX_AGE=86400
STORAGE_CONFIG_PATH=storage-config.yaml
```

## Docker Deployment

### Using Pre-built Image
```bash
# Pull and run the latest image
docker run -p 8080:8080 \
  -e S3_BUCKET=your-bucket-name \
  -e AWS_ACCESS_KEY_ID=your-key \
  -e AWS_SECRET_ACCESS_KEY=your-secret \
  -v $(pwd)/storage-config.yaml:/app/storage-config.yaml \
  syntaxsdev/mediaflow:latest
```

### Building from Source
```bash
make build-image
```

## Production Deployment

### ECS Deployment
1. Create an IAM role with S3 permissions (see AWS S3 IAM Integration section)
2. Create an ECS task definition using `syntaxsdev/mediaflow:latest`
3. Attach the IAM role to your ECS task
4. Set environment variables: `S3_BUCKET`, `S3_REGION`, `PORT`
5. Mount or include your `storage-config.yaml` file
6. Configure your Application Load Balancer to forward requests to the ECS service

### General Deployment Steps
1. Set environment variables for your deployment platform
2. Ensure `storage-config.yaml` is available to the container
3. Configure your load balancer to forward requests to the service
4. Set up appropriate S3 bucket policies and IAM roles
5. Consider using a CDN (CloudFront) for better performance and caching

#### For EC2 Deployments
1. Attach the same IAM role to your EC2 instance
2. MediaFlow will use the instance's IAM role credentials automatically

### AWS S3 IAM Integration

MediaFlow supports automatic AWS S3 authentication through IAM roles, making it ideal for ECS deployments:

#### For ECS Deployments (Recommended)
1. Create an IAM role with S3 permissions:
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

2. Attach the IAM role to your ECS task definition
3. Remove `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` from environment variables
4. MediaFlow will automatically use the ECS task's IAM role credentials


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

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

MIT License