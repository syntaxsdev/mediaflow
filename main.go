package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	// Image APIs (no auth required)
	mux.HandleFunc("/thumb/{type}/{image_id}", imageAPI.HandleThumbnailTypes)
	mux.HandleFunc("/originals/{type}/{image_id}", imageAPI.HandleOriginals)
	
	// Upload APIs (auth required)
	mux.Handle("/v1/uploads/presign", authMiddleware(http.HandlerFunc(uploadHandler.HandlePresign)))
	
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
