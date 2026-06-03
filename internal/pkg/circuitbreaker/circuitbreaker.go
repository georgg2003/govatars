package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// ErrOpen is returned when the circuit is open and the call is fast-failed.
var ErrOpen = errors.New("circuit breaker is open, request blocked")

// State is the circuit breaker phase.
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

// Config tunes failure counting before the circuit opens.
type Config struct {
	// Threshold is how many failures are tolerated before opening (default 5).
	Threshold int
	// Cooldown is how long the circuit stays open before a half-open probe (default 30s).
	Cooldown time.Duration
}

// CircuitBreaker wraps calls to an external dependency with closed / open / half-open logic.
type CircuitBreaker struct {
	mu              sync.Mutex
	state           State
	failureCount    int
	threshold       int
	cooldown        time.Duration
	lastStateChange time.Time
}

// New builds a circuit breaker. Zero Config fields get safe defaults.
func New(cfg Config) *CircuitBreaker {
	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = 5
	}
	cooldown := cfg.Cooldown
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &CircuitBreaker{
		state:           StateClosed,
		threshold:       threshold,
		cooldown:        cooldown,
		lastStateChange: time.Now(),
	}
}

func (cb *CircuitBreaker) checkIsOpened() error {
	if cb.state != StateOpen {
		return nil
	}
	if time.Since(cb.lastStateChange) > cb.cooldown {
		cb.mu.Lock()
		cb.state = StateHalfOpen
		cb.lastStateChange = time.Now()
		cb.mu.Unlock()
	}
	return ErrOpen
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.checkIsOpened(); err != nil {
		return err
	}
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failureCount++
		if cb.state == StateHalfOpen || cb.failureCount >= cb.threshold {
			cb.state = StateOpen
			cb.lastStateChange = time.Now()
		}
		return err
	}

	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.failureCount = 0
	} else if cb.state == StateClosed && cb.failureCount > 0 {
		cb.failureCount--
	}

	return nil
}
