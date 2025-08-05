package response

import (
	"encoding/json"
	"net/http"
)

type ResponseWriter interface {
	Write(w http.ResponseWriter)
	WriteError(w http.ResponseWriter, status int)
}

// JSONResponse for API endpoints
type JSONResponse struct {
	Message string `json:"message"`
}

func (r *JSONResponse) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(r)
}

func (r *JSONResponse) WriteError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(r)
}

type PlainResponse struct {
	Message string
}

func (r *PlainResponse) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(r.Message))
}

func (r *PlainResponse) WriteError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	w.Write([]byte(r.Message))
}

func NewJSONResponse(message string) ResponseWriter {
	return &JSONResponse{Message: message}
}

func NewPlainResponse(message string) ResponseWriter {
	return &PlainResponse{Message: message}
}

// Convenience functions for common patterns
func JSON(message string) ResponseWriter {
	return NewJSONResponse(message)
}

func Plain(message string) ResponseWriter {
	return NewPlainResponse(message)
}
