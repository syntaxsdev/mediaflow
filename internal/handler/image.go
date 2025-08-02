package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"mediacdn/internal/service"
)

type ImageHandler struct {
	imageService *service.ImageService
}

func NewImageHandler(imageService *service.ImageService) *ImageHandler {
	return &ImageHandler{
		imageService: imageService,
	}
}

func (h *ImageHandler) HandleThumbnail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	imagePath := strings.TrimPrefix(r.URL.Path, "/thumb/photos/")
	if imagePath == "" {
		http.Error(w, "Image ID required", http.StatusBadRequest)
		return
	}

	width, quality, err := parseQueryParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	processedImage, contentType, err := h.imageService.ProcessImage(r.Context(), imagePath, width, quality)
	if err != nil {
		log.Printf("Error processing image %s: %v", imagePath, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("ETag", fmt.Sprintf(`"%s_%d_%d"`, imagePath, width, quality))

	if _, err := w.Write(processedImage); err != nil {
		log.Printf("Error writing response: %v", err)
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