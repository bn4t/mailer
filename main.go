package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"mailer/api"
	"mailer/smtp"
	"mailer/storage"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Parse command-line flags
	smtpPort := flag.Int("smtp-port", 2500, "SMTP server port")
	httpPort := flag.Int("http-port", 8080, "HTTP server port")
	flag.Parse()

	smtpAddr := fmt.Sprintf(":%d", *smtpPort)
	httpAddr := fmt.Sprintf(":%d", *httpPort)

	// Create storage
	store := storage.NewStore()

	// Setup HTTP server
	handler := api.NewHandler(store)
	httpServer := &http.Server{
		Addr:    httpAddr,
		Handler: handler.SetupRoutes(),
	}

	// Start SMTP server in goroutine
	go func() {
		if err := smtp.StartServer(store, smtpAddr); err != nil {
			log.Fatalf("SMTP server error: %v", err)
		}
	}()

	// Start HTTP server in goroutine
	go func() {
		log.Printf("HTTP server starting on %s", httpAddr)
		log.Printf("Open http://localhost%s in your browser", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down servers...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Servers stopped")
	fmt.Printf("\nCaptured %d email(s) during this session\n", store.Count())
}
