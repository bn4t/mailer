package imap

import (
	"errors"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"mailer/storage"
)

// Backend implements the IMAP backend interface
type Backend struct {
	store *storage.Store
}

// NewBackend creates a new IMAP backend
func NewBackend(store *storage.Store) *Backend {
	return &Backend{store: store}
}

// Login authenticates a user
// For testing purposes, we accept any username/password combination
func (b *Backend) Login(_ *imap.ConnInfo, username, password string) (backend.User, error) {
	// For development/testing, accept any credentials
	// In production, you would validate credentials here
	return &User{
		username: username,
		backend:  b,
	}, nil
}

// User implements the IMAP user interface
type User struct {
	username string
	backend  *Backend
}

// Username returns the username
func (u *User) Username() string {
	return u.username
}

// ListMailboxes returns a list of mailboxes
// We only have one mailbox: INBOX
func (u *User) ListMailboxes(subscribed bool) ([]backend.Mailbox, error) {
	mailbox := &Mailbox{
		name:         "INBOX",
		user:         u,
		backend:      u.backend,
		deletedFlags: make(map[uint32]bool),
	}
	return []backend.Mailbox{mailbox}, nil
}

// GetMailbox returns a mailbox by name
func (u *User) GetMailbox(name string) (backend.Mailbox, error) {
	if name != "INBOX" {
		return nil, errors.New("mailbox not found")
	}

	return &Mailbox{
		name:         name,
		user:         u,
		backend:      u.backend,
		deletedFlags: make(map[uint32]bool),
	}, nil
}

// CreateMailbox creates a new mailbox (not supported)
func (u *User) CreateMailbox(name string) error {
	return errors.New("creating mailboxes is not supported")
}

// DeleteMailbox deletes a mailbox (not supported)
func (u *User) DeleteMailbox(name string) error {
	return errors.New("deleting mailboxes is not supported")
}

// RenameMailbox renames a mailbox (not supported)
func (u *User) RenameMailbox(existingName, newName string) error {
	return errors.New("renaming mailboxes is not supported")
}

// Logout is called when the user logs out
func (u *User) Logout() error {
	return nil
}
