package smtp

import (
	"bytes"
	"strings"
	"testing"
)

func rawMessage(lines ...string) []byte {
	return []byte(strings.Join(lines, "\r\n"))
}

func TestParseEmailNestedMultipart(t *testing.T) {
	raw := rawMessage(
		"From: =?utf-8?q?J=C3=B6rg_M=C3=BCller?= <joerg@example.com>",
		"To: bob@example.com",
		"Subject: =?utf-8?q?Gr=C3=BC=C3=9Fe?=",
		"Date: Fri, 30 Oct 2026 18:20:04 +0000",
		"Content-Type: multipart/mixed; boundary=outer",
		"",
		"--outer",
		"Content-Type: multipart/alternative; boundary=inner",
		"",
		"--inner",
		"Content-Type: text/plain; charset=iso-8859-1",
		"Content-Transfer-Encoding: 8bit",
		"",
		"Ol\xe9 from the plain part",
		"--inner",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>Olé from the HTML part</p>",
		"--inner--",
		"--outer",
		"Content-Type: text/plain",
		"Content-Disposition: attachment; filename=notes.txt",
		"",
		"attachment text must not become the body",
		"--outer--",
		"",
	)

	email, err := parseEmail(raw, "envelope@example.com", []string{"bob@example.com", "carol@example.com"})
	if err != nil {
		t.Fatalf("parseEmail returned error: %v", err)
	}

	if email.From != "Jörg Müller <joerg@example.com>" {
		t.Errorf("From = %q, want decoded display name", email.From)
	}
	if email.Subject != "Grüße" {
		t.Errorf("Subject = %q, want decoded encoded words", email.Subject)
	}
	if email.Body != "Olé from the plain part" {
		t.Errorf("Body = %q, want ISO-8859-1 decoded plain text", email.Body)
	}
	if email.HTMLBody != "<p>Olé from the HTML part</p>" {
		t.Errorf("HTMLBody = %q, want nested HTML alternative", email.HTMLBody)
	}
	if strings.Contains(email.Body, "attachment") || strings.Contains(email.HTMLBody, "attachment") {
		t.Errorf("attachment leaked into the message body: body=%q html=%q", email.Body, email.HTMLBody)
	}
	if len(email.To) != 2 || email.To[0] != "bob@example.com" || email.To[1] != "carol@example.com" {
		t.Errorf("To = %v, want SMTP recipients", email.To)
	}
	if !bytes.Equal(email.Raw, raw) {
		t.Error("Raw does not preserve the original message")
	}
	if from := strings.Index(email.RawHeaders, "From:"); from != 0 {
		t.Errorf("RawHeaders starts at %d, want original header order starting with From", from)
	}
	if strings.Index(email.RawHeaders, "To:") < strings.Index(email.RawHeaders, "From:") {
		t.Error("RawHeaders did not preserve header order")
	}
}

func TestParseEmailBodies(t *testing.T) {
	tests := []struct {
		name     string
		raw      []byte
		wantBody string
		wantHTML string
	}{
		{
			name: "html only",
			raw: rawMessage(
				"From: erin@example.com",
				"To: bob@example.com",
				"Subject: html only",
				"Content-Type: text/html; charset=utf-8",
				"",
				"<h1>Hello</h1>",
			),
			wantHTML: "<h1>Hello</h1>",
		},
		{
			name: "no content type",
			raw: rawMessage(
				"From: mia@example.com",
				"To: bob@example.com",
				"Subject: plain",
				"",
				"Just text",
			),
			wantBody: "Just text",
		},
		{
			name: "attachment only",
			raw: rawMessage(
				"From: sam@example.com",
				"To: bob@example.com",
				"Subject: attachment only",
				"Content-Type: multipart/mixed; boundary=b",
				"",
				"--b",
				"Content-Type: text/plain",
				"Content-Disposition: attachment; filename=notes.txt",
				"",
				"attached text",
				"--b--",
				"",
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, err := parseEmail(tt.raw, "sender@example.com", []string{"bob@example.com"})
			if err != nil {
				t.Fatalf("parseEmail returned error: %v", err)
			}
			if email.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", email.Body, tt.wantBody)
			}
			if email.HTMLBody != tt.wantHTML {
				t.Errorf("HTMLBody = %q, want %q", email.HTMLBody, tt.wantHTML)
			}
		})
	}
}

func TestParseEmailEmbedsInlineImages(t *testing.T) {
	raw := rawMessage(
		"From: leo@example.com",
		"To: bob@example.com",
		"Subject: inline image",
		"Content-Type: multipart/related; boundary=b",
		"",
		"--b",
		"Content-Type: text/html; charset=utf-8",
		"",
		`<img src="cid:logo@example.com">`,
		"--b",
		"Content-Type: image/png",
		"Content-ID: <logo@example.com>",
		"Content-Transfer-Encoding: base64",
		"",
		"iVBORw0KGgo=",
		"--b--",
		"",
	)

	email, err := parseEmail(raw, "leo@example.com", []string{"bob@example.com"})
	if err != nil {
		t.Fatalf("parseEmail returned error: %v", err)
	}
	if strings.Contains(email.HTMLBody, "cid:") {
		t.Errorf("HTMLBody still contains a cid reference: %q", email.HTMLBody)
	}
	if !strings.Contains(email.HTMLBody, `src="data:image/png;base64,`) {
		t.Errorf("HTMLBody does not contain an embedded data URL: %q", email.HTMLBody)
	}
}
