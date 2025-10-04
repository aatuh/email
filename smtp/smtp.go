package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/aatuh/email/v2"
	"github.com/aatuh/email/v2/internal"
	"github.com/aatuh/email/v2/types"
)

// SMTPConfig configures the SMTP mailer.
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

	// Pool settings (optional). If PoolMaxIdle <= 0, no pooling is used.
	PoolMaxIdle int
	PoolIdleTTL time.Duration
}

// smtpConn is a connection to the SMTP server.
type smtpConn struct {
	c   *smtp.Client
	tls bool
}

// SMTP implements the Mailer interface over SMTP.
type SMTP struct {
	cfg  SMTPConfig
	pool *email.ConnPool
}

// NewSMTP creates a new SMTP mailer.
//
// Parameters:
//   - cfg: The SMTP config.
//
// Returns:
//   - *SMTP: The SMTP mailer.
func NewSMTP(cfg SMTPConfig) *SMTP {
	m := &SMTP{cfg: cfg}
	if cfg.PoolMaxIdle > 0 {
		m.pool = email.NewConnPool(
			cfg.PoolMaxIdle,
			cfg.PoolIdleTTL,
			func() (any, error) { return m.newConn() },
			func(a any) error {
				if sc, ok := a.(*smtpConn); ok && sc.c != nil {
					return sc.c.Quit()
				}
				return nil
			},
			func(a any) bool {
				if sc, ok := a.(*smtpConn); ok && sc.c != nil {
					return sc.c.Noop() == nil
				}
				return false
			},
		)
	}
	return m
}

// Send sends an email.
//
// Parameters:
//   - ctx: The context.
//   - msg: The message.
//   - opts: The options.
//
// Returns:
//   - error: The error if the email fails to send.
func (m *SMTP) Send(
	ctx context.Context,
	msg types.Message,
	opts ...email.Option,
) error {
	var cfg email.SendConfig
	for _, o := range opts {
		o(&cfg)
	}

	if cfg.Rate != nil {
		cfg.Rate.Wait()
	}

	// Build MIME once (DKIM signs body). Hooks wrap build.
	raw, err := internal.BuildMIME(ctx, msg, cfg.ListUnsub, cfg.DKIM, cfg.Hooks)
	if err != nil {
		return err
	}

	// Choose attempt schedule.
	var bo email.Backoff = &singleAttempt{}
	if cfg.Backoff != nil {
		bo = cfg.Backoff
	}

	attempt := 0
	for {
		if cfg.Hooks != nil && cfg.Hooks.OnAttemptStart != nil {
			ctx = cfg.Hooks.OnAttemptStart(ctx, attempt)
		}

		d, ok := bo.Next(attempt)
		if !ok {
			if cfg.Hooks != nil && cfg.Hooks.OnAttemptDone != nil {
				cfg.Hooks.OnAttemptDone(ctx, attempt,
					fmt.Errorf("attempts exhausted"))
			}
			return fmt.Errorf("send attempts exhausted after %d tries",
				attempt)
		}
		if d > 0 {
			select {
			case <-time.After(d):
			case <-ctx.Done():
				if cfg.Hooks != nil && cfg.Hooks.OnAttemptDone != nil {
					cfg.Hooks.OnAttemptDone(ctx, attempt, ctx.Err())
				}
				return ctx.Err()
			}
		}

		err = m.trySend(ctx, msg, raw, &cfg)
		if cfg.Hooks != nil && cfg.Hooks.OnAttemptDone != nil {
			cfg.Hooks.OnAttemptDone(ctx, attempt, err)
		}
		if err == nil {
			return nil
		}
		if !isTransient(err) {
			return err
		}
		attempt++
	}
}

// singleAttempt is a single attempt backoff.
type singleAttempt struct{}

// Next returns sleep before attempt i (0-based). ok=false when no more.
//
// Parameters:
//   - i: The attempt number.
//
// Returns:
//   - time.Duration: The sleep time.
//   - bool: True if there is more to attempt.
func (s *singleAttempt) Next(i int) (time.Duration, bool) {
	if i == 0 {
		return 0, true
	}
	return 0, false
}

