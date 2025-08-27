package upload

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"mediaflow/internal/config"
)

type Handler struct {
	uploadService *Service
	storageConfig *config.StorageConfig
	ctx           context.Context
}

func NewHandler(ctx context.Context, uploadService *Service, storageConfig *config.StorageConfig) *Handler {
	return &Handler{
		uploadService: uploadService,
		storageConfig: storageConfig,
		ctx:           ctx,
	}
}

// HandlePresign handles POST /v1/uploads/presign
func (h *Handler) HandlePresign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "Method not allowed", "")
		return
	}

	// Parse request body
	var req PresignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "Invalid request body", "")
		return
	}

	// Validate required fields
	if req.KeyBase == "" {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "key_base is required", "")
		return
	}
	if req.Ext == "" {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "ext is required", "")
		return
	}
	if req.Mime == "" {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "mime is required", "")
		return
	}
	if req.SizeBytes <= 0 {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "size_bytes must be greater than 0", "")
		return
	}
	if req.Kind == "" {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "kind is required", "")
		return
	}
	if req.Profile == "" {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "profile is required", "")
		return
	}

	// Get profile configuration
	profile := h.storageConfig.GetProfile(req.Profile)
	if profile == nil {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, fmt.Sprintf("No configuration for profile: %s", req.Profile), "Configure profile in your storage config")
		return
	}

	// Validate kind matches profile
	if profile.Kind != req.Kind {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, fmt.Sprintf("Kind mismatch: expected %s, got %s", profile.Kind, req.Kind), "")
		return
	}

	// Generate presigned upload
	presignResp, err := h.uploadService.PresignUpload(h.ctx, &req, profile)
	if err != nil {
		if err.Error() == fmt.Sprintf("mime type not allowed: %s", req.Mime) {
			h.writeError(w, http.StatusBadRequest, ErrMimeNotAllowed, err.Error(), "Check allowed_mimes in upload configuration")
			return
		}
		if err.Error() == fmt.Sprintf("file size exceeds maximum: %d > %d", req.SizeBytes, profile.SizeMaxBytes) {
			h.writeError(w, http.StatusBadRequest, ErrSizeTooLarge, err.Error(), "Reduce file size or check size_max_bytes in configuration")
			return
		}
		// Log the actual error for debugging
		fmt.Printf("Upload error: %v\n", err)
		h.writeError(w, http.StatusInternalServerError, ErrBadRequest, fmt.Sprintf("Failed to generate presigned upload: %v", err), "")
		return
	}

	// Return presigned response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(presignResp)
}

// Note: Part presigning is now handled via batch presigning in the main endpoint
// No separate part endpoint needed for stateless design

// writeError writes a standardized error response
func (h *Handler) writeError(w http.ResponseWriter, statusCode int, code, message, hint string) {
	errorResp := ErrorResponse{
		Code:    code,
		Message: message,
		Hint:    hint,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(errorResp)
}
