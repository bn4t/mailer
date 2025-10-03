package models

import "time"

// Email represents a captured email message
type Email struct {
	ID          int       `json:"id"`
	From        string    `json:"from"`
	To          []string  `json:"to"`
	Subject     string    `json:"subject"`
	Body        string    `json:"body"`
	HTMLBody    string    `json:"htmlBody"`
	Date        time.Time `json:"date"`
	RawHeaders  string    `json:"rawHeaders"`
	ReceivedAt  time.Time `json:"receivedAt"`
}