// trySend tries to send an email.
func (m *SMTP) trySend(
	ctx context.Context,
	msg types.Message,
	raw []byte,
	cfg *email.SendConfig,
) error {
	var conn *smtpConn
	var err error

	if cfg.Pool != nil {
		aconn, aerr := cfg.Pool.Get()
		if aerr != nil {
			return aerr
		}
		if aconn != nil {
			conn = aconn.(*smtpConn)
		}
	}
	if conn == nil {
		conn, err = m.newConn()
		if err != nil {
			return err
		}
		defer func() {
			if cfg.Pool == nil && conn != nil && conn.c != nil {
				_ = conn.c.Quit()
			}
		}()
	}
	defer func() {
		if cfg.Pool != nil && conn != nil {
			cfg.Pool.Put(conn)
		}
	}()

	c := conn.c

	// Set deadlines using context if possible.
	var cancel context.CancelFunc
	if dl, ok := ctx.Deadline(); ok {
		_, cancel = context.WithDeadline(ctx, dl)
	} else if m.cfg.Timeout > 0 {
		_, cancel = context.WithDeadline(ctx, time.Now().Add(m.cfg.Timeout))
	}
	defer cancel()

	if m.cfg.Username != "" && m.cfg.Password != "" {
		auth := smtp.PlainAuth(
			"", m.cfg.Username, m.cfg.Password, m.cfg.Host,
		)
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
	}

	if err := c.Mail(msg.From.Mail); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, rcpt := range msg.RecipientList() {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", rcpt, err)
		}
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp end data: %w", err)
	}

	return nil
}

// newConn creates a new SMTP connection.
func (m *SMTP) newConn() (*smtpConn, error) {
	hostPort := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	local := m.cfg.LocalName
	if local == "" {
		local, _ = internal.OsHostname()
	}

	var c *smtp.Client
	var err error
	if m.cfg.ImplicitTLS {
		conf := &tls.Config{
			ServerName:         m.cfg.Host,
			InsecureSkipVerify: m.cfg.SkipVerify,
		}
		dialer := &net.Dialer{Timeout: m.cfg.Timeout}
		conn, derr := tls.DialWithDialer(dialer, "tcp", hostPort, conf)
		if derr != nil {
			return nil, fmt.Errorf("smtp tls dial: %w", derr)
		}
		c, err = smtp.NewClient(conn, m.cfg.Host)
		if err != nil {
			return nil, fmt.Errorf("smtp new client: %w", err)
		}
	} else {
		dialer := &net.Dialer{Timeout: m.cfg.Timeout}
		conn, derr := dialer.Dial("tcp", hostPort)
		if derr != nil {
			return nil, fmt.Errorf("smtp dial: %w", derr)
		}
		c, err = smtp.NewClient(conn, m.cfg.Host)
		if err != nil {
			return nil, fmt.Errorf("smtp new client: %w", err)
		}
		if m.cfg.StartTLS {
			conf := &tls.Config{
				ServerName:         m.cfg.Host,
				InsecureSkipVerify: m.cfg.SkipVerify,
			}
			if ok, _ := c.Extension("STARTTLS"); ok {
				if terr := c.StartTLS(conf); terr != nil {
					_ = c.Quit()
					return nil, fmt.Errorf("smtp starttls: %w", terr)
				}
			}
		}
	}

	if err := c.Hello(local); err != nil {
		_ = c.Quit()
		return nil, fmt.Errorf("smtp EHLO: %w", err)
	}
	return &smtpConn{c: c, tls: m.cfg.ImplicitTLS || m.cfg.StartTLS}, nil
}

// isTransient checks if an error is transient.
func isTransient(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, " 4") || strings.Contains(msg, "4xx") {
		return true
	}
	for _, s := range []string{
		"timeout",
		"temporarily",
		"try again",
		"connection reset",
		"broken pipe",
	} {
		if strings.Contains(strings.ToLower(msg), s) {
			return true
		}
	}
	return false
}
