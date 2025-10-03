package imap

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/emersion/go-imap"
	"mailer/models"
)

// Mailbox implements the IMAP mailbox interface
type Mailbox struct {
	name           string
	user           *User
	backend        *Backend
	deletedFlags   map[uint32]bool // Track which messages are marked for deletion
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
	status.Flags = []string{imap.SeenFlag, imap.DeletedFlag}
	status.PermanentFlags = []string{imap.SeenFlag, imap.DeletedFlag}
	status.UnseenSeqNum = 0

	for _, item := range items {
		switch item {
		case imap.StatusMessages:
			status.Messages = uint32(len(emails))
		case imap.StatusUidNext:
			status.UidNext = uint32(len(emails) + 1)
		case imap.StatusUidValidity:
			status.UidValidity = 1
		case imap.StatusRecent:
			status.Recent = 0
		case imap.StatusUnseen:
			status.Unseen = 0
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

	for i, email := range emails {
		seqNum := uint32(i + 1)
		uidNum := uint32(email.ID)

		// Check if this message is in the requested sequence set
		if !seqset.Contains(seqNum) {
			continue
		}

		msg := imap.NewMessage(seqNum, items)
		for _, item := range items {
			switch item {
			case imap.FetchEnvelope:
				msg.Envelope = m.buildEnvelope(email)
			case imap.FetchBody, imap.FetchBodyStructure:
				msg.BodyStructure = m.buildBodyStructure(email)
			case imap.FetchFlags:
				msg.Flags = []string{}
				if m.deletedFlags[uidNum] {
					msg.Flags = append(msg.Flags, imap.DeletedFlag)
				}
			case imap.FetchInternalDate:
				msg.InternalDate = email.ReceivedAt
			case imap.FetchRFC822Size:
				msg.Size = uint32(len(email.Body))
			case imap.FetchUid:
				msg.Uid = uidNum
			default:
				// Handle BODY[] and BODY[HEADER] requests
				section, err := imap.ParseBodySectionName(item)
				if err != nil {
					continue
				}
				msg.Body[section] = m.buildBody(email, section)
			}
		}

		ch <- msg
	}

	return nil
}

// buildEnvelope creates an IMAP envelope from an email
func (m *Mailbox) buildEnvelope(email *models.Email) *imap.Envelope {
	return &imap.Envelope{
		Date:    email.Date,
		Subject: email.Subject,
		From:    []*imap.Address{parseAddress(email.From)},
		To:      parseAddresses(email.To),
		Sender:  []*imap.Address{parseAddress(email.From)},
	}
}

// buildBodyStructure creates a body structure for an email
func (m *Mailbox) buildBodyStructure(email *models.Email) *imap.BodyStructure {
	if email.HTMLBody != "" {
		// Multipart message with text and HTML
		return &imap.BodyStructure{
			MIMEType:    "multipart",
			MIMESubType: "alternative",
			Parts: []*imap.BodyStructure{
				{
					MIMEType:    "text",
					MIMESubType: "plain",
					Params:      map[string]string{"charset": "utf-8"},
					Size:        uint32(len(email.Body)),
				},
				{
					MIMEType:    "text",
					MIMESubType: "html",
					Params:      map[string]string{"charset": "utf-8"},
					Size:        uint32(len(email.HTMLBody)),
				},
			},
		}
	}

	// Plain text only
	return &imap.BodyStructure{
		MIMEType:    "text",
		MIMESubType: "plain",
		Params:      map[string]string{"charset": "utf-8"},
		Size:        uint32(len(email.Body)),
	}
}

// buildBody creates the body content for an email
func (m *Mailbox) buildBody(email *models.Email, section *imap.BodySectionName) imap.Literal {
	var buf bytes.Buffer

	if section.Specifier == imap.HeaderSpecifier {
		// Return headers
		fmt.Fprintf(&buf, "From: %s\r\n", email.From)
		fmt.Fprintf(&buf, "To: %s\r\n", email.To[0])
		fmt.Fprintf(&buf, "Subject: %s\r\n", email.Subject)
		fmt.Fprintf(&buf, "Date: %s\r\n", email.Date.Format(time.RFC1123Z))
		buf.WriteString("\r\n")
	} else {
		// Return full message
		fmt.Fprintf(&buf, "From: %s\r\n", email.From)
		fmt.Fprintf(&buf, "To: %s\r\n", email.To[0])
		fmt.Fprintf(&buf, "Subject: %s\r\n", email.Subject)
		fmt.Fprintf(&buf, "Date: %s\r\n", email.Date.Format(time.RFC1123Z))
		buf.WriteString("\r\n")

		if email.HTMLBody != "" {
			buf.WriteString(email.HTMLBody)
		} else {
			buf.WriteString(email.Body)
		}
	}

	return bytes.NewReader(buf.Bytes())
}

// SearchMessages searches for messages
func (m *Mailbox) SearchMessages(uid bool, criteria *imap.SearchCriteria) ([]uint32, error) {
	emails := m.backend.store.GetAll()

	// For simplicity, return all message sequence numbers
	// A full implementation would filter based on criteria
	results := make([]uint32, 0, len(emails))

	for i, email := range emails {
		seqNum := uint32(i + 1)
		uidNum := uint32(email.ID)

		if uid {
			results = append(results, uidNum)
		} else {
			results = append(results, seqNum)
		}
	}

	return results, nil
}

// CreateMessage creates a new message (not supported)
func (m *Mailbox) CreateMessage(flags []string, date time.Time, body imap.Literal) error {
	return errors.New("creating messages is not supported")
}

// UpdateMessagesFlags updates message flags (used for marking as deleted)
func (m *Mailbox) UpdateMessagesFlags(uid bool, seqset *imap.SeqSet, operation imap.FlagsOp, flags []string) error {
	emails := m.backend.store.GetAll()

	// Check if we're setting the Deleted flag
	hasDeletedFlag := false
	for _, flag := range flags {
		if flag == imap.DeletedFlag {
			hasDeletedFlag = true
			break
		}
	}

	if !hasDeletedFlag {
		return nil
	}

	// Mark messages as deleted
	for i, email := range emails {
		seqNum := uint32(i + 1)

		if seqset.Contains(seqNum) {
			if operation == imap.AddFlags || operation == imap.SetFlags {
				m.deletedFlags[uint32(email.ID)] = true
			} else if operation == imap.RemoveFlags {
				delete(m.deletedFlags, uint32(email.ID))
			}
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
	// Delete all messages marked for deletion
	for emailID := range m.deletedFlags {
		m.backend.store.Delete(int(emailID))
	}

	// Clear the deleted flags map
	m.deletedFlags = make(map[uint32]bool)

	return nil
}

// parseAddress parses an email address string into an IMAP address
func parseAddress(addr string) *imap.Address {
	return &imap.Address{
		MailboxName: addr,
		HostName:    "",
	}
}

// parseAddresses parses multiple email addresses
func parseAddresses(addrs []string) []*imap.Address {
	result := make([]*imap.Address, len(addrs))
	for i, addr := range addrs {
		result[i] = parseAddress(addr)
	}
	return result
}
