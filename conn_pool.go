package email

import (
	"container/list"
	"sync"
	"time"
)

// ConnPool is a simple sized pool for client connections.
// It stores opaque connections. Adapter owns the concrete type.
//
// The pool is safe for concurrent use.
type ConnPool struct {
	MaxIdle   int
	IdleTTL   time.Duration
	New       func() (any, error)
	Close     func(any) error
	IsHealthy func(any) bool

	mu    sync.Mutex
	idle  *list.List // list of *poolItem
	inUse int
}

// poolItem is a pool item for a connection.
type poolItem struct {
	conn any
	ts   time.Time
}

// NewConnPool creates a new pool.
//
// Parameters:
//   - maxIdle: The maximum number of idle connections.
//   - idleTTL: The idle timeout.
//   - newFn: The new function.
//   - closeFn: The close function.
//   - isHealthyFn: The is healthy function.
//
// Returns:
//   - *ConnPool: The new pool.
func NewConnPool(
	maxIdle int,
	idleTTL time.Duration,
	newFn func() (any, error),
	closeFn func(any) error,
	isHealthyFn func(any) bool,
) *ConnPool {
	if maxIdle <= 0 {
		maxIdle = 2
	}
	if idleTTL <= 0 {
		idleTTL = 30 * time.Second
	}
	return &ConnPool{
		MaxIdle:   maxIdle,
		IdleTTL:   idleTTL,
		New:       newFn,
		Close:     closeFn,
		IsHealthy: isHealthyFn,
		idle:      list.New(),
	}
}

// Get returns a connection from pool or creates one.
//
// Returns:
//   - any: The connection.
//   - error: An error if the connection creation fails.
func (p *ConnPool) Get() (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Reuse idle if valid.
	for p.idle.Len() > 0 {
		back := p.idle.Back()
		p.idle.Remove(back)
		it := back.Value.(*poolItem)
		if time.Since(it.ts) <= p.IdleTTL && (p.IsHealthy == nil ||
			p.IsHealthy(it.conn)) {
			p.inUse++
			return it.conn, nil
		}
		// Drop stale/unhealthy.
		if p.Close != nil && it.conn != nil {
			_ = p.Close(it.conn)
		}
	}

	// Create new.
	if p.New == nil {
		return nil, nil
	}
	conn, err := p.New()
	if err != nil {
		return nil, err
	}
	p.inUse++
	return conn, nil
}

// Put returns a connection to the pool.
//
// Parameters:
//   - conn: The connection to return to the pool.
func (p *ConnPool) Put(conn any) {
	if conn == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.inUse--
	if p.idle.Len() >= p.MaxIdle {
		if p.Close != nil {
			_ = p.Close(conn)
		}
		return
	}
	p.idle.PushBack(&poolItem{conn: conn, ts: time.Now()})
}

// CloseAll drains the pool and closes all idle connections.
func (p *ConnPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for e := p.idle.Front(); e != nil; e = e.Next() {
		it := e.Value.(*poolItem)
		if p.Close != nil && it.conn != nil {
			_ = p.Close(it.conn)
		}
	}
	p.idle.Init()
}
