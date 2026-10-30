package imap

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"mailer/storage"
)

// Backend implements the IMAP backend interface
type Backend struct {
	store       *storage.Store
	uidValidity uint32

	mu    sync.Mutex
	flags map[int]map[string]bool // Message flags, keyed by UID (email ID)
}

// NewBackend creates a new IMAP backend
func NewBackend(store *storage.Store) *Backend {
	// Emails are kept in memory, so a restart starts an entirely new mailbox.
	// A fresh UIDVALIDITY tells clients to discard cached UIDs.
	validity := uint32(time.Now().Unix())
	if validity == 0 {
		validity = 1
	}

	return &Backend{
		store:       store,
		uidValidity: validity,
		flags:       make(map[int]map[string]bool),
	}
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
		name:    "INBOX",
		user:    u,
		backend: u.backend,
	}
	return []backend.Mailbox{mailbox}, nil
}

// GetMailbox returns a mailbox by name
func (u *User) GetMailbox(name string) (backend.Mailbox, error) {
	if name != "INBOX" {
		return nil, errors.New("mailbox not found")
	}

	return &Mailbox{
		name:    name,
		user:    u,
		backend: u.backend,
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

// messageFlags returns the flags currently set on a message.
func (u *User) messageFlags(id int) []string {
	u.backend.mu.Lock()
	defer u.backend.mu.Unlock()

	return flagList(u.backend.flags[id])
}

// hasFlag reports whether a message has a flag set.
func (u *User) hasFlag(id int, flag string) bool {
	u.backend.mu.Lock()
	defer u.backend.mu.Unlock()

	return u.backend.flags[id][imap.CanonicalFlag(flag)]
}

// addFlags sets one or more flags on a message.
func (u *User) addFlags(id int, flags []string) {
	u.backend.mu.Lock()
	defer u.backend.mu.Unlock()

	set := u.backend.flags[id]
	if set == nil {
		set = make(map[string]bool)
		u.backend.flags[id] = set
	}
	for _, flag := range flags {
		if flag == imap.RecentFlag {
			continue
		}
		set[imap.CanonicalFlag(flag)] = true
	}
}

// removeFlags clears one or more flags from a message.
func (u *User) removeFlags(id int, flags []string) {
	u.backend.mu.Lock()
	defer u.backend.mu.Unlock()

	set := u.backend.flags[id]
	for _, flag := range flags {
		delete(set, imap.CanonicalFlag(flag))
	}
	if len(set) == 0 {
		delete(u.backend.flags, id)
	}
}

// replaceFlags replaces all flags on a message.
func (u *User) replaceFlags(id int, flags []string) {
	u.backend.mu.Lock()
	defer u.backend.mu.Unlock()

	set := make(map[string]bool, len(flags))
	for _, flag := range flags {
		if flag == imap.RecentFlag {
			continue
		}
		set[imap.CanonicalFlag(flag)] = true
	}
	if len(set) == 0 {
		delete(u.backend.flags, id)
	} else {
		u.backend.flags[id] = set
	}
}

// flagList converts a flag set into a stable list.
func flagList(set map[string]bool) []string {
	flags := make([]string, 0, len(set))
	for flag := range set {
		flags = append(flags, flag)
	}
	sort.Strings(flags)
	return flags
}
