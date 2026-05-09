package upload

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mediaflow/internal/config"
	"mediaflow/internal/probe"
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

	// Construct base URL from request
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	// Generate presigned upload
	presignResp, err := h.uploadService.PresignUpload(h.ctx, &req, profile, baseURL)
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

// HandleCompleteMultipart handles POST /v1/uploads/{object_key}/complete/{upload_id}
func (h *Handler) HandleCompleteMultipart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "Method not allowed", "")
		return
	}

	// Extract object_key and upload_id from URL path
	path := strings.TrimPrefix(r.URL.Path, "/v1/uploads/")
	parts := strings.Split(path, "/complete/")
	if len(parts) != 2 {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "Invalid URL format", "Expected /v1/uploads/{object_key}/complete/{upload_id}")
		return
	}

	objectKey := parts[0]
	uploadID := parts[1]

	// Parse request body
	var req CompleteMultipartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "Invalid request body", "")
		return
	}

	// Validate required fields
	if len(req.Parts) == 0 {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "parts is required and cannot be empty", "")
		return
	}

	// Complete the multipart upload
	err := h.uploadService.CompleteMultipartUpload(h.ctx, objectKey, uploadID, &req)
	if err != nil {
		fmt.Printf("Complete multipart error: %v\n", err)
		h.writeError(w, http.StatusInternalServerError, ErrBadRequest, fmt.Sprintf("Failed to complete multipart upload: %v", err), "")
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]string{"status": "completed", "object_key": objectKey}
	_ = json.NewEncoder(w).Encode(response)
}

// HandleAbortMultipart handles DELETE /v1/uploads/{object_key}/abort/{upload_id}
func (h *Handler) HandleAbortMultipart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "Method not allowed", "")
		return
	}

	// Extract object_key and upload_id from URL path
	path := strings.TrimPrefix(r.URL.Path, "/v1/uploads/")
	parts := strings.Split(path, "/abort/")
	if len(parts) != 2 {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "Invalid URL format", "Expected /v1/uploads/{object_key}/abort/{upload_id}")
		return
	}

	objectKey := parts[0]
	uploadID := parts[1]

	// Abort the multipart upload
	err := h.uploadService.AbortMultipartUpload(h.ctx, objectKey, uploadID)
	if err != nil {
		fmt.Printf("Abort multipart error: %v\n", err)
		h.writeError(w, http.StatusInternalServerError, ErrBadRequest, fmt.Sprintf("Failed to abort multipart upload: %v", err), "")
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]string{"status": "aborted", "upload_id": uploadID}
	_ = json.NewEncoder(w).Encode(response)
}

// RouteAssets dispatches /v1/assets/{profile}/{key_base}[/probe] to the right
// handler based on method + suffix.
func (h *Handler) RouteAssets(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/probe"):
		h.HandleProbeAsset(w, r)
	case r.Method == http.MethodDelete:
		h.HandleDeleteAsset(w, r)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "Method not allowed", "")
	}
}

// HandleDeleteAsset handles DELETE /v1/assets/{profile}/{key_base}
// Deletes the original file and all generated thumbnails for an asset.
func (h *Handler) HandleDeleteAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "Method not allowed", "")
		return
	}

	profileName, keyBase, ok := parseAssetPath(r.URL.Path, "")
	if !ok {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "Invalid URL format", "Expected /v1/assets/{profile}/{key_base}")
		return
	}

	// Look up profile config
	profile := h.storageConfig.GetProfile(profileName)
	if profile == nil {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, fmt.Sprintf("Unknown profile: %s", profileName), "")
		return
	}

	// Delete the original + thumbnails
	deleted, err := h.uploadService.DeleteAsset(h.ctx, profile, keyBase)
	if err != nil {
		fmt.Printf("Delete asset error: %v\n", err)
		h.writeError(w, http.StatusInternalServerError, ErrStorageDenied, fmt.Sprintf("Failed to delete asset: %v", err), "")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]any{
		"status":          "deleted",
		"profile":         profileName,
		"key_base":        keyBase,
		"objects_deleted": deleted,
	}
	_ = json.NewEncoder(w).Encode(response)
}

// HandleProbeAsset handles POST /v1/assets/{profile}/{key_base}/probe.
// Returns 200 for both pass and fail; `ok` is the gate, not the status code.
func (h *Handler) HandleProbeAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "Method not allowed", "Use POST")
		return
	}

	profileName, keyBase, ok := parseAssetPath(r.URL.Path, "/probe")
	if !ok {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "Invalid URL format", "Expected /v1/assets/{profile}/{key_base}/probe")
		return
	}

	profile := h.storageConfig.GetProfile(profileName)
	if profile == nil {
		h.writeError(w, http.StatusNotFound, ErrBadRequest, fmt.Sprintf("Unknown profile: %s", profileName), "")
		return
	}
	if profile.Kind != "video" {
		h.writeError(w, http.StatusUnprocessableEntity, ErrBadRequest, "Probe requires kind=video", profileName)
		return
	}

	objectKey := h.uploadService.ResolveAssetKey(profile, keyBase)

	if err := h.uploadService.AssetExists(r.Context(), objectKey); err != nil {
		h.writeError(w, http.StatusNotFound, ErrBadRequest, "Asset not found", objectKey)
		return
	}

	presignURL, err := h.uploadService.PresignGet(r.Context(), objectKey, 120*time.Second)
	if err != nil {
		fmt.Printf("Probe presign error: %v\n", err)
		h.writeError(w, http.StatusInternalServerError, ErrBadRequest, "Failed to presign GET", err.Error())
		return
	}

	result, err := probe.Probe(r.Context(), presignURL, objectKey, profile)
	if err != nil {
		fmt.Printf("Probe error key=%s: %v\n", objectKey, err)
		h.writeError(w, http.StatusBadGateway, ErrUpstream, "Probe failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

// parseAssetPath extracts {profile} and {key_base} from /v1/assets/{profile}/{key_base}{suffix}.
func parseAssetPath(urlPath, suffix string) (profile, keyBase string, ok bool) {
	path := strings.TrimPrefix(urlPath, "/v1/assets/")
	if suffix != "" {
		if !strings.HasSuffix(path, suffix) {
			return "", "", false
		}
		path = strings.TrimSuffix(path, suffix)
	}
	slashIdx := strings.Index(path, "/")
	if slashIdx < 1 || slashIdx == len(path)-1 {
		return "", "", false
	}
	return path[:slashIdx], path[slashIdx+1:], true
}

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
