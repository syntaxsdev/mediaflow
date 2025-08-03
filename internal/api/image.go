package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"mediacdn/internal/config"
	"mediacdn/internal/service"
)

type ImageAPI struct {
	imageService *service.ImageService
	ctx          context.Context
}

func NewImageAPI(ctx context.Context, imageService *service.ImageService) *ImageAPI {
	return &ImageAPI{
		imageService: imageService,
		ctx:          ctx,
	}
}

func (h *ImageAPI) HandleThumbnail(w http.ResponseWriter, r *http.Request) {
	// if r.Method != http.MethodGet {
	// 	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	// 	return
	// }

	// imagePath := strings.TrimPrefix(r.URL.Path, "/thumb/photos/")
	// if imagePath == "" {
	// 	http.Error(w, "Image ID required", http.StatusBadRequest)
	// 	return
	// }

	// width, quality, err := parseQueryParams(r)
	// if err != nil {
	// 	http.Error(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }

	// processedImage, contentType, err := h.imageService.ProcessImage(r.Context(), nil, imagePath, width, quality)
	// if err != nil {
	// 	log.Printf("Error processing image %s: %v", imagePath, err)
	// http.Error(w, "Internal server error", http.StatusInternalServerError)
	// 	return
	// }

	// w.Header().Set("Content-Type", contentType)
	// w.Header().Set("Cache-Control", "public, max-age=86400")
	// w.Header().Set("ETag", fmt.Sprintf(`"%s_%d_%d"`, imagePath, width, quality))

	// if _, err := w.Write(processedImage); err != nil {
	// 	log.Printf("Error writing response: %v", err)
	// }
}

func (h *ImageAPI) HandleThumbnailTypes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/thumb/"), "/")
	thumbType := parts[0]
	fileName := parts[1]

	var imageData []byte

	if r.Method == http.MethodPost {
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		mimeType, err := service.DetermineMimeType(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if mimeType != "image/jpeg" && mimeType != "image/png" {
			http.Error(w, "Invalid file type", http.StatusBadRequest)
			return
		}
		imageData, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	switch thumbType {
	case "profile":
		h.HandleAvatar(w, r, imageData, thumbType, fileName)
	}
}

func (h *ImageAPI) HandleAvatar(w http.ResponseWriter, r *http.Request, imageData []byte, thumbType, imagePath string) {
	so := &config.StorageOptions{
		OriginFolder: "originals",
		ThumbFolder:  "thumbnails",
		Sizes:        []string{"256", "512"},
		Quality:      80,
		ConvertTo:    "webp",
	}
	if r.Method == http.MethodPost {
		err := h.imageService.UploadImage(h.ctx, so, imageData, thumbType, imagePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func parseQueryParams(r *http.Request) (width, quality int, err error) {
	width = 256
	quality = 75

	if w := r.URL.Query().Get("width"); w != "" {
		width, err = strconv.Atoi(w)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid width parameter")
		}
		if width <= 0 || width > 2048 {
			return 0, 0, fmt.Errorf("width must be between 1 and 2048")
		}
	}

	if q := r.URL.Query().Get("quality"); q != "" {
		quality, err = strconv.Atoi(q)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid quality parameter")
		}
		if quality < 1 || quality > 100 {
			return 0, 0, fmt.Errorf("quality must be between 1 and 100")
		}
	}

	return width, quality, nil
}
