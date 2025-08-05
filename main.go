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
	"mediaflow/internal/config"
	"mediaflow/internal/service"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	utils.ProcessId <- os.Getpid()
	storageConfig, err := config.LoadStorageConfig()
	if err != nil {
		log.Fatalf("🚨 Failed to load storage config: %v", err)
	}
	imageService := service.NewImageService(cfg)
	imageAPI := api.NewImageAPI(ctx, imageService, storageConfig)

	mux := http.NewServeMux()

	// APIs
	mux.HandleFunc("/thumb/{type}/{image_id}", imageAPI.HandleThumbnailTypes)
	mux.HandleFunc("/originals/{type}/{image_id}", imageAPI.HandleOriginals)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
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
