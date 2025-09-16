package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"

	"github.com/aatuh/email/types"
)

// SMTP is an SMTP-based sender that sends multi-part emails.
type SMTP struct{}

// NewSMTP creates a new SMTP instance.
func NewSMTP() *SMTP {
	return &SMTP{}
}

// Send sends an email via SMTP.
func (s *SMTP) Send(
	ctx context.Context,
	smtpConfig *types.SMTPConfig,
	smtpAuth *types.SMTPAuth,
	mail *types.Mail,
) error {
	// Build headers.
	headers := map[string]string{
		"From":    formatAddress(mail.FromName, mail.From),
		"To":      formatAddress(mail.ToName, mail.To),
		"Subject": mail.Subject,
	}
	// Merge any custom headers.
	for k, v := range mail.Headers {
		headers[k] = v
	}

	var msg bytes.Buffer

	// Write basic headers.
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}

	// Build body.
	// If both plain text and HTML bodies are provided,
	// then create a multipart/alternative message.
	if mail.PlainBody != "" && mail.HTMLBody != "" {
		boundary := "my-boundary-123456" // Use a unique boundary in production.
		msg.WriteString("MIME-Version: 1.0\r\n")
		msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
		msg.WriteString("\r\n")

		// Plain text part.
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		msg.WriteString(mail.PlainBody + "\r\n")

		// HTML part.
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
		msg.WriteString(mail.HTMLBody + "\r\n")

		// Closing boundary.
		msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		// If only one body is provided, just use that.
		msg.WriteString("\r\n")
		if mail.HTMLBody != "" {
			msg.WriteString(mail.HTMLBody)
		} else {
			msg.WriteString(mail.PlainBody)
		}
	}

	addr := fmt.Sprintf("%s:%d", smtpConfig.SMTPHost, smtpConfig.SMTPPort)
	errCh := make(chan error, 1)

	go func() {
		var err error
		if smtpConfig.UseTLS {
			tlsConfig := smtpConfig.TLSConfig
			if tlsConfig == nil {
				tlsConfig = &tls.Config{
					InsecureSkipVerify: true,
					ServerName:         smtpConfig.SMTPHost,
				}
			}
			conn, err := tls.Dial("tcp", addr, tlsConfig)
			if err != nil {
				errCh <- fmt.Errorf("failed to dial TLS: %w", err)
				return
			}
			client, err := smtp.NewClient(conn, smtpConfig.SMTPHost)
			if err != nil {
				errCh <- fmt.Errorf("failed to create SMTP client: %w", err)
				return
			}
			defer func() {
				if err := client.Quit(); err != nil {
					errCh <- fmt.Errorf("failed to quit SMTP client: %w", err)
				}
			}()
			auth := smtp.PlainAuth("", smtpAuth.Username, smtpAuth.Password,
				smtpConfig.SMTPHost)
			if err = client.Auth(auth); err != nil {
				errCh <- fmt.Errorf("SMTP auth failed: %w", err)
				return
			}
			if err = client.Mail(mail.From); err != nil {
				errCh <- fmt.Errorf("failed to set sender: %w", err)
				return
			}
			if err = client.Rcpt(mail.To); err != nil {
				errCh <- fmt.Errorf("failed to set recipient: %w", err)
				return
			}
			w, err := client.Data()
			if err != nil {
				errCh <- fmt.Errorf("failed to get data writer: %w", err)
				return
			}
			_, err = w.Write(msg.Bytes())
			if err != nil {
				errCh <- fmt.Errorf("failed to write email data: %w", err)
				return
			}
			if err = w.Close(); err != nil {
				errCh <- fmt.Errorf("failed to close data writer: %w", err)
				return
			}
			errCh <- nil
		} else {
			// For non-TLS connections, omit auth.
			err = smtp.SendMail(
				addr, nil, mail.From, []string{mail.To}, msg.Bytes(),
			)
			errCh <- err
		}
	}()

	// Wait for the send operation or context cancellation.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// formatAddress returns a formatted email address.
func formatAddress(name, email string) string {
	if name != "" {
		return fmt.Sprintf("%s <%s>", name, email)
	}
	return email
}
