package imap

import (
	"bytes"
	"errors"
	"fmt"
	stdmail "net/mail"
	"net/textproto"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"mailer/models"
)

// Mailbox implements the IMAP mailbox interface
type Mailbox struct {
	name    string
	user    *User
	backend *Backend
}

// Name returns the mailbox name
func (m *Mailbox) Name() string {
	return m.name
}

// Info returns mailbox info
func (m *Mailbox) Info() (*imap.MailboxInfo, error) {
	info := &imap.MailboxInfo{
		Attributes: []string{},
		Delimiter:  "/",
		Name:       m.name,
	}
	return info, nil
}

// Status returns the mailbox status
func (m *Mailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	emails := m.backend.store.GetAll()

	status := imap.NewMailboxStatus(m.name, items)
	status.Flags = systemFlags()
	status.PermanentFlags = append(systemFlags(), imap.TryCreateFlag)
	status.Messages = uint32(len(emails))
	status.Recent = 0
	status.UidNext = uint32(m.backend.store.NextID())
	status.UidValidity = m.backend.uidValidity

	for i, email := range emails {
		if !m.user.hasFlag(email.ID, imap.SeenFlag) {
			status.Unseen++
			if status.UnseenSeqNum == 0 {
				status.UnseenSeqNum = uint32(i + 1)
			}
		}
	}

	return status, nil
}

// SetSubscribed sets the mailbox subscription status (not implemented)
func (m *Mailbox) SetSubscribed(subscribed bool) error {
	return nil
}

// Check is called when the client sends a CHECK command (not implemented)
func (m *Mailbox) Check() error {
	return nil
}

// ListMessages lists messages in the mailbox
func (m *Mailbox) ListMessages(uid bool, seqset *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	defer close(ch)

	emails := m.backend.store.GetAll()
	markSeen := fetchMarksSeen(items)

	for i, email := range emails {
		seqNum := uint32(i + 1)
		uidNum := uint32(email.ID)

		checkNum := seqNum
		if uid {
			checkNum = uidNum
		}
		if seqset != nil && !seqset.Contains(checkNum) {
			continue
		}

		if markSeen && !m.user.hasFlag(email.ID, imap.SeenFlag) {
			m.user.addFlags(email.ID, []string{imap.SeenFlag})
		}

		msg := imap.NewMessage(seqNum, items)
		for _, item := range items {
			switch item {
			case imap.FetchEnvelope:
				msg.Envelope = buildEnvelope(email)
			case imap.FetchBody, imap.FetchBodyStructure:
				msg.BodyStructure = buildBodyStructure(email)
			case imap.FetchFlags:
				msg.Flags = m.user.messageFlags(email.ID)
			case imap.FetchInternalDate:
				msg.InternalDate = email.ReceivedAt
			case imap.FetchRFC822Size:
				msg.Size = uint32(len(messageRaw(email)))
			case imap.FetchUid:
				msg.Uid = uidNum
			default:
				section, err := imap.ParseBodySectionName(item)
				if err != nil {
					continue
				}
				msg.Body[section] = bytes.NewReader(m.buildBody(email, section))
			}
		}

		ch <- msg
	}

	return nil
}

// buildEnvelope creates an IMAP envelope from an email
func buildEnvelope(email *models.Email) *imap.Envelope {
	from := parseAddress(email.From)

	envelope := &imap.Envelope{
		Date:    email.Date,
		Subject: email.Subject,
		To:      parseAddresses(email.To),
	}
	if from != nil {
		envelope.From = []*imap.Address{from}
		envelope.Sender = []*imap.Address{from}
	}

	return envelope
}

// buildBodyStructure creates a body structure matching the raw message
func buildBodyStructure(email *models.Email) *imap.BodyStructure {
	root, _, err := parseRawMessage(messageRaw(email))
	if err != nil {
		return &imap.BodyStructure{
			MIMEType:    "text",
			MIMESubType: "plain",
			Params:      map[string]string{"charset": "utf-8"},
			Encoding:    "8bit",
			Size:        uint32(len(email.Body)),
			Lines:       lineCount([]byte(email.Body)),
		}
	}

	return root.bodyStructure()
}

// SearchMessages searches messages using the supplied IMAP criteria
func (m *Mailbox) SearchMessages(uid bool, criteria *imap.SearchCriteria) ([]uint32, error) {
	emails := m.backend.store.GetAll()

	results := make([]uint32, 0, len(emails))
	for i, email := range emails {
		seqNum := uint32(i + 1)
		uidNum := uint32(email.ID)

		if m.matches(email, seqNum, uidNum, criteria) {
			if uid {
				results = append(results, uidNum)
			} else {
				results = append(results, seqNum)
			}
		}
	}

	return results, nil
}

