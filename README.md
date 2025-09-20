# email

Lightweight, SMTP-first email toolkit for Go. Batteries included:
clean API, templates via `fs.FS`, multipart (text+HTML), attachments,
inline images (CID), connection pooling, timeouts, retries with jitter,
and optional rate limiting.

* Standard library only.
* Works with `embed.FS` or disk templates.
* Designed for production but tiny enough for hobby apps.

## Features

* Context-aware `Mailer.Send(ctx, Message, ...Option)`.
* Text + HTML multipart, or single-part bodies.
* Attachments and inline images (Content-ID / `cid:`).
* Custom headers, automatic `Message-ID`, and `List-Unsubscribe`.
* SMTP with STARTTLS or implicit TLS (465), timeouts.
* Connection pooling with health checks and idle TTL.
* Exponential backoff w/ jitter, transient-error retries.
* Token-bucket rate limiting (optional, sharable).

## Install

```bash
go get github.com/aatuh/email
```

## Quick start

```go
package main

import (
  "context"
  "strings"
  "time"

  "github.com/aatuh/email"
  "github.com/aatuh/email/smtp"
  "github.com/aatuh/email/types"
)

func main() {
  ctx := context.Background()

  msg := types.Message{
    From:    types.MustAddr("App <no-reply@example.com>"),
    To:      []types.Address{types.MustAddr("Ada <ada@example.com>")},
    Subject: "Welcome",
    Plain:   []byte("Hi Ada,\n\nWelcome aboard!\n"),
    HTML:    []byte("<p>Hi <b>Ada</b>,</p><p>Welcome aboard!</p>"),
    Attach: []types.Attachment{
      {
        Filename:    "hello.txt",
        ContentType: "text/plain",
        Reader:      strings.NewReader("Hi!"),
      },
    },
  }

  smtp := smtp.NewSMTP(smtp.SMTPConfig{
    Host:        "smtp.example.com",
    Port:        587,
    Username:    "user",
    Password:    "pass",
    Timeout:     10 * time.Second,
    StartTLS:    true,
    PoolMaxIdle: 2,
    PoolIdleTTL: 30 * time.Second,
  })

  bo := email.ExponentialBackoff(
    4,                   // total attempts
    250*time.Millisecond, // base delay
    4*time.Second,        // max backoff
    true,                 // full jitter
  )

  rl := email.NewTokenBucket(10, 10) // 10 msg/s, burst 10

  err := smtp.Send(ctx, msg,
    email.WithListUnsubscribe(
      "<mailto:unsubscribe@example.com>, <https://exmpl/unsub>"),
    email.WithRetry(bo),
    email.WithRateLimit(rl),
  )
  if err != nil {
    panic(err)
  }
}
```

## Templating with `fs.FS` (supports `embed.FS`)

Convention:

* `name.txt.tmpl` renders the text body.
* `name.html.tmpl` renders the HTML body.
* You may provide one or both.

```go
package main

import (
  "context"
  "embed"
  "time"

  "github.com/aatuh/email"
  "github.com/aatuh/email/smtp"
  "github.com/aatuh/email/types"
)

//go:embed templates/*
var templatesFS embed.FS

func main() {
  ctx := context.Background()

  tpl := email.MustLoadTemplates(templatesFS)
  plain, html, err := tpl.Render("welcome", map[string]any{
    "Name": "Ada",
  })
  if err != nil {
    panic(err)
  }

  msg := types.Message{
    From:    types.MustAddr("App <no-reply@example.com>"),
    To:      []types.Address{types.MustAddr("Ada <ada@example.com>")},
    Subject: "Welcome",
    Plain:   plain,
    HTML:    html,
  }

  smtp := smtp.NewSMTP(smtp.SMTPConfig{
    Host:     "smtp.example.com",
    Port:     587,
    Username: "user",
    Password: "pass",
    Timeout:  10 * time.Second,
    StartTLS: true,
  })

  if err := smtp.Send(ctx, msg); err != nil {
    panic(err)
  }
}
```

`templates/welcome.txt.tmpl`:

```text
Hi {{.Name}},

Welcome aboard!
```

`templates/welcome.html.tmpl`:

```html
<p>Hi <b>{{.Name}}</b>,</p>
<p>Welcome aboard!</p>
```

## Attachments and inline images (CID)

```go
msg := types.Message{
  // ...
  HTML: []byte(`<p>Logo below:</p><img src="cid:logo">`),
  Attach: []types.Attachment{
    {
      Filename:    "logo.png",
      ContentType: "image/png",
      ContentID:   "logo", // becomes "cid:logo" in HTML
      Reader:      bytes.NewReader(logoBytes),
    },
  },
}
```

If `ContentID` is set, the attachment is marked `inline` and gets a
`Content-ID` header. Otherwise it is a regular attachment.

## Headers and unsubscribe

