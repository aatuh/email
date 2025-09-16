package types

import (
	"context"
	"crypto/tls"
)

// Emailer defines an interface for sending emails.
type Emailer interface {
	Send(
		ctx context.Context,
		smtpConfig *SMTPConfig,
		smtpAuth *SMTPAuth,
		mail *Mail,
	) error
}

// SMTPConfig holds SMTP server configuration.
type SMTPConfig struct {
	SMTPHost  string
	SMTPPort  int
	UseTLS    bool
	TLSConfig *tls.Config
}

// SMTPAuth holds SMTP authentication credentials.
type SMTPAuth struct {
	Username string
	Password string
}

// Mail represents an email message.
type Mail struct {
	FromName  string
	From      string
	ToName    string
	To        string
	Subject   string
	PlainBody string
	HTMLBody  string
	Headers   map[string]string
}
