package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	utils "mediaflow/internal"
	"mediaflow/internal/config"
	"mediaflow/internal/service"
)

type ImageAPI struct {
	imageService  *service.ImageService
	storageConfig *config.StorageConfig
	ctx           context.Context
}

func NewImageAPI(ctx context.Context, imageService *service.ImageService, storageConfig *config.StorageConfig) *ImageAPI {
	return &ImageAPI{
		imageService:  imageService,
		storageConfig: storageConfig,
		ctx:           ctx,
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

	h.HandleThumbnailType(w, r, imageData, thumbType, fileName)
}

func (h *ImageAPI) HandleThumbnailType(w http.ResponseWriter, r *http.Request, imageData []byte, thumbType, imagePath string) {
	so := h.storageConfig.GetStorageOptions(thumbType)

	if r.Method == http.MethodPost {
		err := h.imageService.UploadImage(h.ctx, so, imageData, thumbType, imagePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if r.Method == http.MethodGet {
		size, _, err := parseQueryParams(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// size := fmt.Sprintf("%d_%d", width, quality)

		baseName := utils.BaseName(imagePath)
		imageData, err := h.imageService.GetImage(h.ctx, so, false, baseName, size)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cd := so.CacheDuration
		if cd == 0 {
			// 24 hours
			cd = 86400
		}

		w.Header().Set("Content-Type", "image/"+so.ConvertTo)
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", cd))
		w.Header().Set("ETag", fmt.Sprintf(`"%s/%s_%s"`, thumbType, baseName, size))
		w.Write(imageData)
	}
}

func parseQueryParams(r *http.Request) (width, quality string, err error) {
	var w int = 256
	var q int = 80

	if width := r.URL.Query().Get("width"); width != "" {
		w, err = strconv.Atoi(width)
		if err != nil {
			return "0", "0", fmt.Errorf("invalid width parameter")
		}
		if w <= 0 || w > 2048 {
			return "0", "0", fmt.Errorf("width must be between 1 and 2048")
		}
	}

	if quality := r.URL.Query().Get("quality"); quality != "" {
		q, err = strconv.Atoi(quality)
		if err != nil {
			return "0", "0", fmt.Errorf("invalid quality parameter")
		}
		if q < 1 || q > 100 {
			return "0", "0", fmt.Errorf("quality must be between 1 and 100")
		}
	}

	return fmt.Sprintf("%d", w), fmt.Sprintf("%d", q), nil
}
