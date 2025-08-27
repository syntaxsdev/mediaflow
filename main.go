package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	utils "mediaflow/internal"
	"mediaflow/internal/api"
	"mediaflow/internal/auth"
	"mediaflow/internal/config"
	"mediaflow/internal/response"
	"mediaflow/internal/service"
	"mediaflow/internal/upload"
)

// methodBasedAuth applies authentication middleware only to specific HTTP methods
func methodBasedAuth(authMiddleware func(http.Handler) http.Handler, handler http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			authMiddleware(http.HandlerFunc(handler)).ServeHTTP(w, r)
		} else {
			// No authentication for read methods (GET, HEAD, OPTIONS)
			handler(w, r)
		}
	})
}

func main() {
	cfg := config.Load()
	ctx := context.Background()
	utils.ProcessId <- os.Getpid()
	imageService := service.NewImageService(cfg)
	storageConfig, err := config.LoadStorageConfig(imageService.S3Client, cfg)
	if err != nil {
		log.Fatalf("🚨 Failed to load storage config: %v", err)
	}
	imageAPI := api.NewImageAPI(ctx, imageService, storageConfig)

	// Setup upload service and handlers
	uploadService := upload.NewService(imageService.S3Client, cfg)
	uploadHandler := upload.NewHandler(ctx, uploadService, storageConfig)

	// Setup authentication middleware
	authConfig := &auth.Config{APIKey: cfg.APIKey}
	authMiddleware := auth.APIKeyMiddleware(authConfig)

	mux := http.NewServeMux()

	// Image APIs
	mux.Handle("/thumb/{type}/{image_id}", methodBasedAuth(authMiddleware, imageAPI.HandleThumbnailTypes))
	mux.Handle("/originals/{type}/{image_id}", authMiddleware(http.HandlerFunc(imageAPI.HandleOriginals)))

	// Upload APIs (auth required)
	mux.Handle("/v1/uploads/presign", authMiddleware(http.HandlerFunc(uploadHandler.HandlePresign)))
	mux.HandleFunc("/v1/uploads/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/complete/") {
			authMiddleware(http.HandlerFunc(uploadHandler.HandleCompleteMultipart)).ServeHTTP(w, r)
		} else if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/abort/") {
			authMiddleware(http.HandlerFunc(uploadHandler.HandleAbortMultipart)).ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		response.JSON("OK").Write(w)
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("Starting server on port %s 🚀", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start 🚨: %v", err)
		}
	}()

	signal.Notify(utils.QuitChan, syscall.SIGINT, syscall.SIGTERM)
	<-utils.QuitChan

	log.Println("Shutting down server... 🛑")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown 🚨: %v", err)
	}

	log.Println("Server exited")
}