You can set any header on `Message.Headers`. Common ones are set for you:
`From`, `To`, `Cc`, `Subject`, `Date`, `MIME-Version`, `Message-ID`.

Add `List-Unsubscribe` per send:

```go
err := smtp.Send(ctx, msg,
  email.WithListUnsubscribe("<mailto:unsub@example.com>, <https://u/x>"),
)
```

`Message.TrackingID` adds `X-Tracking-ID: ...`.

## Connection pooling and timeouts

Enable pooling via `SMTPConfig`:

```go
smtp := smtp.NewSMTP(smtp.SMTPConfig{
  Host:        "smtp.example.com",
  Port:        587,
  Username:    "user",
  Password:    "pass",
  Timeout:     10 * time.Second,
  StartTLS:    true,
  PoolMaxIdle: 4,              // keep up to 4 idle connections
  PoolIdleTTL: 60 * time.Second, // close idle after 60s
})
```

The pool performs a simple `NOOP` health check when reusing connections.

Timeouts:

* `SMTPConfig.Timeout` applies to dial and I/O.
* A context deadline takes precedence if provided to `Send`.

## STARTTLS vs implicit TLS (465)

* Use `StartTLS: true` for submission ports like 587.
* Use `ImplicitTLS: true` for port 465.

```go
// Port 465:
smtp := smtp.NewSMTP(smtp.SMTPConfig{
  Host:        "smtp.example.com",
  Port:        465,
  Username:    "user",
  Password:    "pass",
  Timeout:     10 * time.Second,
  ImplicitTLS: true,
})
```

`SkipVerify` exists for local dev only. Do not use it in production.

## Retries and backoff with jitter

```go
bo := email.ExponentialBackoff(
  5,                    // total attempts (initial + 4 retries)
  200*time.Millisecond, // base
  5*time.Second,        // cap
  true,                 // full jitter
)

err := smtp.Send(ctx, msg, email.WithRetry(bo))
```

Transient errors (timeouts, 4xx, temporary network issues) are retried.
Non-transient errors return immediately.

## Rate limiting

To prevent bursts, share a token bucket across sends or across workers:

```go
bucket := email.NewTokenBucket(5, 10) // 5 msg/s, burst 10
err := smtp.Send(ctx, msg, email.WithRateLimit(bucket))
```

## API reference (brief)

```go
// Package types
type Address struct {
  Name string
  Mail string
}
func MustAddr(s string) types.Address
func ParseAddress(s string) (types.Address, error)
func ParseAddressList(list []string) ([]types.Address, error)

type Attachment struct {
  Filename    string
  ContentType string
  ContentID   string
  Reader      io.Reader
}

type Message struct {
  From       types.Address
  To, Cc, Bcc []types.Address
  Subject    string
  Plain      []byte
  HTML       []byte
  Attach     []types.Attachment
  Headers    map[string]string
  TrackingID string
}
func (m *types.Message) Validate() error

// Package email
type Mailer interface {
  Send(ctx context.Context, msg types.Message, opts ...Option) error
}

type Option func(*SendConfig)
func WithListUnsubscribe(v string) Option
func WithRetry(b Backoff) Option
func WithRateLimit(bucket *TokenBucket) Option
func WithPool(pool *ConnPool) Option

type Backoff interface {
  Next(i int) (time.Duration, bool)
}
func ExponentialBackoff(
  attempts int, base, max time.Duration, fullJitter bool,
) Backoff

type TokenBucket struct { /* ... */ }
func NewTokenBucket(rate float64, burst int) *TokenBucket

type TemplateSet struct { /* ... */ }
func MustLoadTemplates(fsys fs.FS) *TemplateSet
func LoadTemplates(fsys fs.FS) (*TemplateSet, error)
func (t *TemplateSet) Render(name string, data any) ([]byte, []byte, error)

// Package smtp
type SMTPConfig struct {
  Host        string
  Port        int
  Username    string
  Password    string
  LocalName   string
  Timeout     time.Duration
  StartTLS    bool
  ImplicitTLS bool
  SkipVerify  bool
  PoolMaxIdle int
  PoolIdleTTL time.Duration
}

func NewSMTP(cfg smtp.SMTPConfig) *smtp.SMTP
```

## Error handling

`Send` returns descriptive errors for SMTP phases:

* `smtp auth: ...`
* `smtp MAIL FROM: ...`
* `smtp RCPT TO <addr>: ...`
* `smtp DATA: ...`
* `smtp write: ...`
* `smtp end data: ...`

Use `context.WithTimeout` to bound total send time. When retries are
enabled, the total wall time equals the sum of backoff delays plus the
final attempt duration.

## Security notes

* Prefer STARTTLS or implicit TLS with certificate verification on.
* Do not store credentials in code. Use env vars or secrets managers.
* Consider outbound rate limits.

## Roadmap

* Optional DKIM signing.
* Hooks for metrics/tracing (OpenTelemetry) without new deps.
