package internal

import (
    "bytes"
    "context"
    "mime/multipart"
    "net/textproto"
    "strings"
    "testing"

    "github.com/aatuh/email/types"
)

func TestBuildMIMEPlainOnly(t *testing.T) {
    msg := types.Message{
        From: types.Address{Mail: "no-reply@example.com"},
        To:   []types.Address{{Mail: "to@example.com"}},
        Plain: []byte("hello\nworld"),
        Subject: "Hi",
    }
    b, err := BuildMIME(context.Background(), msg, "<mailto:unsub@x>", nil, nil)
    if err != nil { t.Fatalf("build: %v", err) }
    s := string(b)
    if !strings.Contains(s, "Content-Type: text/plain; charset=\"UTF-8\"") {
        t.Fatalf("missing text content-type: %s", s)
    }
    if !strings.Contains(s, "List-Unsubscribe: <mailto:unsub@x>") {
        t.Fatalf("missing List-Unsubscribe")
    }
    if !strings.Contains(s, "\r\n\r\nhello\r\nworld\r\n") {
        t.Fatalf("body not quoted-printable/CRLF: %q", s)
    }
}

func TestBuildMIMEHTMLOnly(t *testing.T) {
    msg := types.Message{
        From: types.Address{Mail: "no-reply@example.com"},
        To:   []types.Address{{Mail: "to@example.com"}},
        HTML: []byte("<p>Hi</p>"),
    }
    b, err := BuildMIME(context.Background(), msg, "", nil, nil)
    if err != nil { t.Fatalf("build: %v", err) }
    s := string(b)
    if !strings.Contains(s, "Content-Type: text/html; charset=\"UTF-8\"") {
        t.Fatalf("missing html content-type: %s", s)
    }
}

func TestBuildMIMEMultipartAlternative(t *testing.T) {
    msg := types.Message{
        From:  types.Address{Mail: "no-reply@example.com"},
        To:    []types.Address{{Mail: "to@example.com"}},
        Plain: []byte("hi"),
        HTML:  []byte("<b>hi</b>"),
    }
    b, err := BuildMIME(context.Background(), msg, "", nil, nil)
    if err != nil { t.Fatalf("build: %v", err) }
    s := string(b)
    if !strings.Contains(s, "multipart/alternative;") {
        t.Fatalf("expected multipart/alternative: %s", s)
    }
}

func TestBuildMIMEMixedWithAttachment(t *testing.T) {
    var buf bytes.Buffer
    buf.WriteString("data")
    msg := types.Message{
        From:  types.Address{Mail: "no-reply@example.com"},
        To:    []types.Address{{Mail: "to@example.com"}},
        HTML:  []byte("<img src=\"cid:logo\">"),
        Attach: []types.Attachment{
            {Filename: "logo.png", ContentType: "image/png", ContentID: "logo", Reader: bytes.NewReader([]byte("PNG"))},
            {Filename: "file.txt", Reader: bytes.NewReader([]byte("hello"))},
        },
    }
    b, err := BuildMIME(context.Background(), msg, "", nil, nil)
    if err != nil { t.Fatalf("build: %v", err) }
    s := strings.ToLower(string(b))
    if !strings.Contains(s, "multipart/mixed;") {
        t.Fatalf("expected multipart/mixed: %s", s)
    }
    if !strings.Contains(s, "content-id: <logo>") || !strings.Contains(s, "inline; filename=") {
        t.Fatalf("expected inline image with Content-ID: %s", s)
    }
    if !strings.Contains(s, "attachment; filename=") {
        t.Fatalf("expected regular attachment: %s", s)
    }
}

// Ensure quoted-printable line folding works under 76/75 char rules.
func TestQuotedPrintableWrapping(t *testing.T) {
    long := strings.Repeat("A", 200)
    var buf bytes.Buffer
    writeQuotedPrintable(&buf, []byte(long))
    // Ensure CRLF and soft breaks exist.
    s := buf.String()
    if !strings.Contains(s, "=\r\n") {
        t.Fatalf("expected soft breaks in quoted-printable")
    }
}

func TestBuildMIMEHooks(t *testing.T) {
    var started, done bool
    var size int
    hooks := &types.Hooks{
        OnBuildStart: func(ctx context.Context, msg *types.Message) context.Context {
            started = true
            return ctx
        },
        OnBuildDone: func(ctx context.Context, msg *types.Message, s int, err error) {
            done = true
            size = s
            if err != nil {
                t.Fatalf("unexpected build error: %v", err)
            }
        },
    }
    msg := types.Message{
        From:  types.Address{Mail: "no-reply@example.com"},
        To:    []types.Address{{Mail: "to@example.com"}},
        Plain: []byte("hi"),
    }
    b, err := BuildMIME(context.Background(), msg, "", nil, hooks)
    if err != nil { t.Fatalf("build: %v", err) }
    if !started || !done || size == 0 || len(b) == 0 {
        t.Fatalf("hooks not invoked or size not set: started=%v done=%v size=%d len=%d", started, done, size, len(b))
    }
}

func TestNewCRLFWriterWraps(t *testing.T) {
    var buf bytes.Buffer
    w := newCRLFWriter(&buf, 10)
    _, _ = w.Write([]byte(strings.Repeat("x", 25)))
    // Expect CRLF inserted roughly every 10 cols.
    if strings.Count(buf.String(), "\r\n") < 2 {
        t.Fatalf("expected multiple CRLFs, got: %q", buf.String())
    }
}

// Sanity check header folding keeps within line limits and CRLF.
func TestWriteFoldedHeader(t *testing.T) {
    var buf bytes.Buffer
    val := strings.Repeat("word ", 30)
    writeFoldedHeader(&buf, "Subject", val)
    s := buf.String()
    if !strings.HasPrefix(s, "Subject: ") || !strings.HasSuffix(s, "\r\n") {
        t.Fatalf("header format invalid: %q", s)
    }
}

// Ensure multipart writer boundaries are present and valid.
func TestMultipartBoundaryHelpers(t *testing.T) {
    var b1, b2 bytes.Buffer
    w1, bd1 := newMixed(&b1)
    w2, bd2 := newAlternative(&b2)
    if bd1 == "" || bd2 == "" { t.Fatalf("empty boundary") }
    // Write a simple part to ensure writers are functional.
    hdr := textproto.MIMEHeader{"Content-Type": []string{"text/plain"}}
    p1, _ := w1.CreatePart(hdr)
    p1.Write([]byte("x"))
    w1.Close()
    p2, _ := w2.CreatePart(hdr)
    p2.Write([]byte("x"))
    w2.Close()
    if _, err := multipart.NewReader(&b1, bd1).NextPart(); err != nil {
        t.Fatalf("mixed boundary unreadable: %v", err)
    }
    if _, err := multipart.NewReader(&b2, bd2).NextPart(); err != nil {
        t.Fatalf("alt boundary unreadable: %v", err)
    }
}
