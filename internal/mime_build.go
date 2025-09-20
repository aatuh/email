package internal

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"

	"github.com/aatuh/email/types"
)

// buildMIME assembles headers + body. If dkim != nil, it signs the
// message and inserts a DKIM-Signature header. Hooks wrap build timing.
func BuildMIME(
	ctx context.Context,
	msg types.Message,
	listUnsub string,
	dkim *types.DKIMConfig,
	hooks *types.Hooks,
) ([]byte, error) {
	if err := msg.Validate(); err != nil {
		return nil, err
	}

	if hooks != nil && hooks.OnBuildStart != nil {
		ctx = hooks.OnBuildStart(ctx, &msg)
	}

	h := msg.CloneHeaders()
	ensureListUnsub(h, listUnsub)

	setHeader(h, "From", msg.From.String())
	if len(msg.To) > 0 {
		setHeader(h, "To", joinAddrs(msg.To))
	}
	if len(msg.Cc) > 0 {
		setHeader(h, "Cc", joinAddrs(msg.Cc))
	}
	setHeader(h, "Subject", sanitizeHeader(msg.Subject))
	setHeader(h, "Date", time.Now().UTC().Format(time.RFC1123Z))
	setHeader(h, "MIME-Version", "1.0")
	if msg.TrackingID != "" {
		setHeader(h, "X-Tracking-ID", sanitizeHeader(msg.TrackingID))
	}
	if _, ok := h["Message-ID"]; !ok {
		setHeader(h, "Message-ID", genMessageID(msg))
	}

	// Build body first into bodyBuf so DKIM can hash it.
	var bodyBuf bytes.Buffer
	hasPlain := len(msg.Plain) > 0
	hasHTML := len(msg.HTML) > 0
	hasAttach := len(msg.Attach) > 0

	switch {
	case hasAttach:
		mixedW, mixedBoundary := newMixed(&bodyBuf)
		h["Content-Type"] = fmt.Sprintf(
			`multipart/mixed; boundary="%s"`, mixedBoundary,
		)
		// Alternatives nested part.
		if hasPlain || hasHTML {
			var altBuf bytes.Buffer
			altW, altBoundary := newAlternative(&altBuf)
			if hasPlain {
				writeTextPart(altW, msg.Plain)
			}
			if hasHTML {
				writeHTMLPart(altW, msg.HTML)
			}
			_ = altW.Close()

			hdr := textproto.MIMEHeader{}
			hdr.Set("Content-Type",
				fmt.Sprintf(`multipart/alternative; boundary="%s"`,
					altBoundary))
			pw, _ := mixedW.CreatePart(hdr)
			_, _ = io.Copy(pw, &altBuf)
		}
		for _, a := range msg.Attach {
			writeAttachment(mixedW, a)
		}
		_ = mixedW.Close()

	case hasPlain && hasHTML:
		altW, altBoundary := newAlternative(&bodyBuf)
		h["Content-Type"] = fmt.Sprintf(
			`multipart/alternative; boundary="%s"`, altBoundary,
		)
		writeTextPart(altW, msg.Plain)
		writeHTMLPart(altW, msg.HTML)
		_ = altW.Close()

	case hasHTML:
		h["Content-Type"] = `text/html; charset="UTF-8"`
		h["Content-Transfer-Encoding"] = "quoted-printable"
		writeQuotedPrintable(&bodyBuf, msg.HTML)

	default:
		h["Content-Type"] = `text/plain; charset="UTF-8"`
		h["Content-Transfer-Encoding"] = "quoted-printable"
		writeQuotedPrintable(&bodyBuf, msg.Plain)
	}

	// If DKIM enabled, compute and insert DKIM-Signature.
	if dkim != nil {
		sigVal, err := BuildDKIMSignature(h, bodyBuf.Bytes(), *dkim)
		if err != nil {
			if hooks != nil && hooks.OnBuildDone != nil {
				hooks.OnBuildDone(ctx, &msg, 0, err)
			}
			return nil, err
		}
		setHeader(h, "DKIM-Signature", sigVal)
	}

	// Now write headers + CRLF + body to final buffer.
	var out bytes.Buffer
	writeHeaders(&out, h)
	_, _ = io.Copy(&out, &bodyBuf)

	if hooks != nil && hooks.OnBuildDone != nil {
		hooks.OnBuildDone(ctx, &msg, out.Len(), nil)
	}

	return out.Bytes(), nil
}

func joinAddrs(xs []types.Address) string {
	out := make([]string, 0, len(xs))
	for _, a := range xs {
		out = append(out, a.String())
	}
	return strings.Join(out, ", ")
}

func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

