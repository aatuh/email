# email

Minimal helpers to render templates and send multi‑part emails via an
interface (`types.Emailer`) with an SMTP implementation.

## Install

```go
import (
  "github.com/aatuh/email"
  "github.com/aatuh/email/types"
)
```

## Quick start

```go
ctx := context.Background()
sender := email.NewSMTP()

smtpCfg := &types.SMTPConfig{SMTPHost: "smtp.example.com", SMTPPort: 587, UseTLS: true}
auth := &types.SMTPAuth{Username: "user", Password: "pass"}

mail := &types.Mail{
  FromName:  "App",
  From:      "no-reply@example.com",
  ToName:    "User",
  To:        "user@example.com",
  Subject:   "Welcome",
  PlainBody: "Hello {{.Name}}",
  HTMLBody:  "<p>Hello <b>{{.Name}}</b></p>",
}

err := email.SendEmailWithTemplate(ctx, sender, mail.To, map[string]any{"Name": "Ada"}, mail, smtpCfg, auth)
if err != nil { /* handle */ }
```

### Retry helper

```go
err := email.SendEmailWithRetries(ctx, sender, smtpCfg, auth, mail, 3, time.Second)
```

## Notes

- `SendEmailWithTemplate` renders both plain and HTML bodies with
  `text/template`/`html/template`.
- SMTP sender builds multi-part/alternative messages when both bodies are
  provided.
- Implement `types.Emailer` to plug in any email backend.
