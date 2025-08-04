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
	"mediaflow/internal/models"
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

func (h *ImageAPI) HandleThumbnailTypes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/thumb/"), "/")
	thumbType := parts[0]
	fileName := parts[1]

	var imageData []byte
	var mimeType string

	if r.Method == http.MethodPost {
		file, _, err := r.FormFile("file")
		if err != nil {
			models.NewResponse(err.Error()).WriteError(w, http.StatusBadRequest)
			return
		}
		defer file.Close()

		mimeType, err = service.DetermineMimeType(file)
		if err != nil {
			models.NewResponse(err.Error()).WriteError(w, http.StatusBadRequest)
			return
		}
		if mimeType != "image/jpeg" && mimeType != "image/png" {
			models.NewResponse("Invalid file type").WriteError(w, http.StatusBadRequest)
			return
		}
		imageData, err = io.ReadAll(file)
		if err != nil {
			models.NewResponse(err.Error()).WriteError(w, http.StatusBadRequest)
			return

		}
	}

	h.HandleThumbnailType(w, r, imageData, thumbType, fileName)
}

func (h *ImageAPI) HandleThumbnailType(w http.ResponseWriter, r *http.Request, imageData []byte, thumbType, imagePath string) {
	so := h.storageConfig.GetStorageOptions(thumbType)
	baseName := utils.BaseName(imagePath)
	if r.Method == http.MethodPost {
		err := h.imageService.UploadImage(h.ctx, so, imageData, thumbType, baseName)
		if err != nil {
			models.NewResponse(err.Error()).WriteError(w, http.StatusInternalServerError)
			return
		}
	}

	if r.Method == http.MethodGet {
		size, _, err := parseQueryParams(r)
		if err != nil {
			models.NewResponse(err.Error()).WriteError(w, http.StatusBadRequest)
			return
		}
		imageData, err := h.imageService.GetImage(h.ctx, so, false, baseName, size)
		if err != nil {
			models.NewResponse(err.Error()).WriteError(w, http.StatusInternalServerError)
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

func (h *ImageAPI) HandleOriginals(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/originals/"), "/")
	thumbType := parts[0]
	fileName := parts[1]

	h.HandleThumbnailType(w, r, nil, thumbType, fileName)
}

// Utils that belong here

// Parse query params for width and quality
func parseQueryParams(r *http.Request) (width, quality string, err error) {
	var w int
	var q int

	if width := r.URL.Query().Get("width"); width != "" {
		w, err = strconv.Atoi(width)
		if err != nil {
			return "", "", fmt.Errorf("invalid width parameter")
		}
		if w <= 0 || w > 2048 {
			return "", "", fmt.Errorf("width must be between 1 and 2048")
		}
	}

	if quality := r.URL.Query().Get("quality"); quality != "" {
		q, err = strconv.Atoi(quality)
		if err != nil {
			return "", "", fmt.Errorf("invalid quality parameter")
		}
		if q < 1 || q > 100 {
			return "", "", fmt.Errorf("quality must be between 1 and 100")
		}
	}

	return width, quality, nil
}