func genMessageID(m types.Message) string {
	var r [12]byte
	_, _ = rand.Read(r[:])
	host := "localhost"
	if i := strings.LastIndex(m.From.Mail, "@"); i != -1 {
		host = m.From.Mail[i+1:]
	}
	return fmt.Sprintf("<%x%x@%s>", time.Now().UnixNano(), r, host)
}

func writeHeaders(w io.Writer, h map[string]string) {
	for k, v := range h {
		writeFoldedHeader(w, k, v)
	}
	io.WriteString(w, "\r\n")
}

func writeFoldedHeader(w io.Writer, key, val string) {
	const limit = 78
	line := key + ": " + val
	if len(line) <= limit {
		io.WriteString(w, line+"\r\n")
		return
	}
	words := strings.Fields(val)
	curr := key + ":"
	for _, wd := range words {
		if len(curr)+1+len(wd) > limit {
			io.WriteString(w, curr+"\r\n")
			curr = " " + wd
		} else {
			curr += " " + wd
		}
	}
	io.WriteString(w, curr+"\r\n")
}

func newMixed(buf *bytes.Buffer) (*multipart.Writer, string) {
	w := multipart.NewWriter(buf)
	return w, w.Boundary()
}

func newAlternative(buf *bytes.Buffer) (*multipart.Writer, string) {
	w := multipart.NewWriter(buf)
	return w, w.Boundary()
}

func writeTextPart(w *multipart.Writer, body []byte) {
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", `text/plain; charset="UTF-8"`)
	h.Set("Content-Transfer-Encoding", "quoted-printable")
	pw, _ := w.CreatePart(h)
	writeQuotedPrintable(pw, body)
}

func writeHTMLPart(w *multipart.Writer, body []byte) {
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", `text/html; charset="UTF-8"`)
	h.Set("Content-Transfer-Encoding", "quoted-printable")
	pw, _ := w.CreatePart(h)
	writeQuotedPrintable(pw, body)
}

func writeAttachment(w *multipart.Writer, a types.Attachment) {
	ct := a.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	h := textproto.MIMEHeader{}
	if a.ContentID != "" {
		h.Set("Content-Disposition",
			fmt.Sprintf(`inline; filename="%s"`,
				mime.QEncoding.Encode("UTF-8", a.Filename)))
		h.Set("Content-ID", fmt.Sprintf("<%s>", a.ContentID))
	} else {
		h.Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="%s"`,
				mime.QEncoding.Encode("UTF-8", a.Filename)))
	}
	h.Set("Content-Type", ct)
	h.Set("Content-Transfer-Encoding", "base64")

	pw, _ := w.CreatePart(h)
	enc := base64.NewEncoder(base64.StdEncoding, newCRLFWriter(pw, 76))
	defer enc.Close()
	_, _ = io.Copy(enc, a.Reader)
}

// writeQuotedPrintable writes text as quoted-printable with CRLF breaks.
func writeQuotedPrintable(w io.Writer, b []byte) {
	const hex = "0123456789ABCDEF"
	col := 0
	for _, c := range b {
		var out []byte
		switch {
		case c == '\r':
			continue
		case c == '\n':
			w.Write([]byte("\r\n"))
			col = 0
			continue
		case c == '=' || c < 32 || c > 126:
			out = []byte{'=', hex[c>>4], hex[c&15]}
		default:
			out = []byte{c}
		}
		if col+len(out) > 75 {
			w.Write([]byte("=\r\n"))
			col = 0
		}
		w.Write(out)
		col += len(out)
	}
	w.Write([]byte("\r\n"))
}

type crlfWriter struct {
	w   io.Writer
	col int
	n   int
}

func newCRLFWriter(w io.Writer, n int) *crlfWriter {
	return &crlfWriter{w: w, n: n}
}

func (cw *crlfWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		remain := cw.n - cw.col
		if remain <= 0 {
			if _, err := cw.w.Write([]byte("\r\n")); err != nil {
				return written, err
			}
			cw.col = 0
			remain = cw.n
		}
		chunk := p
		if len(chunk) > remain {
			chunk = p[:remain]
		}
		n, err := cw.w.Write(chunk)
		written += n
		cw.col += n
		if err != nil {
			return written, err
		}
		p = p[n:]
		if cw.col >= cw.n {
			if _, err := cw.w.Write([]byte("\r\n")); err != nil {
				return written, err
			}
			cw.col = 0
		}
	}
	return written, nil
}

// setHeader sets/overwrites a header key.
func setHeader(h map[string]string, key, val string) {
	if val == "" {
		return
	}
	h[key] = val
}

// ensureListUnsub folds header variants into the standard key.
func ensureListUnsub(h map[string]string, listUnsub string) {
	if listUnsub == "" {
		return
	}
	setHeader(h, "List-Unsubscribe", listUnsub)
}
