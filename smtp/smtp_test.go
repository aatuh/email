package smtp

import "testing"

func TestIsTransient(t *testing.T) {
    cases := []struct{
        err error
        want bool
    }{
        {errString("421 try again later"), true},
        {errString("4xx mailbox full"), true},
        {errString("Timeout while reading"), true},
        {errString("connection reset by peer"), true},
        {errString("permanent 550 user unknown"), false},
        {errString("syntax error"), false},
    }
    for _, c := range cases {
        if got := isTransient(c.err); got != c.want {
            t.Fatalf("isTransient(%q)=%v want %v", c.err, got, c.want)
        }
    }
}

type errString string
func (e errString) Error() string { return string(e) }

