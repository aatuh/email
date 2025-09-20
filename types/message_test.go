package types

import (
    "reflect"
    "testing"
)

func TestMessageValidate(t *testing.T) {
    m := Message{}
    if err := m.Validate(); err == nil {
        t.Fatalf("expected error for missing From and recipients")
    }
    m.From = Address{Mail: "from@example.com"}
    if err := m.Validate(); err == nil {
        t.Fatalf("expected error for no recipients")
    }
    m.To = []Address{{Mail: "to@example.com"}}
    if err := m.Validate(); err == nil {
        t.Fatalf("expected error for no body or attachments")
    }
    m.Plain = []byte("hi")
    if err := m.Validate(); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}

func TestRecipientList(t *testing.T) {
    m := Message{
        To:  []Address{{Mail: "a@example.com"}},
        Cc:  []Address{{Mail: "b@example.com"}},
        Bcc: []Address{{Mail: "c@example.com"}},
    }
    got := m.RecipientList()
    want := []string{"a@example.com", "b@example.com", "c@example.com"}
    if !reflect.DeepEqual(got, want) {
        t.Fatalf("recipients mismatch:\n got=%v\nwant=%v", got, want)
    }
}

func TestAddressParseAndString(t *testing.T) {
    a, err := ParseAddress("Ada Lovelace <ada@example.com>")
    if err != nil {
        t.Fatalf("parse: %v", err)
    }
    if a.Name != "Ada Lovelace" || a.Mail != "ada@example.com" {
        t.Fatalf("unexpected address: %+v", a)
    }
    if s := a.String(); s != "\"Ada Lovelace\" <ada@example.com>" && s != "Ada Lovelace <ada@example.com>" {
        t.Fatalf("unexpected String: %q", s)
    }

    if _, err := ParseAddress("not-an-email"); err == nil {
        t.Fatalf("expected error for invalid address")
    }
}

func TestParseAddressList(t *testing.T) {
    xs, err := ParseAddressList([]string{"Ada <ada@example.com>", "bob@example.com"})
    if err != nil {
        t.Fatalf("list: %v", err)
    }
    if len(xs) != 2 || xs[0].Mail != "ada@example.com" || xs[1].Mail != "bob@example.com" {
        t.Fatalf("unexpected list: %+v", xs)
    }

    xs, err = ParseAddressList([]string{"Ada <ada@example.com>, Bob <bob@example.com>"})
    if err != nil || len(xs) != 2 {
        t.Fatalf("combined list failed: %v, %+v", err, xs)
    }

    xs, err = ParseAddressList(nil)
    if err != nil || xs != nil {
        t.Fatalf("nil input: %v, %+v", err, xs)
    }
}
