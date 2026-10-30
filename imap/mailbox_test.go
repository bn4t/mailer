package imap

import (
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"mailer/models"
	"mailer/storage"
)

func newTestMailbox(t *testing.T) (*Mailbox, *storage.Store) {
	t.Helper()

	store := storage.NewStore()
	backend := NewBackend(store)
	user, err := backend.Login(nil, "test", "password")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	mailbox := &Mailbox{
		name:    "INBOX",
		user:    user.(*User),
		backend: backend,
	}
	return mailbox, store
}

func TestFlagsAreSharedAcrossConnections(t *testing.T) {
	store := storage.NewStore()
	backend := NewBackend(store)
	first, _ := backend.Login(nil, "first", "password")
	second, _ := backend.Login(nil, "second", "password")
	firstUser := first.(*User)
	secondUser := second.(*User)

	id := store.Save(&models.Email{Subject: "shared"})
	firstUser.addFlags(id, []string{imap.SeenFlag})

	if !secondUser.hasFlag(id, imap.SeenFlag) {
		t.Error("flags set by one connection are not visible to another connection")
	}

	secondUser.addFlags(id, []string{imap.DeletedFlag})
	mailbox := &Mailbox{name: "INBOX", user: firstUser, backend: backend}
	if err := mailbox.Expunge(); err != nil {
		t.Fatalf("Expunge returned error: %v", err)
	}
	if store.Count() != 0 {
		t.Error("message deleted by another connection was not expunged")
	}
}

