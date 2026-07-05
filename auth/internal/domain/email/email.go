// Package email sends Auth's transactional mail — for MVP just the registration
// verification link. It uses only the standard library (net/smtp) so the service
// carries no extra dependency; the local dev/Docker target is Mailpit, which
// accepts unauthenticated plaintext SMTP on port 1025.
package email

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
)

// Sender delivers the verification email. Kept as an interface so the auth
// server unit-tests against a fake and so the wiring can swap in a NoopSender
// when SMTP isn't configured (local `go run`).
type Sender interface {
	// SendVerification emails `to` a message containing verifyURL (the
	// ${app_base_url}/verify?token=... link the user clicks to confirm).
	SendVerification(ctx context.Context, to, verifyURL string) error
}

// SMTPSender sends via a plain SMTP server (no auth, no TLS) — matching Mailpit.
// It is intentionally minimal; a real provider (with auth/TLS) is a post-MVP
// swap behind the same interface.
type SMTPSender struct {
	addr string // host:port
	from string
}

// NewSMTPSender builds a sender for host:port with the given From address.
func NewSMTPSender(host, port, from string) *SMTPSender {
	return &SMTPSender{addr: net.JoinHostPort(host, port), from: from}
}

// SendVerification composes a minimal RFC 5322 message and hands it to the SMTP
// server. Delivery errors are returned so the caller can log them; per the Auth
// contract a send failure must NOT fail registration (the account already
// exists and the user can Resend).
func (s *SMTPSender) SendVerification(_ context.Context, to, verifyURL string) error {
	msg := buildMessage(s.from, to, verifyURL)
	if err := smtp.SendMail(s.addr, nil, s.from, []string{to}, msg); err != nil {
		return fmt.Errorf("email: send verification to %s: %w", to, err)
	}
	return nil
}

// buildMessage returns the raw bytes of a simple text/plain verification email.
func buildMessage(from, to, verifyURL string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: Confirm your DogMap email\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString("Welcome to DogMap!\r\n\r\n")
	b.WriteString("Confirm your email address by opening this link:\r\n")
	b.WriteString(verifyURL + "\r\n\r\n")
	b.WriteString("If you didn't create a DogMap account, you can ignore this email.\r\n")
	return []byte(b.String())
}

// NoopSender is used when no SMTP host is configured (e.g. local `go run`). It
// logs the link instead of sending it, so the flow is still exercisable without
// a mail server.
type NoopSender struct{}

func (NoopSender) SendVerification(ctx context.Context, to, verifyURL string) error {
	slog.InfoContext(ctx, "email (noop) verification link", "to", to, "verify_url", verifyURL)
	return nil
}
