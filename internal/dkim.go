package internal

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aatuh/email/v2/types"
)

// buildDKIMSignature creates the DKIM-Signature header value for the
// given headers map and body bytes using relaxed/relaxed c14n and
// rsa-sha256. Only standard library is used.
func BuildDKIMSignature(
	headers map[string]string,
	body []byte,
	cfg types.DKIMConfig,
) (string, error) {
	if cfg.Domain == "" || cfg.Selector == "" || len(cfg.KeyPEM) == 0 {
		return "", errors.New("dkim: incomplete config")
	}
	key, err := parseRSAPrivateKey(cfg.KeyPEM)
	if err != nil {
		return "", fmt.Errorf("dkim: parse key: %w", err)
	}

	// Canonicalize body (relaxed) and compute bh=
	cBody := dkimCanonicalizeBodyRelaxed(body)
	bh := sha256.Sum256(cBody)
	bhB64 := base64.StdEncoding.EncodeToString(bh[:])

	// Determine header list to sign in order. If cfg.Headers is empty,
	// sign a sensible default subset commonly used.
	hlist := cfg.Headers
	if len(hlist) == 0 {
		hlist = []string{
			"from", "to", "subject", "date",
			"mime-version", "content-type", "message-id",
		}
	}
	// Take only headers present; keep requested order.
	var signedNames []string
	var signedLines []string
	for _, name := range hlist {
		hn := headerLookup(headers, name)
		if hn == "" {
			continue
		}
		val := headers[hn]
		signedNames = append(signedNames, strings.ToLower(hn))
		signedLines = append(signedLines, dkimCanonHeaderRelaxed(hn, val))
	}

	// Prepare DKIM-Signature header (without b= value).
	now := time.Now().Unix()
	dkimFields := map[string]string{
		"v":  "1",
		"a":  "rsa-sha256",
		"c":  "relaxed/relaxed",
		"d":  cfg.Domain,
		"s":  cfg.Selector,
		"t":  fmt.Sprintf("%d", now),
		"bh": bhB64,
		"h":  strings.Join(signedNames, ":"),
	}
	// Join tag=value; order by tag name (typical practice).
	var tags []string
	for k := range dkimFields {
		tags = append(tags, k)
	}
	sort.Strings(tags)
	var b strings.Builder
	for i, k := range tags {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(dkimFields[k])
	}
	// Append b= empty; folding done by our header writer later.
	unsignedDKIM := "DKIM-Signature: " + b.String() + "; b="

	// Build signing input: signed headers + DKIM-Signature w/ empty b=
	var toSign bytes.Buffer
	for _, line := range signedLines {
		toSign.WriteString(line)
		toSign.WriteString("\r\n")
	}
	toSign.WriteString(dkimCanonLine(unsignedDKIM))
	toSign.WriteString("\r\n")

	// Sign with RSA-SHA256
	hash := sha256.Sum256(toSign.Bytes())
	sig, err := rsa.SignPKCS1v15(
		nil, key, crypto.SHA256, hash[:],
	)
	if err != nil {
		return "", fmt.Errorf("dkim: sign: %w", err)
	}
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	// Final DKIM-Signature header value (without field name).
	// We do not add CRLF here; caller writes headers with folding.
	return b.String() + "; b=" + sigB64, nil
}

// parseRSAPrivateKey parses an RSA private key from PEM bytes.
func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rk, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an RSA key")
		}
		return rk, nil
	default:
		return nil, fmt.Errorf("unsupported key type: %s", block.Type)
	}
}

// relaxed body canonicalization per RFC 6376 3.4.4:
// - Ignore all trailing WSP at line end
// - Reduce WSP runs to a single SP within lines
// - Remove all trailing empty lines
func dkimCanonicalizeBodyRelaxed(b []byte) []byte {
	lines := splitCRLF(b)
	var out bytes.Buffer
	// Process lines and strip trailing WSP; compress WSP.
	for _, ln := range lines {
		ln = trimCRLF(ln)
		ln = stripTrailingWSP(ln)
		ln = compressWSP(ln).Bytes()
		out.Write(ln)
		out.WriteString("\r\n")
	}
	// Remove trailing empty lines.
	res := out.Bytes()
	for bytes.HasSuffix(res, []byte("\r\n")) {
		// Check if the last line is empty (i.e., ends with two CRLF)
		if len(res) >= 4 &&
			bytes.Equal(res[len(res)-4:], []byte("\r\n\r\n")) {
			res = res[:len(res)-2]
		} else {
			break
		}
	}
	return res
}

// relaxed header canonicalization per RFC 6376 3.4.2
func dkimCanonHeaderRelaxed(name, value string) string {
	lname := strings.ToLower(strings.TrimSpace(name))
	// Unfold: replace CRLF + WSP with a single SP.
	v := unfoldHeader(value)
	// Compress WSP and trim.
	v = strings.TrimSpace(compressWSP([]byte(v)).String())
	return lname + ":" + v
}

// dkimCanonLine canonicalizes a header line.
func dkimCanonLine(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return strings.ToLower(strings.TrimSpace(line))
	}
	return dkimCanonHeaderRelaxed(parts[0], parts[1])
}

// unfoldHeader unfolds a header line.
func unfoldHeader(v string) string {
	v = strings.ReplaceAll(v, "\r\n", "\n")
	var b strings.Builder
	prevSpace := false
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\n' {
			// If next char is WSP, replace sequence with single SP.
			if i+1 < len(v) && (v[i+1] == ' ' || v[i+1] == '\t') {
				if !prevSpace {
					b.WriteByte(' ')
					prevSpace = true
				}
				i++ // skip the WSP char
				continue
			}
			continue
		}
		if c == ' ' || c == '\t' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		} else {
			prevSpace = false
			b.WriteByte(c)
		}
	}
	return b.String()
}

// splitCRLF splits a byte slice into lines.
func splitCRLF(b []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if i+1 < len(b) && b[i] == '\r' && b[i+1] == '\n' {
			lines = append(lines, b[start:i+2])
			i++
			start = i + 1
		}
	}
	if start < len(b) {
		lines = append(lines, append([]byte{}, b[start:]...))
	}
	if len(lines) == 0 {
		return [][]byte{[]byte{}}
	}
	return lines
}

// trimCRLF trims CRLF from a byte slice.
func trimCRLF(b []byte) []byte {
	return bytes.TrimSuffix(bytes.TrimSuffix(b, []byte("\n")), []byte("\r"))
}

// stripTrailingWSP strips trailing whitespace in a byte slice.
func stripTrailingWSP(b []byte) []byte {
	i := len(b) - 1
	for i >= 0 && (b[i] == ' ' || b[i] == '\t') {
		i--
	}
	return b[:i+1]
}

// wsWriter is a writer that compresses whitespace.
type wsWriter struct{ bytes.Buffer }

// compressWSP compresses whitespace in a byte slice.
func compressWSP(b []byte) *wsWriter {
	var w wsWriter
	space := false
	for _, c := range b {
		if c == ' ' || c == '\t' {
			if !space {
				w.WriteByte(' ')
				space = true
			}
		} else {
			w.WriteByte(c)
			space = false
		}
	}
	return &w
}

// headerLookup looks up a header name in a map.
func headerLookup(h map[string]string, name string) string {
	lname := strings.ToLower(name)
	for k := range h {
		if strings.ToLower(k) == lname {
			return k
		}
	}
	return ""
}
