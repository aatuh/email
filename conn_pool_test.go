package email

import (
    "errors"
    "sync/atomic"
    "testing"
    "time"
)

func TestConnPoolBasicReuseAndTTL(t *testing.T) {
    var created, closed int32
    p := NewConnPool(2, 50*time.Millisecond,
        func() (any, error) { atomic.AddInt32(&created, 1); return new(int), nil },
        func(a any) error { atomic.AddInt32(&closed, 1); return nil },
        func(a any) bool { return true },
    )

    c1, err := p.Get()
    if err != nil || c1 == nil { t.Fatalf("get1: %v %v", c1, err) }
    c2, _ := p.Get()
    if created != 2 { t.Fatalf("expected 2 created, got %d", created) }
    p.Put(c1)
    p.Put(c2)

    // Reuse within TTL should not create new.
    c3, _ := p.Get()
    if created != 2 { t.Fatalf("unexpected creation on reuse: %d", created) }
    p.Put(c3)

    // After TTL, idle becomes stale and is closed on next Get.
    time.Sleep(60 * time.Millisecond)
    _, _ = p.Get()
    if closed == 0 { t.Fatalf("expected stale idle close") }
}

func TestConnPoolMaxIdleEvicts(t *testing.T) {
    var closed int32
    p := NewConnPool(1, time.Minute,
        func() (any, error) { return new(int), nil },
        func(a any) error { atomic.AddInt32(&closed, 1); return nil },
        func(a any) bool { return true },
    )
    c1, _ := p.Get()
    c2, _ := p.Get()
    p.Put(c1)
    p.Put(c2) // should be closed because MaxIdle=1
    if atomic.LoadInt32(&closed) != 1 {
        t.Fatalf("expected one close due to MaxIdle, got %d", closed)
    }
}

func TestConnPoolNewMayBeNil(t *testing.T) {
    p := NewConnPool(1, time.Minute, nil, nil, nil)
    c, err := p.Get()
    if err != nil || c != nil {
        t.Fatalf("expected nil conn and no error, got %v %v", c, err)
    }
}

func TestConnPoolNewError(t *testing.T) {
    p := NewConnPool(1, time.Minute,
        func() (any, error) { return nil, errors.New("boom") }, nil, nil)
    if _, err := p.Get(); err == nil {
        t.Fatalf("expected error from New()")
    }
}

