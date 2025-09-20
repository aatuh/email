package email

import (
    "sync/atomic"
    "testing"
    "time"
)

func TestTokenBucketWait(t *testing.T) {
    tb := NewTokenBucket(10, 2) // 10 tokens/s, burst 2

    // Consume initial burst quickly without significant blocking.
    start := time.Now()
    tb.Wait()
    tb.Wait()
    if time.Since(start) > 20*time.Millisecond {
        t.Fatalf("initial burst took too long")
    }

    // Third wait should block roughly ~100ms or less given 10/s.
    start = time.Now()
    tb.Wait()
    if time.Since(start) < 50*time.Millisecond {
        t.Fatalf("expected some blocking for third token")
    }

    // Parallel waits should each eventually proceed.
    var done int32
    for i := 0; i < 3; i++ {
        go func() { tb.Wait(); atomic.AddInt32(&done, 1) }()
    }
    time.Sleep(400 * time.Millisecond)
    if atomic.LoadInt32(&done) < 2 { // allow some slack
        t.Fatalf("expected at least two goroutines to acquire tokens")
    }
}

