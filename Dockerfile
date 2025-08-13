    FROM golang:1.24-alpine AS builder

    WORKDIR /app
    
    # Install build dependencies (including libwebp and vips for CGO)
    RUN apk add --no-cache build-base libwebp-dev vips-dev pkgconfig
    
    COPY go.mod go.sum ./
    RUN go mod download
    
    COPY . .
    
    # Build with CGO enabled
    RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o mediaflow .
    
    FROM alpine:latest
    
    # Install ca-certificates, libwebp and vips runtime (with fallback for ARM64)
    RUN apk --no-cache add ca-certificates && \
        (apk add --no-cache libwebp vips || echo "libwebp/vips not available for this platform")
    
    # Create a non-root user
    RUN addgroup -g 1001 -S appgroup && \
        adduser -u 1001 -S appuser -G appgroup
    
    WORKDIR /app
    
    COPY --from=builder /app/mediaflow .
    
    RUN chown -R appuser:appgroup /app
    
    USER appuser
    
    EXPOSE 8080
    
    CMD ["./mediaflow"]
    