func (m *Mailbox) matches(email *models.Email, seqNum, uidNum uint32, criteria *imap.SearchCriteria) bool {
	if criteria == nil {
		return true
	}

	if criteria.SeqNum != nil && !criteria.SeqNum.Contains(seqNum) {
		return false
	}
	if criteria.Uid != nil && !criteria.Uid.Contains(uidNum) {
		return false
	}

	if !criteria.Since.IsZero() && email.ReceivedAt.Before(criteria.Since) {
		return false
	}
	if !criteria.Before.IsZero() && !email.ReceivedAt.Before(criteria.Before) {
		return false
	}
	if !criteria.SentSince.IsZero() && email.Date.Before(criteria.SentSince) {
		return false
	}
	if !criteria.SentBefore.IsZero() && !email.Date.Before(criteria.SentBefore) {
		return false
	}

	flags := m.user.messageFlags(email.ID)
	flagSet := make(map[string]bool, len(flags))
	for _, flag := range flags {
		flagSet[flag] = true
	}
	for _, flag := range criteria.WithFlags {
		if !flagSet[imap.CanonicalFlag(flag)] {
			return false
		}
	}
	for _, flag := range criteria.WithoutFlags {
		if flagSet[imap.CanonicalFlag(flag)] {
			return false
		}
	}

	raw := messageRaw(email)
	if criteria.Larger > 0 && uint32(len(raw)) <= criteria.Larger {
		return false
	}
	if criteria.Smaller > 0 && uint32(len(raw)) >= criteria.Smaller {
		return false
	}

	headers, _ := parseHeaderBytes(rawHeaderBytes(raw))
	bodyText := strings.ToLower(email.Body + "\n" + email.HTMLBody)
	text := strings.ToLower(email.RawHeaders + "\n" + email.Body + "\n" + email.HTMLBody)

	for key, values := range criteria.Header {
		for _, value := range values {
			if !headerContains(headers, key, value) {
				return false
			}
		}
	}
	for _, value := range criteria.Body {
		if !strings.Contains(bodyText, strings.ToLower(value)) {
			return false
		}
	}
	for _, value := range criteria.Text {
		if !strings.Contains(text, strings.ToLower(value)) {
			return false
		}
	}

	for _, sub := range criteria.Not {
		if m.matches(email, seqNum, uidNum, sub) {
			return false
		}
	}
	for _, pair := range criteria.Or {
		if !m.matches(email, seqNum, uidNum, pair[0]) && !m.matches(email, seqNum, uidNum, pair[1]) {
			return false
		}
	}

	return true
}

func headerContains(headers textproto.MIMEHeader, key, value string) bool {
	values := headers[textproto.CanonicalMIMEHeaderKey(key)]
	if len(values) == 0 {
		return false
	}

	needle := strings.ToLower(value)
	for _, candidate := range values {
		if strings.Contains(strings.ToLower(candidate), needle) {
			return true
		}
	}
	return false
}

// CreateMessage creates a new message (not supported)
func (m *Mailbox) CreateMessage(flags []string, date time.Time, body imap.Literal) error {
	return errors.New("creating messages is not supported")
}

// UpdateMessagesFlags updates message flags (used for marking as deleted)
func (m *Mailbox) UpdateMessagesFlags(uid bool, seqset *imap.SeqSet, operation imap.FlagsOp, flags []string) error {
	emails := m.backend.store.GetAll()

	for i, email := range emails {
		seqNum := uint32(i + 1)
		uidNum := uint32(email.ID)

		checkNum := seqNum
		if uid {
			checkNum = uidNum
		}
		if seqset != nil && !seqset.Contains(checkNum) {
			continue
		}

		switch operation {
		case imap.AddFlags:
			m.user.addFlags(email.ID, flags)
		case imap.RemoveFlags:
			m.user.removeFlags(email.ID, flags)
		case imap.SetFlags:
			m.user.replaceFlags(email.ID, flags)
		}
	}

	return nil
}

// CopyMessages copies messages to another mailbox (not supported)
func (m *Mailbox) CopyMessages(uid bool, seqset *imap.SeqSet, dest string) error {
	return errors.New("copying messages is not supported")
}

// Expunge permanently removes messages marked as deleted
func (m *Mailbox) Expunge() error {
	emails := m.backend.store.GetAll()

	var deleted []int
	for _, email := range emails {
		if m.user.hasFlag(email.ID, imap.DeletedFlag) {
			deleted = append(deleted, email.ID)
		}
	}

	for _, id := range deleted {
		m.backend.store.Delete(id)
		m.user.replaceFlags(id, nil)
	}

	return nil
}

