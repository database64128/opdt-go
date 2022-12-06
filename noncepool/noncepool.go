package noncepool

import "time"

// NoncePool stores nonces for [retention, 2*retention) to protect against replay attacks
// during the replay window.
//
// NoncePool is not safe for concurrent use.
type NoncePool[T comparable] struct {
	pool      map[T]time.Time
	retention time.Duration
	lastClean time.Time
}

// clean removes expired nonces from the pool.
func (p *NoncePool[T]) clean() {
	if now := time.Now(); now.Sub(p.lastClean) > p.retention {
		for nonce, added := range p.pool {
			if now.Sub(added) > p.retention {
				delete(p.pool, nonce)
			}
		}
		p.lastClean = now
	}
}

// Check returns whether the given nonce is valid (not in the pool).
func (p *NoncePool[T]) Check(nonce T) bool {
	p.clean()
	_, ok := p.pool[nonce]
	return !ok
}

// Add adds the given nonce to the pool.
func (p *NoncePool[T]) Add(nonce T) {
	p.pool[nonce] = time.Now()
}

// New returns a new NoncePool with the given retention.
func New[T comparable](retention time.Duration) *NoncePool[T] {
	return &NoncePool[T]{
		pool:      make(map[T]time.Time),
		retention: retention,
		lastClean: time.Now(),
	}
}
