package api

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"mailer/storage"
	"net/http"
	"strconv"
	"strings"
)

//go:embed web/*
var webFS embed.FS

// Handler provides HTTP handlers for the API
type Handler struct {
	store    *storage.Store
	smtpAddr string
	imapAddr string
	httpAddr string
}

// NewHandler creates a new API handler
func NewHandler(store *storage.Store, smtpAddr string, imapAddr string, httpAddr string) *Handler {
	return &Handler{
		store:    store,
		smtpAddr: smtpAddr,
		imapAddr: imapAddr,
		httpAddr: httpAddr,
	}
}

// SetupRoutes configures all HTTP routes
func (h *Handler) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/config", h.handleConfig)
	mux.HandleFunc("/api/emails", h.handleEmails)
	mux.HandleFunc("/api/emails/", h.handleEmailByID)

	// Static files from embedded filesystem
	webContent, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(webContent)))

	return h.corsMiddleware(mux)
}

// handleConfig returns server configuration
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config := map[string]interface{}{
		"smtpAddr": h.smtpAddr,
		"imapAddr": h.imapAddr,
		"httpAddr": h.httpAddr,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// handleEmails handles GET (list all) and DELETE (delete all)
func (h *Handler) handleEmails(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listEmails(w, r)
	case http.MethodDelete:
		h.deleteAllEmails(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleEmailByID handles GET (single email) and DELETE (single email)
func (h *Handler) handleEmailByID(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/emails/")
	id, err := strconv.Atoi(path)
	if err != nil {
		http.Error(w, "Invalid email ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getEmail(w, r, id)
	case http.MethodDelete:
		h.deleteEmail(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listEmails returns all emails
func (h *Handler) listEmails(w http.ResponseWriter, r *http.Request) {
	emails := h.store.GetAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(emails)
}

// getEmail returns a specific email by ID
func (h *Handler) getEmail(w http.ResponseWriter, r *http.Request, id int) {
	email, exists := h.store.GetByID(id)
	if !exists {
		http.Error(w, "Email not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(email)
}

// deleteEmail deletes a specific email
func (h *Handler) deleteEmail(w http.ResponseWriter, r *http.Request, id int) {
	if h.store.Delete(id) {
		w.WriteHeader(http.StatusNoContent)
		log.Printf("Email %d deleted", id)
	} else {
		http.Error(w, "Email not found", http.StatusNotFound)
	}
}

// deleteAllEmails deletes all emails
func (h *Handler) deleteAllEmails(w http.ResponseWriter, r *http.Request) {
	h.store.DeleteAll()
	w.WriteHeader(http.StatusNoContent)
	log.Printf("All emails deleted")
}

// corsMiddleware adds CORS headers
func (h *Handler) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
