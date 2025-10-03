package smtp

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mailer/models"
	"mailer/storage"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
)

// Backend implements SMTP server backend
type Backend struct {
	store *storage.Store
}

// NewBackend creates a new SMTP backend
func NewBackend(store *storage.Store) *Backend {
	return &Backend{store: store}
}

// NewSession creates a new SMTP session
func (b *Backend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &Session{store: b.store}, nil
}

// Session represents an SMTP session
type Session struct {
	store *storage.Store
	from  string
	to    []string
}

// AuthPlain handles PLAIN authentication (accept all)
func (s *Session) AuthPlain(username, password string) error {
	return nil
}

// Mail sets the sender
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	return nil
}

// Rcpt adds a recipient
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

// Data receives the email data
func (s *Session) Data(r io.Reader) error {
	// Parse the email
	msg, err := mail.ReadMessage(r)
	if err != nil {
		log.Printf("Error reading message: %v", err)
		return err
	}

	// Extract headers
	subject := msg.Header.Get("Subject")
	date := msg.Header.Get("Date")
	from := msg.Header.Get("From")
	if from == "" {
		from = s.from
	}

	// Parse date
	parsedDate := time.Now()
	if date != "" {
		if t, err := mail.ParseDate(date); err == nil {
			parsedDate = t
		}
	}

	// Extract body
	body, htmlBody := extractBody(msg)

	// Store raw headers
	rawHeaders := formatHeaders(msg.Header)

	// Create email object
	email := &models.Email{
		From:       from,
		To:         s.to,
		Subject:    subject,
		Body:       body,
		HTMLBody:   htmlBody,
		Date:       parsedDate,
		RawHeaders: rawHeaders,
		ReceivedAt: time.Now(),
	}

	// Save to store
	id := s.store.Save(email)
	log.Printf("Email received and stored with ID: %d (From: %s, Subject: %s)", id, from, subject)

	return nil
}

// Reset resets the session state
func (s *Session) Reset() {
	s.from = ""
	s.to = nil
}

// Logout ends the session
func (s *Session) Logout() error {
	return nil
}

// extractBody extracts plain text and HTML body from message
func extractBody(msg *mail.Message) (string, string) {
	contentType := msg.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		// Simple text body
		body, _ := io.ReadAll(msg.Body)
		decoded := decodeBody(body, msg.Header.Get("Content-Transfer-Encoding"))
		return decoded, ""
	}

	var plainText, htmlText string

	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(msg.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("Error reading part: %v", err)
				break
			}

			partType := p.Header.Get("Content-Type")
			partMedia, _, _ := mime.ParseMediaType(partType)
			encoding := p.Header.Get("Content-Transfer-Encoding")

			body, _ := io.ReadAll(p)
			bodyStr := decodeBody(body, encoding)

			if strings.HasPrefix(partMedia, "text/plain") {
				plainText = bodyStr
			} else if strings.HasPrefix(partMedia, "text/html") {
				htmlText = bodyStr
			}
		}
	} else if strings.HasPrefix(mediaType, "text/plain") {
		body, _ := io.ReadAll(msg.Body)
		plainText = decodeBody(body, msg.Header.Get("Content-Transfer-Encoding"))
	} else if strings.HasPrefix(mediaType, "text/html") {
		body, _ := io.ReadAll(msg.Body)
		htmlText = decodeBody(body, msg.Header.Get("Content-Transfer-Encoding"))
	} else {
		body, _ := io.ReadAll(msg.Body)
		plainText = decodeBody(body, msg.Header.Get("Content-Transfer-Encoding"))
	}

	return plainText, htmlText
}

// decodeBody decodes the body based on Content-Transfer-Encoding
func decodeBody(body []byte, encoding string) string {
	encoding = strings.ToLower(strings.TrimSpace(encoding))

	switch encoding {
	case "quoted-printable":
		r := quotedprintable.NewReader(strings.NewReader(string(body)))
		decoded, err := io.ReadAll(r)
		if err != nil {
			log.Printf("Error decoding quoted-printable: %v", err)
			return string(body)
		}
		return string(decoded)

	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(string(body))
		if err != nil {
			log.Printf("Error decoding base64: %v", err)
			return string(body)
		}
		return string(decoded)

	default:
		// No encoding or 7bit/8bit - return as-is
		return string(body)
	}
}

// formatHeaders formats email headers as a string
func formatHeaders(header mail.Header) string {
	var sb strings.Builder
	for key, values := range header {
		for _, value := range values {
			sb.WriteString(fmt.Sprintf("%s: %s\n", key, value))
		}
	}
	return sb.String()
}

// StartServer starts the SMTP server
func StartServer(store *storage.Store, addr string) error {
	be := NewBackend(store)
	s := smtp.NewServer(be)

	s.Addr = addr
	s.Domain = "localhost"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 10 * 1024 * 1024 // 10MB
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true

	log.Printf("SMTP server starting on %s", addr)
	return s.ListenAndServe()
}

// ParseEmailAddress extracts email from address (handles "Name <email>" format)
func ParseEmailAddress(addr string) string {
	if parsed, err := mail.ParseAddress(addr); err == nil {
		return parsed.Address
	}
	return addr
}
