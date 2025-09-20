package internal

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/sha256"
    "crypto/x509"
    "encoding/base64"
    "encoding/pem"
    "strings"
    "testing"

    "github.com/aatuh/email/types"
)

func TestBuildDKIMSignatureIncludesBH(t *testing.T) {
    // For an empty body, canonicalized representation is "\r\n" per relaxed rules.
    body := []byte("\r\n")
    sum := sha256.Sum256(body)
    bh := base64.StdEncoding.EncodeToString(sum[:])

    // Generate a small RSA key for testing.
    key, err := rsa.GenerateKey(rand.Reader, 1024)
    if err != nil {
        t.Fatalf("generate key: %v", err)
    }
    keyDER := x509.MarshalPKCS1PrivateKey(key)
    keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})

    headers := map[string]string{
        "From": "no-reply@example.com",
        "To":   "to@example.com",
        "Date": "Mon, 01 Jan 2000 00:00:00 +0000",
    }
    cfg := types.DKIMConfig{Domain: "example.com", Selector: "sel", KeyPEM: keyPEM, Headers: []string{"from", "to", "date"}}
    sig, err := BuildDKIMSignature(headers, []byte{}, cfg)
    if err != nil {
        t.Fatalf("dkim sign: %v", err)
    }
    if !strings.Contains(sig, "bh="+bh) {
        t.Fatalf("signature missing expected bh: %s", sig)
    }
    if !strings.Contains(sig, "d=example.com") || !strings.Contains(sig, "s=sel") {
        t.Fatalf("missing domain/selector: %s", sig)
    }
}
