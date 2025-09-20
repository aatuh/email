package email

import (
	"context"

	"github.com/aatuh/email/types"
)

// Mailer defines the interface for email sending adapters.
// Implementations should handle connection management, authentication,
// and delivery according to their specific protocol (SMTP, API, etc.).
type Mailer interface {
	// Send sends an email message with the given options.
	//
	// Parameters:
	//   - ctx: The context for cancellation and timeouts.
	//   - msg: The email message to send.
	//   - opts: Optional configuration for this send operation.
	//
	// Returns:
	//   - error: An error if the email fails to send. Implementations
	//     should return context errors when the operation is cancelled
	//     or times out.
	Send(ctx context.Context, msg types.Message, opts ...Option) error
}
