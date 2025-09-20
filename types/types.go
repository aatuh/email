package types

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"
)

// Attachment represents a file attachment or inline image.
type Attachment struct {
	Filename    string    // file name to present in email client
	ContentType string    // e.g. "application/pdf"
	ContentID   string    // set to serve as inline image "cid:<ContentID>"
	Reader      io.Reader // streamed content
}

// Message is the high-level representation of an email.
type Message struct {
	From       Address
	To         []Address
	Cc         []Address
	Bcc        []Address
	Subject    string
	Plain      []byte // optional
	HTML       []byte // optional
	Attach     []Attachment
	Headers    map[string]string
	TrackingID string
}

// Validate minimal correctness before send.
//
// Returns:
//   - error: An error if the message is invalid.
func (m *Message) Validate() error {
	if m.From.Mail == "" {
		return errors.New("missing From")
	}
	if len(m.To) == 0 && len(m.Cc) == 0 && len(m.Bcc) == 0 {
		return errors.New("no recipients")
	}
	if len(m.Plain) == 0 && len(m.HTML) == 0 && len(m.Attach) == 0 {
		return errors.New("no body or attachments")
	}
	return nil
}

// RecipientList returns combined To+Cc+Bcc for envelope use.
func (m *Message) RecipientList() []string {
	var out []string
	add := func(xs []Address) {
		for _, a := range xs {
			if s := strings.TrimSpace(a.Mail); s != "" {
				out = append(out, s)
			}
		}
	}
	add(m.To)
	add(m.Cc)
	add(m.Bcc)
	return out
}

// CloneHeaders returns a shallow copy safe for per-send mutation.
func (m *Message) CloneHeaders() map[string]string {
	cp := make(map[string]string, len(m.Headers))
	for k, v := range m.Headers {
		cp[k] = v
	}
	return cp
}

// Address represents a single email address with an optional display name.
type Address struct {
	Name string
	Mail string
}

// String renders a display-friendly representation for headers.
//
// Returns:
//   - string: The display-friendly representation of the address.
func (a Address) String() string {
	if a.Name == "" {
		return a.Mail
	}
	// Use mail.Address to handle quoting as needed.
	return (&mail.Address{Name: a.Name, Address: a.Mail}).String()
}

// Hooks allows you to integrate tracing/metrics without extra deps.
// Return a derived context from Start hooks if you want to carry spans.
type Hooks struct {
	OnBuildStart func(ctx context.Context, msg *Message) context.Context
	OnBuildDone  func(ctx context.Context, msg *Message, size int,
		err error)
	OnAttemptStart func(ctx context.Context, attempt int) context.Context
	OnAttemptDone  func(ctx context.Context, attempt int, err error)
}

// DKIMConfig enables DKIM signing (rsa-sha256, relaxed/relaxed).
// Headers lists which header field names to include in "h=" in order.
// Use lowercase names (e.g. "from", "to", "subject").
type DKIMConfig struct {
	Domain   string
	Selector string
	KeyPEM   []byte
	Headers  []string
}

// MustAddr parses an address like "Ada <ada@example.com>" or
// "ada@example.com". Panics on error. Use in examples or tests.
//
// Parameters:
//   - s: The address string to parse.
//
// Returns:
//   - Address: The parsed address.
func MustAddr(s string) Address {
	addr, err := ParseAddress(s)
	if err != nil {
		panic(err)
	}
	return addr
}

// ParseAddress parses a single address string into Address.
//
// Parameters:
//   - s: The address string to parse.
//
// Returns:
//   - Address: The parsed address.
//   - error: An error if the address string is invalid.
func ParseAddress(s string) (Address, error) {
	s = strings.TrimSpace(s)
	ma, err := mail.ParseAddress(s)
	if err != nil {
		return Address{}, fmt.Errorf("parse address: %w", err)
	}
	// We keep it simple and trust net/mail. If you need punycode for
	// non-ascii domains, add it here. For now we accept the literal.
	return Address{Name: ma.Name, Mail: strings.TrimSpace(ma.Address)}, nil
}

// ParseAddressList parses a header-like list into []Address.
//
// Parameters:
//   - list: The list of address strings to parse.
//
// Returns:
//   - []Address: The parsed addresses.
//   - error: An error if the address list is invalid.
func ParseAddressList(list []string) ([]Address, error) {
	if len(list) == 0 {
		return nil, nil
	}
	// Accept multiple forms: already split or combined string with commas.
	var joined string
	if len(list) == 1 {
		joined = list[0]
	} else {
		joined = strings.Join(list, ",")
	}
	parsed, err := mail.ParseAddressList(joined)
	if err != nil {
		return nil, fmt.Errorf("parse address list: %w", err)
	}
	out := make([]Address, 0, len(parsed))
	for _, ma := range parsed {
		out = append(out, Address{Name: ma.Name, Mail: ma.Address})
	}
	return out, nil
}
