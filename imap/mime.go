package imap

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"mailer/models"
)

// mimePart is a single entity in a message's MIME tree. body contains the
// entity body exactly as transmitted, including its transfer encoding.
type mimePart struct {
	header            textproto.MIMEHeader
	mediaType         string
	mediaSubType      string
	params            map[string]string
	encoding          string
	disposition       string
	dispositionParams map[string]string
	body              []byte
	children          []*mimePart
}

// parseRawMessage parses a full message into a MIME tree. It returns the tree
// and the raw header block.
func parseRawMessage(raw []byte) (*mimePart, []byte, error) {
	if len(raw) == 0 {
		return nil, nil, errors.New("empty message")
	}

	headerBytes, body := splitRawMessage(raw)
	header, err := parseHeaderBytes(headerBytes)
	if err != nil && len(header) == 0 {
		header = make(textproto.MIMEHeader)
	}

	return parseMIMEPart(header, body), headerBytes, nil
}

// parseMIMEPart recursively parses a MIME entity.
func parseMIMEPart(header textproto.MIMEHeader, body []byte) *mimePart {
	part := &mimePart{
		header: header,
		body:   body,
	}

	contentType := header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" {
		mediaType = "text/plain"
		params = nil
	}

	part.mediaType, part.mediaSubType, _ = strings.Cut(strings.ToLower(mediaType), "/")
	part.params = params

	part.encoding = strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding")))
	if part.encoding == "" {
		part.encoding = "7bit"
	}

	disposition, dispositionParams, err := mime.ParseMediaType(header.Get("Content-Disposition"))
	if err == nil && disposition != "" {
		part.disposition = strings.ToLower(disposition)
		part.dispositionParams = dispositionParams
	}

	if part.mediaType == "multipart" && params["boundary"] != "" {
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			child, err := reader.NextRawPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}

			childBody, err := io.ReadAll(child)
			if err != nil {
				break
			}
			part.children = append(part.children, parseMIMEPart(child.Header, childBody))
		}
	}

	// A multipart declaration without a usable boundary or parts cannot be
	// represented as an IMAP multipart body structure. Treat its content as
	// plain text instead of emitting an invalid structure.
	if part.mediaType == "multipart" && len(part.children) == 0 {
		part.mediaType = "text"
		part.mediaSubType = "plain"
		part.params = nil
		part.encoding = "7bit"
	}

	return part
}

// bodyStructure converts a MIME part into an IMAP body structure.
func (p *mimePart) bodyStructure() *imap.BodyStructure {
	structure := &imap.BodyStructure{
		MIMEType:          p.mediaType,
		MIMESubType:       p.mediaSubType,
		Params:            p.params,
		Id:                p.header.Get("Content-Id"),
		Description:       p.header.Get("Content-Description"),
		Encoding:          p.encoding,
		Size:              uint32(len(p.body)),
		Disposition:       p.disposition,
		DispositionParams: p.dispositionParams,
	}

	if p.mediaType == "multipart" {
		for _, child := range p.children {
			structure.Parts = append(structure.Parts, child.bodyStructure())
		}
		return structure
	}

	if p.mediaType == "text" {
		structure.Lines = lineCount(p.body)
	}

	if p.mediaType == "message" && p.mediaSubType == "rfc822" {
		structure.Lines = lineCount(p.body)
		if inner, _, err := parseRawMessage(p.body); err == nil {
			structure.BodyStructure = inner.bodyStructure()
		}
	}

	return structure
}

// headerBytes renders the part's MIME header followed by a blank line.
func (p *mimePart) headerBytes() []byte {
	return formatMIMEHeader(p.header)
}

func formatMIMEHeader(header textproto.MIMEHeader) []byte {
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	for _, key := range keys {
		for _, value := range header[key] {
			fmt.Fprintf(&buf, "%s: %s\r\n", key, value)
		}
	}
	buf.WriteString("\r\n")
	return buf.Bytes()
}

func parseHeaderBytes(header []byte) (textproto.MIMEHeader, error) {
	var buf bytes.Buffer
	buf.Write(header)
	buf.WriteString("\r\n\r\n")
	return textproto.NewReader(bufio.NewReader(&buf)).ReadMIMEHeader()
}

// splitRawMessage splits a message into its header block and body.
func splitRawMessage(raw []byte) ([]byte, []byte) {
	if i := bytes.Index(raw, []byte("\r\n\r\n")); i >= 0 {
		return raw[:i], raw[i+4:]
	}
	if i := bytes.Index(raw, []byte("\n\n")); i >= 0 {
		return raw[:i], raw[i+2:]
	}
	return raw, nil
}

func rawHeaderBytes(raw []byte) []byte {
	header, _ := splitRawMessage(raw)
	return header
}

// messageRaw returns the original message, or a best-effort reconstruction for
// emails created without one.
func messageRaw(email *models.Email) []byte {
	if len(email.Raw) > 0 {
		return email.Raw
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", email.From)
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(email.To, ", "))
	fmt.Fprintf(&buf, "Subject: %s\r\n", email.Subject)
	fmt.Fprintf(&buf, "Date: %s\r\n", email.Date.Format(time.RFC1123Z))

	switch {
	case email.Body != "" && email.HTMLBody != "":
		const boundary = "mailer-boundary"
		fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
		fmt.Fprintf(&buf, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", boundary, email.Body)
		fmt.Fprintf(&buf, "--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n", boundary, email.HTMLBody)
		fmt.Fprintf(&buf, "--%s--\r\n", boundary)
	case email.HTMLBody != "":
		fmt.Fprintf(&buf, "Content-Type: text/html; charset=utf-8\r\n\r\n%s", email.HTMLBody)
	default:
		fmt.Fprintf(&buf, "Content-Type: text/plain; charset=utf-8\r\n\r\n%s", email.Body)
	}

	return buf.Bytes()
}

func findPart(root *mimePart, path []int) *mimePart {
	if len(path) == 0 {
		return root
	}
	// Non-multipart messages only have part 1, which is the message itself.
	if len(root.children) == 0 && len(path) == 1 && path[0] == 1 {
		return root
	}

	part := root
	for _, index := range path {
		if index < 1 || index > len(part.children) {
			return nil
		}
		part = part.children[index-1]
	}
	return part
}

func lineCount(body []byte) uint32 {
	if len(body) == 0 {
		return 0
	}

	lines := bytes.Count(body, []byte("\n"))
	if body[len(body)-1] != '\n' {
		lines++
	}
	return uint32(lines)
}
