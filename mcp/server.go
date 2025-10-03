package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mailer/models"
)

// Server provides MCP access to the mailer daemon
type Server struct {
	apiURL string
	client *http.Client
}

// NewServer creates a new MCP server that connects to the mailer daemon
func NewServer(apiURL string) *Server {
	return &Server{
		apiURL: apiURL,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// ListEmailsInput defines input for list_emails tool
type ListEmailsInput struct {
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Subject string `json:"subject,omitempty"`
}

// ListEmailsOutput defines output for list_emails tool
type ListEmailsOutput struct {
	Emails []EmailSummary `json:"emails"`
	Count  int            `json:"count"`
}

// EmailSummary provides a brief email summary
type EmailSummary struct {
	ID         int    `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Subject    string `json:"subject"`
	ReceivedAt string `json:"receivedAt"`
}

// GetEmailInput defines input for get_email tool
type GetEmailInput struct {
	ID int `json:"id"`
}

// GetEmailOutput defines output for get_email tool
type GetEmailOutput struct {
	Email *models.Email `json:"email"`
}

// SearchEmailsInput defines input for search_emails tool
type SearchEmailsInput struct {
	Query string `json:"query"`
}

// SearchEmailsOutput defines output for search_emails tool
type SearchEmailsOutput struct {
	Emails []EmailSummary `json:"emails"`
	Count  int            `json:"count"`
}

// StatsOutput defines output for get_stats tool
type StatsOutput struct {
	TotalEmails int    `json:"totalEmails"`
	SMTPAddr    string `json:"smtpAddr"`
	HTTPAddr    string `json:"httpAddr"`
}

// Run starts the MCP server
func (s *Server) Run(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mailer",
		Version: "1.0.0",
	}, nil)

	// Add resources
	server.AddResource(
		&mcp.Resource{
			URI:         "email://list",
			Name:        "Email List",
			Description: "List of all captured emails",
			MIMEType:    "application/json",
		},
		s.resourceEmailList,
	)

	// Add tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_emails",
		Description: "List all captured emails with optional filters",
	}, s.listEmails)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_email",
		Description: "Get full details of a specific email by ID",
	}, s.getEmail)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_emails",
		Description: "Search emails by content in subject or body",
	}, s.searchEmails)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_stats",
		Description: "Get email statistics and server configuration",
	}, s.getStats)

	// Run with stdio transport
	return server.Run(ctx, &mcp.StdioTransport{})
}

// resourceEmailList provides the email list resource
func (s *Server) resourceEmailList(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	emails, err := s.fetchAllEmails()
	if err != nil {
		return nil, err
	}

	data, err := json.MarshalIndent(emails, "", "  ")
	if err != nil {
		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      "email://list",
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

// listEmails tool implementation
func (s *Server) listEmails(ctx context.Context, req *mcp.CallToolRequest, input ListEmailsInput) (*mcp.CallToolResult, *ListEmailsOutput, error) {
	emails, err := s.fetchAllEmails()
	if err != nil {
		return nil, nil, err
	}

	// Apply filters
	filtered := make([]EmailSummary, 0)
	for _, email := range emails {
		if input.From != "" && !strings.Contains(strings.ToLower(email.From), strings.ToLower(input.From)) {
			continue
		}
		if input.To != "" && !strings.Contains(strings.ToLower(strings.Join(email.To, ",")), strings.ToLower(input.To)) {
			continue
		}
		if input.Subject != "" && !strings.Contains(strings.ToLower(email.Subject), strings.ToLower(input.Subject)) {
			continue
		}

		filtered = append(filtered, EmailSummary{
			ID:         email.ID,
			From:       email.From,
			To:         strings.Join(email.To, ", "),
			Subject:    email.Subject,
			ReceivedAt: email.ReceivedAt.Format(time.RFC3339),
		})
	}

	return nil, &ListEmailsOutput{
		Emails: filtered,
		Count:  len(filtered),
	}, nil
}

// getEmail tool implementation
func (s *Server) getEmail(ctx context.Context, req *mcp.CallToolRequest, input GetEmailInput) (*mcp.CallToolResult, *GetEmailOutput, error) {
	email, err := s.fetchEmailByID(input.ID)
	if err != nil {
		return nil, nil, err
	}

	return nil, &GetEmailOutput{Email: email}, nil
}

// searchEmails tool implementation
func (s *Server) searchEmails(ctx context.Context, req *mcp.CallToolRequest, input SearchEmailsInput) (*mcp.CallToolResult, *SearchEmailsOutput, error) {
	emails, err := s.fetchAllEmails()
	if err != nil {
		return nil, nil, err
	}

	query := strings.ToLower(input.Query)
	results := make([]EmailSummary, 0)

	for _, email := range emails {
		if strings.Contains(strings.ToLower(email.Subject), query) ||
			strings.Contains(strings.ToLower(email.Body), query) {
			results = append(results, EmailSummary{
				ID:         email.ID,
				From:       email.From,
				To:         strings.Join(email.To, ", "),
				Subject:    email.Subject,
				ReceivedAt: email.ReceivedAt.Format(time.RFC3339),
			})
		}
	}

	return nil, &SearchEmailsOutput{
		Emails: results,
		Count:  len(results),
	}, nil
}

// getStats tool implementation
func (s *Server) getStats(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, *StatsOutput, error) {
	emails, err := s.fetchAllEmails()
	if err != nil {
		return nil, nil, err
	}

	config, err := s.fetchConfig()
	if err != nil {
		return nil, nil, err
	}

	return nil, &StatsOutput{
		TotalEmails: len(emails),
		SMTPAddr:    config.SMTPAddr,
		HTTPAddr:    config.HTTPAddr,
	}, nil
}

// fetchAllEmails retrieves all emails from the daemon
func (s *Server) fetchAllEmails() ([]*models.Email, error) {
	resp, err := s.client.Get(s.apiURL + "/api/emails")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch emails: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var emails []*models.Email
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return nil, fmt.Errorf("failed to decode emails: %w", err)
	}

	return emails, nil
}

// fetchEmailByID retrieves a specific email from the daemon
func (s *Server) fetchEmailByID(id int) (*models.Email, error) {
	resp, err := s.client.Get(s.apiURL + "/api/emails/" + strconv.Itoa(id))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("email with ID %d not found", id)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var email models.Email
	if err := json.NewDecoder(resp.Body).Decode(&email); err != nil {
		return nil, fmt.Errorf("failed to decode email: %w", err)
	}

	return &email, nil
}

// Config represents server configuration
type Config struct {
	SMTPAddr string `json:"smtpAddr"`
	HTTPAddr string `json:"httpAddr"`
}

// fetchConfig retrieves server configuration from the daemon
func (s *Server) fetchConfig() (*Config, error) {
	resp, err := s.client.Get(s.apiURL + "/api/config")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var config Config
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return &config, nil
}