// buildBody returns the content of a body section.
func (m *Mailbox) buildBody(email *models.Email, section *imap.BodySectionName) []byte {
	raw := messageRaw(email)
	root, rawHeaders, err := parseRawMessage(raw)
	if err != nil {
		return nil
	}

	part := root
	if len(section.Path) > 0 {
		part = findPart(root, section.Path)
		if part == nil {
			return nil
		}
	}

	var data []byte
	switch section.Specifier {
	case imap.HeaderSpecifier:
		if len(section.Path) == 0 {
			data = headerSection(rawHeaders, root.header, section)
		} else if part.mediaType == "message" && part.mediaSubType == "rfc822" {
			inner, innerHeaders, err := parseRawMessage(part.body)
			if err != nil {
				return nil
			}
			data = headerSection(innerHeaders, inner.header, section)
		} else {
			data = part.headerBytes()
		}
	case imap.MIMESpecifier:
		data = part.headerBytes()
	case imap.TextSpecifier:
		if part.mediaType == "message" && part.mediaSubType == "rfc822" {
			_, innerBody := splitRawMessage(part.body)
			data = innerBody
		} else {
			data = part.body
		}
	default: // EntireSpecifier
		if len(section.Path) == 0 {
			data = raw
		} else {
			data = part.body
		}
	}

	return applyPartial(data, section.Partial)
}

// headerSection returns the requested header fields followed by a blank line.
func headerSection(rawHeaders []byte, header textproto.MIMEHeader, section *imap.BodySectionName) []byte {
	if len(section.Fields) == 0 && !section.NotFields {
		return appendHeaderTerminator(normalizeCRLF(rawHeaders))
	}
	return formatHeaderFields(header, section.Fields, section.NotFields)
}

func formatHeaderFields(header textproto.MIMEHeader, fields []string, notFields bool) []byte {
	selected := make(map[string]bool, len(fields))
	for _, field := range fields {
		selected[textproto.CanonicalMIMEHeaderKey(field)] = true
	}

	keys := make([]string, 0, len(header))
	for key := range header {
		include := len(fields) == 0
		if notFields {
			include = !selected[key]
		} else if !include {
			include = selected[key]
		}
		if include {
			keys = append(keys, key)
		}
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

func appendHeaderTerminator(header []byte) []byte {
	var buf bytes.Buffer
	buf.Write(header)
	if len(header) > 0 && !bytes.HasSuffix(header, []byte("\r\n")) {
		buf.WriteString("\r\n")
	}
	buf.WriteString("\r\n")
	return buf.Bytes()
}

func normalizeCRLF(data []byte) []byte {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n", "\r\n")
	return []byte(text)
}

func applyPartial(data []byte, partial []int) []byte {
	if len(partial) != 2 {
		return data
	}

	start, length := partial[0], partial[1]
	if start < 0 {
		start = 0
	}
	if start >= len(data) {
		return nil
	}

	data = data[start:]
	if length > 0 && len(data) > length {
		data = data[:length]
	}
	return data
}

// parseAddress parses an email address string into an IMAP address
func parseAddress(addr string) *imap.Address {
	if addr == "" {
		return nil
	}

	if parsed, err := stdmail.ParseAddress(addr); err == nil {
		local, domain, _ := strings.Cut(parsed.Address, "@")
		return &imap.Address{
			PersonalName: parsed.Name,
			MailboxName:  local,
			HostName:     domain,
		}
	}

	local, domain, _ := strings.Cut(addr, "@")
	return &imap.Address{
		MailboxName: local,
		HostName:    domain,
	}
}

// parseAddresses parses multiple email addresses
func parseAddresses(addrs []string) []*imap.Address {
	result := make([]*imap.Address, 0, len(addrs))
	for _, addr := range addrs {
		if parsed := parseAddress(addr); parsed != nil {
			result = append(result, parsed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func systemFlags() []string {
	return []string{
		imap.SeenFlag,
		imap.AnsweredFlag,
		imap.FlaggedFlag,
		imap.DeletedFlag,
		imap.DraftFlag,
	}
}

// fetchMarksSeen reports whether a fetch implicitly marks messages as \Seen.
func fetchMarksSeen(items []imap.FetchItem) bool {
	for _, item := range items {
		section, err := imap.ParseBodySectionName(item)
		if err != nil {
			continue
		}
		if !section.Peek {
			return true
		}
	}
	return false
}
