# MediaFlow - Configurable Media Processing API

A lightweight Go service for processing and serving images with YAML-configurable storage options and S3 integration.

## Features

- **Presigned Uploads**: `/v1/uploads/presign` - secure direct-to-S3 uploads with validation
- **Original Image Serving**: `/originals/{type}/{image_id}` - serve original images directly from storage
- **Thumbnail Generation**: `/thumb/{type}/{image_id}` - on-demand thumbnail generation
- **Unified Configuration**: Profile-based YAML config combining upload and processing rules
- **Multiple Formats**: Convert images to WebP, JPEG, PNG with configurable quality
- **Video Support**: Ready for video upload and processing (processing features coming soon)
- **S3 Integration**: Direct S3 uploads with multipart support for large files
- **CDN-Optimized**: Cache-Control and ETag headers for optimal CDN performance
- **Graceful Shutdown**: Production-ready server lifecycle management


## Future Features
- Other media support (currently on images)
- Server level caching of high frequency media

## API Endpoints

### Presigned Uploads
```
POST /v1/uploads/presign
```
Generates presigned URLs for secure direct-to-S3 uploads.

**Request Body:**
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

**Response for Single Upload:**
```json
{
  "object_key": "raw/ab/unique-file-id.jpg",
  "upload": {
    "single": {
      "method": "PUT",
      "url": "https://presigned-s3-url",
      "headers": {
        "Content-Type": "image/jpeg",
        "If-None-Match": "*"
      },
      "expires_at": "2024-01-01T12:00:00Z"
    }
  }
}
```

**Response for Multipart Upload:**
```json
{
  "object_key": "raw/ab/unique-file-id.jpg",
  "upload": {
    "multipart": {
      "upload_id": "abc123xyz",
      "part_size": 8388608,
      "parts": [
        {
          "part_number": 1,
          "method": "PUT",
          "url": "https://presigned-s3-part-url-1",
          "headers": {"Content-Type": "image/jpeg"},
          "expires_at": "2024-01-01T12:00:00Z"
        }
      ],
      "complete": {
        "method": "POST",
        "url": "https://your-api/v1/uploads/raw%2Fab%2Funique-file-id.jpg/complete/abc123xyz",
        "headers": {"Content-Type": "application/json"},
        "expires_at": "2024-01-01T12:00:00Z"
      },
      "abort": {
        "method": "DELETE", 
        "url": "https://your-api/v1/uploads/raw%2Fab%2Funique-file-id.jpg/abort/abc123xyz",
        "headers": {},
        "expires_at": "2024-01-01T12:00:00Z"
      }
    }
  }
}
```

**Parameters:**
- `key_base`: Unique identifier for the file
- `ext`: File extension (optional, for backward compatibility)
- `mime`: MIME type of the file
- `size_bytes`: File size in bytes
- `kind`: Media type (`image` or `video`)
- `profile`: Configuration profile to use (`avatar`, `photo`, `video`, etc.)
- `multipart`: Upload strategy (`auto`, `force`, or `off`)

### Multipart Upload Completion
```
POST /v1/uploads/{object_key}/complete/{upload_id}
```
Completes a multipart upload by providing the ETags for all uploaded parts.

**Request Body:**
```json
{
  "parts": [
    {
      "part_number": 1,
      "etag": "\"d41d8cd98f00b204e9800998ecf8427e\""
    },
    {
      "part_number": 2,
      "etag": "\"098f6bcd4621d373cade4e832627b4f6\""
    }
  ]
}
```

**Response:**
```json
{
  "status": "completed",
  "object_key": "raw/ab/unique-file-id.jpg"
}
```

### Multipart Upload Abort
```
DELETE /v1/uploads/{object_key}/abort/{upload_id}
```
Aborts a multipart upload and cleans up any uploaded parts.

**Response:**
```json
{
  "status": "aborted",
  "upload_id": "abc123xyz"
}
```

### Thumbnails
```
GET /thumb/{type}/{image_id}?width=512
POST /thumb/{type}/{image_id}
```
Generates and serves thumbnails with configurable dimensions. POST requests require authentication.

**GET Parameters:**
- `type`: Image category (avatar, photo, banner, or any configured type)
- `image_id`: Unique identifier for the image
- `width`: Image width in pixels (optional, defaults to the type's `default_size` from storage config)

**POST Parameters:**
- Requires authentication (API key)
- Request body should contain the image data
- Used for uploading images to be processed

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

MediaFlow uses YAML configuration to define profiles that combine upload settings and processing rules:

```yaml
profiles:
  avatar:
    # Upload configuration
    kind: "image"
    allowed_mimes: ["image/jpeg", "image/png", "image/webp"]
    size_max_bytes: 5242880  # 5MB
    multipart_threshold_mb: 15
    part_size_mb: 8
    token_ttl_seconds: 900  # 15 minutes
    path_template: "raw/{shard?}/{key_base}"
    enable_sharding: true
    
    # Processing configuration
    origin_folder: "originals/avatars"
    thumb_folder: "thumbnails/avatars"
    sizes: ["128", "256"]
    default_size: "256"
    quality: 90
    convert_to: "webp"
  
  photo:
    kind: "image"
    allowed_mimes: ["image/jpeg", "image/png", "image/webp"]
    size_max_bytes: 20971520  # 20MB
    multipart_threshold_mb: 15
    part_size_mb: 8
    token_ttl_seconds: 900
    path_template: "raw/{shard?}/{key_base}"
    enable_sharding: true
    
    origin_folder: "originals/photos"
    thumb_folder: "thumbnails/photos"
    sizes: ["256", "512", "1024"]
    default_size: "256"
    quality: 90
    convert_to: "webp"
  
  video:
    kind: "video"
    allowed_mimes: ["video/mp4", "video/quicktime", "video/webm"]
    size_max_bytes: 104857600  # 100MB
    multipart_threshold_mb: 15
    part_size_mb: 8
    token_ttl_seconds: 1800  # 30 minutes
    path_template: "raw/{shard?}/{key_base}"
    enable_sharding: true
    
    origin_folder: "originals/videos"
    thumb_folder: "posters/videos"  # Video thumbnails
    proxy_folder: "proxies/videos"   # Compressed versions
    formats: ["mp4", "webm"]
    quality: 80

  default:
    kind: "image"
    allowed_mimes: ["image/jpeg", "image/png"]
    size_max_bytes: 10485760  # 10MB
    multipart_threshold_mb: 15
    part_size_mb: 8
    token_ttl_seconds: 900
    path_template: "raw/{shard?}/{key_base}"
    enable_sharding: true
    
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
git clone https://github.com/syntaxsdev/mediaflow
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