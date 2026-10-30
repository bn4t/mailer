package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"mailer/models"
	"mailer/storage"
)

func newTestHandler(t *testing.T) (*Handler, *storage.Store) {
	t.Helper()

	store := storage.NewStore()
	handler := NewHandler(store, ":2500", ":1143", ":8080")
	return handler, store
}

func TestListEmailsSummaryOmitsBodies(t *testing.T) {
	handler, store := newTestHandler(t)
	store.Save(&models.Email{
		From:       "alice@example.com",
		To:         []string{"bob@example.com"},
		Subject:    "hello",
		Body:       "plain body",
		HTMLBody:   "<p>html body</p>",
		RawHeaders: "Subject: hello\n",
	})

	recorder := httptest.NewRecorder()
	handler.SetupRoutes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/emails?summary=1", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	var summaries []map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &summaries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("got %d summaries, want 1", len(summaries))
	}
	for _, field := range []string{"body", "htmlBody", "rawHeaders"} {
		if _, exists := summaries[0][field]; exists {
			t.Errorf("summary unexpectedly contains %q", field)
		}
	}
	if summaries[0]["subject"] != "hello" {
		t.Errorf("subject = %v, want hello", summaries[0]["subject"])
	}

	full := httptest.NewRecorder()
	handler.SetupRoutes().ServeHTTP(full, httptest.NewRequest(http.MethodGet, "/api/emails", nil))
	if !strings.Contains(full.Body.String(), `"body":"plain body"`) {
		t.Error("full email list does not contain the body")
	}
}

func TestEmailByIDAcceptsTrailingSlash(t *testing.T) {
	handler, store := newTestHandler(t)
	id := store.Save(&models.Email{Subject: "hello"})

	recorder := httptest.NewRecorder()
	handler.SetupRoutes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/emails/"+strconv.Itoa(id)+"/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", recorder.Code, recorder.Body.String())
	}

	var email models.Email
	if err := json.Unmarshal(recorder.Body.Bytes(), &email); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if email.ID != id {
		t.Errorf("id = %d, want %d", email.ID, id)
	}
}

func TestDeleteEndpoints(t *testing.T) {
	handler, store := newTestHandler(t)
	first := store.Save(&models.Email{Subject: "first"})
	store.Save(&models.Email{Subject: "second"})

	router := handler.SetupRoutes()

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/emails/"+strconv.Itoa(first), nil))
	if recorder.Code != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", recorder.Code)
	}
	if store.Count() != 1 {
		t.Errorf("count after deleting one = %d, want 1", store.Count())
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/emails", nil))
	if recorder.Code != http.StatusNoContent {
		t.Errorf("delete-all status = %d, want 204", recorder.Code)
	}
	if store.Count() != 0 {
		t.Errorf("count after deleting all = %d, want 0", store.Count())
	}
}

func TestBundledAlpineIsServed(t *testing.T) {
	handler, _ := newTestHandler(t)

	recorder := httptest.NewRecorder()
	handler.SetupRoutes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/alpine.min.js", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Alpine") {
		t.Error("alpine.min.js was not served from the embedded filesystem")
	}
}
