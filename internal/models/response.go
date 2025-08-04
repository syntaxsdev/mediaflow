package models

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Message string `json:"message"`
}

func NewResponse(message string) *Response {
	return &Response{
		Message: message,
	}
}

func (r *Response) Write(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(r)
}

func (r *Response) WriteError(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(r)
}
