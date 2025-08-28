# MediaFlow | Rolling Batch Parts v1.1 (Stateless Part Batching)

## 1) Problem Statement

Current implementation pre-generates up to 100 part URLs upfront, which:
- Wastes resources for smaller files that don't need all parts
- Limits maximum file size to 100 × part_size (e.g., 800MB with 8MB parts)
- Creates very large response payloads for large files
- Doesn't follow the "rolling batch" pattern from original spec

## 2) Solution: Rolling Batch Parts

Implement on-demand part URL generation in configurable batches.

### Initial Presign Response
- Generate only first **N parts** (configurable, default 10-20)
- Include metadata to indicate more parts available
- Client can request additional batches as upload progresses

### Rolling Batch Endpoint
New endpoint for requesting additional part batches as needed.

## 3) API Changes

### 3.1 Enhanced Multipart Response
```json
{
  "object_key": "raw/43/large-video.mp4",
  "upload": {
    "multipart": {
      "upload_id": "abc123...",
      "part_size": 8388608,
      "total_parts_estimated": 125,
      "parts": [
        {"part_number": 1, "method": "PUT", "url": "...", "expires_at": "..."},
        {"part_number": 2, "method": "PUT", "url": "...", "expires_at": "..."}
        // ... up to initial_batch_size parts
      ],
      "batch_info": {
        "parts_in_batch": 10,
        "next_part_number": 11,
        "has_more_parts": true,
        "batch_endpoint": "/v1/uploads/presign/parts"
      }
    }
  }
}
```

### 3.2 New Endpoint: POST /v1/uploads/presign/parts

**Request:**
```json
{
  "upload_id": "abc123...",
  "object_key": "raw/43/large-video.mp4",
  "start_part": 11,
  "count": 10,
  "expires_seconds": 1800
}
```

**Response:**
```json
{
  "parts": [
    {"part_number": 11, "method": "PUT", "url": "...", "expires_at": "..."},
    {"part_number": 12, "method": "PUT", "url": "...", "expires_at": "..."}
    // ... up to 'count' parts
  ],
  "batch_info": {
    "parts_in_batch": 10,
    "next_part_number": 21,
    "has_more_parts": true
  }
}
```

## 4) Configuration

### 4.1 Profile Config Extensions
```yaml
profiles:
  video:
    # ... existing config
    initial_batch_size: 10        # Parts in first response
    max_batch_size: 20           # Max parts per batch request
    part_url_ttl_seconds: 1800   # Individual part URL expiry
```

### 4.2 Batch Size Logic
- **Small files** (< 10 parts): Generate all parts in initial response
- **Medium files** (10-50 parts): Generate initial batch, allow additional requests
- **Large files** (50+ parts): Rolling batches required

## 5) Implementation Details

### 5.1 Service Layer Changes
```go
type BatchInfo struct {
    PartsInBatch   int  `json:"parts_in_batch"`
    NextPartNumber int  `json:"next_part_number,omitempty"`
    HasMoreParts   bool `json:"has_more_parts"`
    BatchEndpoint  string `json:"batch_endpoint,omitempty"`
}

type MultipartUpload struct {
    UploadID         string     `json:"upload_id"`
    PartSize         int64      `json:"part_size"`
    TotalPartsEst    int        `json:"total_parts_estimated"`
    Parts            []PartUpload `json:"parts"`
    BatchInfo        *BatchInfo `json:"batch_info,omitempty"`
}
```

### 5.2 Batch Request Validation
- Validate `upload_id` exists and is not expired
- Validate `object_key` matches the upload
- Validate `start_part` is sequential (no gaps)
- Validate `count` ≤ `max_batch_size`
- Rate limiting per upload_id

### 5.3 Error Handling
```json
{
  "code": "invalid_upload_id",
  "message": "Upload ID not found or expired",
  "hint": "Start a new upload"
}

{
  "code": "invalid_part_range", 
  "message": "Part 25 requested but only up to part 20 uploaded",
  "hint": "Request sequential parts only"
}

{
  "code": "batch_size_exceeded",
  "message": "Requested 50 parts, maximum 20 allowed",
  "hint": "Reduce count parameter"
}
```

## 6) Client Workflow

### 6.1 Upload Flow
1. **Initial presign**: Get first batch of part URLs + upload metadata
2. **Start uploading**: Use provided part URLs 
3. **Monitor progress**: When approaching end of current batch
4. **Request next batch**: Call `/presign/parts` for more URLs
5. **Continue upload**: Use new part URLs
6. **Complete upload**: Client calls S3 CompleteMultipartUpload

### 6.2 Batch Request Timing
- **Proactive**: Request next batch when 80% through current batch
- **Lazy**: Request next batch only when current batch exhausted
- **Configurable**: Client can tune based on upload speed

## 7) Benefits

### 7.1 Efficiency
- Smaller initial response payloads
- Generate URLs only as needed
- Reduce wasted presigned URLs for failed uploads

### 7.2 Scalability  
- Support unlimited file sizes
- Configurable batch sizes per use case
- Better resource utilization

### 7.3 Flexibility
- Client controls batch timing
- Different strategies for different file types
- Graceful handling of network interruptions

## 8) Configuration Examples

### 8.1 Small Files (Images, Documents)
```yaml
avatar:
  initial_batch_size: 5
  max_batch_size: 10
  part_size_mb: 5
```

### 8.2 Large Files (Videos)  
```yaml
video:
  initial_batch_size: 10
  max_batch_size: 20
  part_size_mb: 8
```

### 8.3 Massive Files (Raw Video, Datasets)
```yaml
raw_video:
  initial_batch_size: 15
  max_batch_size: 25
  part_size_mb: 16
```

## 9) Migration Path

### 9.1 Phase 1: Add Batch Info
- Add `batch_info` to existing multipart responses
- Maintain current behavior (generate all parts up to 100)
- `has_more_parts: false` for backward compatibility

### 9.2 Phase 2: Implement Rolling Batches
- Add `/presign/parts` endpoint
- Reduce initial batch sizes
- Enable `has_more_parts: true` for large files

### 9.3 Phase 3: Optimize
- Add rate limiting and abuse prevention
- Implement batch size auto-tuning
- Add metrics and monitoring

## 10) Security Considerations

### 10.1 Upload ID Validation
- Verify upload_id is valid and not expired
- Prevent unauthorized part generation
- Log all batch requests for audit

### 10.2 Rate Limiting
- Limit batch requests per upload_id per minute
- Prevent abuse of part generation endpoint
- Implement exponential backoff hints

### 10.3 Resource Protection
- Maximum total parts per upload (e.g., 10,000)
- Maximum concurrent uploads per client
- Cleanup expired upload sessions

## 11) Future Enhancements

### 11.1 Adaptive Batching
- Auto-adjust batch size based on upload speed
- Larger batches for fast connections
- Smaller batches for slow/unreliable connections

### 11.2 Part Prediction
- Pre-generate next batch based on upload progress
- Cache commonly requested part ranges
- Optimize for typical upload patterns

### 11.3 Resume Support
- Track which parts have been uploaded
- Support resume from any part number
- Handle partial uploads gracefully