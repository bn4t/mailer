package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mailer/models"
)

func newTestServer(t *testing.T, emails []*models.Email) *Server {
	t.Helper()

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(emails); err != nil {
			t.Errorf("encode emails: %v", err)
		}
	}))
	t.Cleanup(daemon.Close)

	return NewServer(daemon.URL)
}

func TestSearchEmailsIncludesHTMLBody(t *testing.T) {
	server := newTestServer(t, []*models.Email{
		{ID: 1, Subject: "newsletter", HTMLBody: "<p>release secret</p>"},
		{ID: 2, Subject: "plain", Body: "release secret"},
		{ID: 3, Subject: "unrelated", Body: "nothing here"},
	})

	_, output, err := server.searchEmails(context.Background(), nil, SearchEmailsInput{Query: "SECRET"})
	if err != nil {
		t.Fatalf("searchEmails returned error: %v", err)
	}
	if output.Count != 2 {
		t.Fatalf("count = %d, want 2", output.Count)
	}
	if output.Emails[0].ID != 1 || output.Emails[1].ID != 2 {
		t.Errorf("search returned IDs %d, %d, want 1, 2", output.Emails[0].ID, output.Emails[1].ID)
	}
}

func TestListEmailsPagination(t *testing.T) {
	server := newTestServer(t, []*models.Email{
		{ID: 1, Subject: "first"},
		{ID: 2, Subject: "second"},
		{ID: 3, Subject: "third"},
	})

	_, output, err := server.listEmails(context.Background(), nil, ListEmailsInput{Offset: 1, Limit: 1})
	if err != nil {
		t.Fatalf("listEmails returned error: %v", err)
	}
	if output.Count != 1 || output.TotalCount != 3 || !output.HasMore {
		t.Errorf("pagination = count %d, total %d, hasMore %v; want 1, 3, true", output.Count, output.TotalCount, output.HasMore)
	}
	if output.Emails[0].ID != 2 {
		t.Errorf("paged email ID = %d, want 2", output.Emails[0].ID)
	}
}

func TestListEmailsFilters(t *testing.T) {
	server := newTestServer(t, []*models.Email{
		{ID: 1, From: "Alice <alice@example.com>", To: []string{"bob@example.com"}, Subject: "Hello"},
		{ID: 2, From: "carol@example.com", To: []string{"dave@example.com"}, Subject: "Other"},
	})

	_, output, err := server.listEmails(context.Background(), nil, ListEmailsInput{From: "ALICE", To: "bob", Subject: "hell"})
	if err != nil {
		t.Fatalf("listEmails returned error: %v", err)
	}
	if output.Count != 1 || output.Emails[0].ID != 1 {
		t.Errorf("filtered result = %+v, want only email 1", output.Emails)
	}
}
