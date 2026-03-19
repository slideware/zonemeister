package mail

import (
	"strings"
	"testing"
	"time"
)

func TestBuildMessage(t *testing.T) {
	before := time.Now().UTC()
	msg := buildMessage("sender@example.com", "recipient@example.com", "Test Subject", "Hello, world!")
	after := time.Now().UTC()

	if !strings.Contains(msg, "From: sender@example.com\r\n") {
		t.Error("missing From header")
	}
	if !strings.Contains(msg, "To: recipient@example.com\r\n") {
		t.Error("missing To header")
	}
	if !strings.Contains(msg, "Subject: Test Subject\r\n") {
		t.Error("missing Subject header")
	}
	if !strings.Contains(msg, "Content-Type: text/plain; charset=utf-8\r\n") {
		t.Error("missing Content-Type header")
	}
	if !strings.Contains(msg, "MIME-Version: 1.0\r\n") {
		t.Error("missing MIME-Version header")
	}
	if !strings.Contains(msg, "Date: ") {
		t.Error("missing Date header")
	}

	// Body should appear after double CRLF.
	parts := strings.SplitN(msg, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatal("message missing header/body separator")
	}
	if parts[1] != "Hello, world!" {
		t.Errorf("unexpected body: %q", parts[1])
	}

	// Verify Date is within the expected time range.
	dateIdx := strings.Index(msg, "Date: ")
	dateEnd := strings.Index(msg[dateIdx:], "\r\n")
	dateStr := msg[dateIdx+6 : dateIdx+dateEnd]
	parsedDate, err := time.Parse(time.RFC1123Z, dateStr)
	if err != nil {
		t.Fatalf("invalid Date header: %v", err)
	}
	if parsedDate.Before(before.Add(-time.Second)) || parsedDate.After(after.Add(time.Second)) {
		t.Errorf("Date header %v not within expected range [%v, %v]", parsedDate, before, after)
	}
}

func TestBuildMessageMultilineBody(t *testing.T) {
	body := "Line one.\nLine two.\nLine three."
	msg := buildMessage("a@b.com", "c@d.com", "Multi", body)

	parts := strings.SplitN(msg, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatal("message missing header/body separator")
	}
	if parts[1] != body {
		t.Errorf("unexpected body: %q", parts[1])
	}
}

func TestNewMailer(t *testing.T) {
	cfg := SMTPConfig{
		Host:     "smtp.example.com",
		Port:     587,
		User:     "user",
		Password: "pass",
		From:     "noreply@example.com",
	}
	m := NewMailer(cfg)
	if m.cfg.Host != "smtp.example.com" {
		t.Error("config not stored correctly")
	}
}