func TestSearchUsesFlags(t *testing.T) {
	mailbox, store := newTestMailbox(t)
	first := store.Save(&models.Email{Subject: "first", Body: "hello"})
	second := store.Save(&models.Email{Subject: "second", Body: "world"})

	mailbox.user.addFlags(first, []string{imap.DeletedFlag})

	deleted, err := mailbox.SearchMessages(false, &imap.SearchCriteria{
		WithFlags: []string{imap.DeletedFlag},
	})
	if err != nil {
		t.Fatalf("SearchMessages returned error: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != 1 {
		t.Errorf("deleted search = %v, want [1]", deleted)
	}

	unseen, err := mailbox.SearchMessages(false, &imap.SearchCriteria{
		WithoutFlags: []string{imap.SeenFlag},
	})
	if err != nil {
		t.Fatalf("SearchMessages returned error: %v", err)
	}
	if len(unseen) != 2 {
		t.Errorf("unseen search = %v, want both messages", unseen)
	}

	mailbox.user.addFlags(second, []string{imap.SeenFlag})
	unseen, _ = mailbox.SearchMessages(false, &imap.SearchCriteria{
		WithoutFlags: []string{imap.SeenFlag},
	})
	if len(unseen) != 1 || unseen[0] != 1 {
		t.Errorf("unseen search after marking second seen = %v, want [1]", unseen)
	}
}

func TestSearchHeaderAndBody(t *testing.T) {
	mailbox, store := newTestMailbox(t)
	store.Save(&models.Email{
		Subject:    "Password reset",
		From:       "noreply@example.com",
		To:         []string{"bob@example.com"},
		Body:       "Use this token",
		RawHeaders: "Subject: Password reset\nFrom: noreply@example.com\n",
	})
	store.Save(&models.Email{
		Subject:    "Weekly news",
		From:       "news@example.com",
		To:         []string{"bob@example.com"},
		Body:       "Nothing to see",
		RawHeaders: "Subject: Weekly news\nFrom: news@example.com\n",
	})

	criteria := &imap.SearchCriteria{
		Header: textproto.MIMEHeader{"Subject": []string{"password"}},
	}
	results, _ := mailbox.SearchMessages(false, criteria)
	if len(results) != 1 || results[0] != 1 {
		t.Errorf("subject search = %v, want [1]", results)
	}

	results, _ = mailbox.SearchMessages(false, &imap.SearchCriteria{Body: []string{"token"}})
	if len(results) != 1 || results[0] != 1 {
		t.Errorf("body search = %v, want [1]", results)
	}

	results, _ = mailbox.SearchMessages(false, &imap.SearchCriteria{Not: []*imap.SearchCriteria{
		{Header: textproto.MIMEHeader{"Subject": []string{"password"}}},
	}})
	if len(results) != 1 || results[0] != 2 {
		t.Errorf("NOT search = %v, want [2]", results)
	}
}

func TestExpungeOnlyRemovesDeletedMessages(t *testing.T) {
	mailbox, store := newTestMailbox(t)
	first := store.Save(&models.Email{Subject: "first"})
	store.Save(&models.Email{Subject: "second"})

	mailbox.user.addFlags(first, []string{imap.DeletedFlag})
	if err := mailbox.Expunge(); err != nil {
		t.Fatalf("Expunge returned error: %v", err)
	}

	remaining := store.GetAll()
	if len(remaining) != 1 || remaining[0].Subject != "second" {
		t.Fatalf("remaining emails = %v, want only second", remaining)
	}
}

func TestStatusTracksUnseenAndUidNext(t *testing.T) {
	mailbox, store := newTestMailbox(t)
	store.Save(&models.Email{Subject: "first"})
	store.Save(&models.Email{Subject: "second"})
	store.DeleteAll()
	third := store.Save(&models.Email{Subject: "third"})

	status, err := mailbox.Status([]imap.StatusItem{
		imap.StatusMessages,
		imap.StatusUnseen,
		imap.StatusUidNext,
		imap.StatusUidValidity,
	})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.Messages != 1 {
		t.Errorf("Messages = %d, want 1", status.Messages)
	}
	if status.Unseen != 1 || status.UnseenSeqNum != 1 {
		t.Errorf("Unseen = %d, UnseenSeqNum = %d, want 1, 1", status.Unseen, status.UnseenSeqNum)
	}
	if status.UidNext != uint32(third+1) {
		t.Errorf("UidNext = %d, want %d", status.UidNext, third+1)
	}
	if status.UidValidity == 0 {
		t.Error("UidValidity must be non-zero")
	}

	mailbox.user.addFlags(third, []string{imap.SeenFlag})
	status, _ = mailbox.Status([]imap.StatusItem{imap.StatusUnseen})
	if status.Unseen != 0 {
		t.Errorf("Unseen after marking seen = %d, want 0", status.Unseen)
	}
}

func TestEnvelopeParsesAddresses(t *testing.T) {
	email := &models.Email{
		From:    "Alice Smith <alice@example.com>",
		To:      []string{"bob@example.com", "Carol <carol@example.com>"},
		Subject: "Hello",
	}

	envelope := buildEnvelope(email)
	if len(envelope.From) != 1 {
		t.Fatalf("From has %d addresses, want 1", len(envelope.From))
	}
	from := envelope.From[0]
	if from.PersonalName != "Alice Smith" || from.MailboxName != "alice" || from.HostName != "example.com" {
		t.Errorf("From = %+v, want split personal name and host", from)
	}
	if len(envelope.To) != 2 {
		t.Fatalf("To has %d addresses, want 2", len(envelope.To))
	}
	if envelope.To[1].PersonalName != "Carol" || envelope.To[1].MailboxName != "carol" {
		t.Errorf("To[1] = %+v, want Carol <carol@example.com>", envelope.To[1])
	}
}

func TestBuildBodySections(t *testing.T) {
	email := &models.Email{
		Raw: []byte(strings.Join([]string{
			"From: carol@example.com",
			"To: bob@example.com",
			"Subject: alternative",
			"Content-Type: multipart/alternative; boundary=b",
			"",
			"--b",
			"Content-Type: text/plain; charset=utf-8",
			"Content-Transfer-Encoding: base64",
			"",
			"cGxhaW4=",
			"--b",
			"Content-Type: text/html; charset=utf-8",
			"",
			"<p>html</p>",
			"--b--",
			"",
		}, "\r\n")),
	}

	mailbox := &Mailbox{}
	tests := []struct {
		item string
		want string
	}{
		{"BODY[1]", "cGxhaW4="},
		{"BODY[2]", "<p>html</p>"},
		{"BODY[1.MIME]", "Content-Transfer-Encoding: base64"},
		{"BODY[HEADER.FIELDS (SUBJECT)]", "Subject: alternative\r\n\r\n"},
	}
	for _, tt := range tests {
		section, err := imap.ParseBodySectionName(imap.FetchItem(tt.item))
		if err != nil {
			t.Fatalf("%s: %v", tt.item, err)
		}
		got := string(mailbox.buildBody(email, section))
		if !strings.Contains(got, tt.want) {
			t.Errorf("%s = %q, want it to contain %q", tt.item, got, tt.want)
		}
	}

	section, _ := imap.ParseBodySectionName("BODY[]")
	if got := string(mailbox.buildBody(email, section)); got != string(email.Raw) {
		t.Error("BODY[] did not return the original message")
	}
}

func TestListMessagesMarksSeen(t *testing.T) {
	mailbox, store := newTestMailbox(t)
	id := store.Save(&models.Email{
		From:       "alice@example.com",
		To:         []string{"bob@example.com"},
		Subject:    "hello",
		Body:       "body",
		RawHeaders: "From: alice@example.com\nSubject: hello\n",
		ReceivedAt: time.Now(),
	})

	seqset, _ := imap.ParseSeqSet("1")
	messages := make(chan *imap.Message, 1)
	if err := mailbox.ListMessages(false, seqset, []imap.FetchItem{"BODY[]", imap.FetchFlags}, messages); err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	msg := <-messages
	if msg == nil {
		t.Fatal("ListMessages returned no message")
	}
	if !mailbox.user.hasFlag(id, imap.SeenFlag) {
		t.Error("fetching BODY[] did not mark the message as seen")
	}
	if len(msg.Flags) != 1 || msg.Flags[0] != imap.SeenFlag {
		t.Errorf("flags = %v, want [\\Seen]", msg.Flags)
	}

	messages = make(chan *imap.Message, 1)
	if err := mailbox.ListMessages(false, seqset, []imap.FetchItem{"BODY.PEEK[]"}, messages); err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
}
