package email

import (
    "testing"
    "time"
)

func TestExponentialBackoffBasic(t *testing.T) {
    b := ExponentialBackoff(4, 100*time.Millisecond, 250*time.Millisecond, false)
    // i=0 returns immediately
    if d, ok := b.Next(0); !ok || d != 0 {
        t.Fatalf("first attempt should be ok with 0 delay, got %v %v", d, ok)
    }
    // Ranges are probabilistic; just ensure within expected bounds.
    d1, ok := b.Next(1)
    if !ok || d1 < 50*time.Millisecond || d1 > 100*time.Millisecond {
        t.Fatalf("attempt1 out of half-jitter range: %v", d1)
    }
    d2, ok := b.Next(2)
    if !ok || d2 < 100*time.Millisecond || d2 > 200*time.Millisecond {
        t.Fatalf("attempt2 out of half-jitter range: %v", d2)
    }
    d3, ok := b.Next(3)
    if !ok || d3 < 125*time.Millisecond || d3 > 250*time.Millisecond {
        t.Fatalf("attempt3 should be capped at max: %v", d3)
    }
    if _, ok := b.Next(4); ok {
        t.Fatalf("attempts should be exhausted at i=4")
    }
}

