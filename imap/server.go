package imap

import (
	"log"

	"github.com/emersion/go-imap/server"
	"mailer/storage"
)

// StartServer starts the IMAP server
func StartServer(store *storage.Store, addr string) error {
	// Create backend
	be := NewBackend(store)

	// Create server
	s := server.New(be)
	s.Addr = addr

	// Allow insecure auth for development
	// In production, you should use TLS
	s.AllowInsecureAuth = true

	log.Printf("IMAP server starting on %s", addr)
	log.Printf("IMAP: Use any username/password to login")

	if err := s.ListenAndServe(); err != nil {
		return err
	}

	return nil
}
