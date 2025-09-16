package email

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/aatuh/email/types"
)

// SendEmailWithTemplate renders both email templates and sends the email.
func SendEmailWithTemplate(
	ctx context.Context,
	emailer types.Emailer,
	toEmail string,
	templateData map[string]any,
	mail *types.Mail,
	smtpConfig *types.SMTPConfig,
	smtpAuth *types.SMTPAuth,
) error {
	renderedPlain, err := RenderTemplate(mail.PlainBody, templateData)
	if err != nil {
		return fmt.Errorf(
			"SendEmailWithTemplate: error rendering plain email template: %w",
			err,
		)
	}

	renderedHTML, err := RenderTemplate(mail.HTMLBody, templateData)
	if err != nil {
		return fmt.Errorf(
			"SendEmailWithTemplate: error rendering HTML email template: %w",
			err,
		)
	}

	err = emailer.Send(
		ctx,
		smtpConfig,
		smtpAuth,
		&types.Mail{
			FromName:  mail.FromName,
			From:      mail.From,
			ToName:    mail.ToName,
			To:        toEmail,
			Subject:   mail.Subject,
			PlainBody: renderedPlain.String(),
			HTMLBody:  renderedHTML.String(),
			Headers:   mail.Headers,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"SendEmailWithTemplate: error sending email: %w", err,
		)
	}

	return nil
}

// RenderTemplate parses and renders an email template with the provided
// variables.
func RenderTemplate(
	templateString string, templateVariables map[string]any,
) (*bytes.Buffer, error) {
	tmpl, err := template.New("email").Parse(templateString)
	if err != nil {
		return nil, fmt.Errorf(
			"RenderTemplate: failed to parse email template: %w", err,
		)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateVariables); err != nil {
		return nil, fmt.Errorf(
			"RenderTemplate: failed to execute email template: %w", err,
		)
	}
	return &buf, nil
}

// SendEmailWithRetries sends an email with retry logic. It respects
// context cancellation between retries.
func SendEmailWithRetries(
	ctx context.Context,
	sender types.Emailer,
	smtpConfig *types.SMTPConfig,
	smtpAuth *types.SMTPAuth,
	mail *types.Mail,
	maxTryCount int,
	sleepTime time.Duration,
) error {
	var tryCount int
	for {
		err := sender.Send(ctx, smtpConfig, smtpAuth, mail)
		if err == nil {
			return nil
		}
		tryCount++
		if tryCount >= maxTryCount {
			return fmt.Errorf(
				"SendEmailWithRetries: failed to send email after %d attempts: %w",
				maxTryCount,
				err,
			)
		}
		// Wait before retrying, but respect context cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepTime):
			// Continue with the next attempt.
		}
	}
}
