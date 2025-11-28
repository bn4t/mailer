package storage

import (
	"mailer/models"
	"sort"
	"sync"
)

// Store manages email storage in memory
type Store struct {
	mu      sync.RWMutex
	emails  map[int]*models.Email
	nextID  int
}

// NewStore creates a new email store
func NewStore() *Store {
	return &Store{
		emails: make(map[int]*models.Email),
		nextID: 1,
	}
}

// Save stores a new email and returns its ID
func (s *Store) Save(email *models.Email) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	email.ID = s.nextID
	s.emails[s.nextID] = email
	s.nextID++

	return email.ID
}

// GetAll returns all stored emails sorted by ID for consistent ordering
func (s *Store) GetAll() []*models.Email {
	s.mu.RLock()
	defer s.mu.RUnlock()

	emails := make([]*models.Email, 0, len(s.emails))
	for _, email := range s.emails {
		emails = append(emails, email)
	}

	// Sort by ID to ensure consistent sequence numbers (required for IMAP)
	sort.Slice(emails, func(i, j int) bool {
		return emails[i].ID < emails[j].ID
	})

	return emails
}

// GetByID returns a specific email by ID
func (s *Store) GetByID(id int) (*models.Email, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	email, exists := s.emails[id]
	return email, exists
}

// Delete removes an email by ID
func (s *Store) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.emails[id]; exists {
		delete(s.emails, id)
		return true
	}
	return false
}

// DeleteAll removes all emails
func (s *Store) DeleteAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.emails = make(map[int]*models.Email)
	s.nextID = 1
}

// Count returns the number of stored emails
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.emails)
}
