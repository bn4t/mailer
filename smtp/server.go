package smtp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	stdmail "net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"
	"github.com/emersion/go-smtp"
	"mailer/models"
	"mailer/storage"
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
	raw, err := io.ReadAll(r)
	if err != nil {
		log.Printf("Error reading message: %v", err)
		return err
	}

	email, err := parseEmail(raw, s.from, s.to)
	if err != nil {
		log.Printf("Error parsing message: %v", err)
		return err
	}

	id := s.store.Save(email)
	log.Printf("Email received and stored with ID: %d (From: %s, Subject: %s)", id, email.From, email.Subject)

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

// parseEmail turns a raw RFC 5322 message into an Email. The raw message is
// retained so IMAP clients can fetch it exactly as it was received.
func parseEmail(raw []byte, envelopeFrom string, recipients []string) (*models.Email, error) {
	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer mr.Close()

	header := mr.Header

	subject, err := header.Subject()
	if err != nil {
		subject = decodeHeader(header.Get("Subject"))
	}

	from := envelopeFrom
	if addrs, err := header.AddressList("From"); err == nil && len(addrs) > 0 {
		from = formatAddress(addrs[0])
	} else if value := header.Get("From"); value != "" {
		from = decodeHeader(value)
	}

	date := time.Now()
	if parsed, err := header.Date(); err == nil && !parsed.IsZero() {
		date = parsed
	}

	body, htmlBody, inlineImages := extractBodies(mr)
	htmlBody = embedInlineImages(htmlBody, inlineImages)

	return &models.Email{
		From:       from,
		To:         recipients,
		Subject:    subject,
		Body:       body,
		HTMLBody:   htmlBody,
		Date:       date,
		RawHeaders: rawHeaderText(raw),
		ReceivedAt: time.Now(),
		Raw:        raw,
	}, nil
}

// extractBodies walks the MIME tree and returns the first plain-text and HTML
// alternatives plus any inline images that HTML refers to via Content-ID.
// Attachments are ignored.
func extractBodies(mr *gomail.Reader) (string, string, map[string]string) {
	var plainText, htmlText string
	inlineImages := make(map[string]string)

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Unknown charsets still yield a readable part; the body simply
			// stays in its original encoding. Other errors stop the walk.
			if part == nil || !message.IsUnknownCharset(err) {
				log.Printf("Error reading message part: %v", err)
				break
			}
		}
		if part == nil {
			continue
		}

		contentType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		contentType = strings.ToLower(contentType)
		if contentType == "" {
			contentType = "text/plain"
		}

		if _, ok := part.Header.(*gomail.InlineHeader); ok {
			switch contentType {
			case "text/plain":
				if plainText == "" {
					if data, err := io.ReadAll(part.Body); err == nil {
						plainText = string(data)
					}
				}
				continue
			case "text/html":
				if htmlText == "" {
					if data, err := io.ReadAll(part.Body); err == nil {
						htmlText = string(data)
					}
				}
				continue
			}
		}

		if !strings.HasPrefix(contentType, "image/") {
			continue
		}

		contentID := strings.Trim(strings.TrimSpace(part.Header.Get("Content-Id")), "<>")
		if contentID == "" {
			continue
		}
		if data, err := io.ReadAll(part.Body); err == nil {
			inlineImages[contentID] = fmt.Sprintf(
				"data:%s;base64,%s",
				contentType,
				base64.StdEncoding.EncodeToString(data),
			)
		}
	}

	return plainText, htmlText, inlineImages
}

// embedInlineImages replaces cid: references in HTML with data URLs so the
// preview renders embedded images without exposing separate attachment routes.
func embedInlineImages(html string, images map[string]string) string {
	for contentID, dataURL := range images {
		references := []string{
			"cid:" + contentID,
			"cid:" + url.QueryEscape(contentID),
		}
		for _, reference := range references {
			html = strings.ReplaceAll(html, reference, dataURL)
		}
	}
	return html
}

// rawHeaderText returns the original header block with normalized line endings
// for display. Unlike mail.Header it preserves the order the headers arrived in.
func rawHeaderText(raw []byte) string {
	var headers []byte
	if i := bytes.Index(raw, []byte("\r\n\r\n")); i >= 0 {
		headers = raw[:i]
	} else if i := bytes.Index(raw, []byte("\n\n")); i >= 0 {
		headers = raw[:i]
	} else {
		headers = raw
	}

	text := strings.ReplaceAll(string(headers), "\r\n", "\n")
	if text == "" {
		return text
	}
	return strings.TrimRight(text, "\n") + "\n"
}

// decodeHeader decodes RFC 2047 encoded words, falling back to the raw value.
func decodeHeader(value string) string {
	decoded, err := (&mime.WordDecoder{CharsetReader: message.CharsetReader}).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

// formatAddress renders an address in "Name <address>" form.
func formatAddress(addr *gomail.Address) string {
	if addr.Name == "" {
		return addr.Address
	}
	return fmt.Sprintf("%s <%s>", addr.Name, addr.Address)
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
	if parsed, err := stdmail.ParseAddress(addr); err == nil {
		return parsed.Address
	}
	return addr
}
