package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"mailer/api"
	imapserver "mailer/imap"
	mcpserver "mailer/mcp"
	"mailer/smtp"
	"mailer/storage"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	// Determine subcommand
	var command string
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		command = os.Args[1]
		os.Args = append(os.Args[:1], os.Args[2:]...)
	} else {
		command = "server"
	}

	switch command {
	case "mcp":
		runMCP()
	case "server":
		runServer()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		fmt.Fprintf(os.Stderr, "Usage: %s [server|mcp] [flags]\n", os.Args[0])
		os.Exit(1)
	}
}

func runMCP() {
	apiURL := flag.String("api-url", "http://localhost:8080", "Mailer daemon API URL")
	flag.Parse()

	server := mcpserver.NewServer(*apiURL)
	if err := server.Run(context.Background()); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}

func runServer() {
	// Parse command-line flags
	smtpAddr := flag.String("smtp-addr", ":2500", "SMTP server bind address (e.g., :2500 or 127.0.0.1:2500)")
	imapAddr := flag.String("imap-addr", ":1143", "IMAP server bind address (e.g., :1143 or 127.0.0.1:1143)")
	httpAddr := flag.String("http-addr", ":8080", "HTTP server bind address (e.g., :8080 or 127.0.0.1:8080)")
	flag.Parse()

	// Create storage
	store := storage.NewStore()

	// Setup HTTP server
	handler := api.NewHandler(store, *smtpAddr, *imapAddr, *httpAddr)
	httpServer := &http.Server{
		Addr:    *httpAddr,
		Handler: handler.SetupRoutes(),
	}

	// Start SMTP server in goroutine
	go func() {
		if err := smtp.StartServer(store, *smtpAddr); err != nil {
			log.Fatalf("SMTP server error: %v", err)
		}
	}()

	// Start IMAP server in goroutine
	go func() {
		if err := imapserver.StartServer(store, *imapAddr); err != nil {
			log.Fatalf("IMAP server error: %v", err)
		}
	}()

	// Start HTTP server in goroutine
	go func() {
		log.Printf("HTTP server starting on %s", *httpAddr)

		// Construct proper URL for browser
		browserURL := *httpAddr
		if browserURL[0] == ':' {
			browserURL = "localhost" + browserURL
		} else if len(browserURL) >= 7 && browserURL[:7] == "0.0.0.0" {
			browserURL = "localhost" + browserURL[7:]
		}
		log.Printf("Open http://%s in your browser", browserURL)

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
