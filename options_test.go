package email

import (
    "testing"

    "github.com/aatuh/email/types"
)

func TestOptionsApply(t *testing.T) {
    var cfg SendConfig
    rl := NewTokenBucket(5, 2)
    pool := NewConnPool(1, 0, nil, nil, nil)
    hooks := &types.Hooks{}
    dkim := types.DKIMConfig{Domain: "example.com", Selector: "sel", KeyPEM: []byte("k")}

    opts := []Option{
        WithListUnsubscribe("<mailto:unsub@example.com>"),
        WithRateLimit(rl),
        WithPool(pool),
        WithHooks(hooks),
        WithDKIM(dkim),
    }
    for _, o := range opts { o(&cfg) }

    if cfg.ListUnsub == "" || cfg.Rate != rl || cfg.Pool != pool || cfg.Hooks != hooks || cfg.DKIM == nil {
        t.Fatalf("options not applied: %+v", cfg)
    }
    if cfg.DKIM.Domain != "example.com" || cfg.DKIM.Selector != "sel" {
        t.Fatalf("dkim option not set correctly: %+v", cfg.DKIM)
    }
}

