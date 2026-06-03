package circuitbreaker

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var errTest = errors.New("test error")

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := New(Config{Threshold: 3, Cooldown: time.Minute})

	for range 2 {
		require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)
	}
	require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)
	require.ErrorIs(t, cb.Execute(func() error { return nil }), ErrOpen)
}

func TestCircuitBreaker_RecoversFromHalfOpen(t *testing.T) {
	cb := New(Config{Threshold: 2, Cooldown: 20 * time.Millisecond})
	require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)
	require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)
	require.ErrorIs(t, cb.Execute(func() error { return nil }), ErrOpen)

	time.Sleep(25 * time.Millisecond)

	// checkIsOpened moves Open -> HalfOpen but still blocks this call.
	require.ErrorIs(t, cb.Execute(func() error { return nil }), ErrOpen)
	require.NoError(t, cb.Execute(func() error { return nil }))
	require.NoError(t, cb.Execute(func() error { return nil }))
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := New(Config{Threshold: 1, Cooldown: 20 * time.Millisecond})
	require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)

	time.Sleep(25 * time.Millisecond)

	require.ErrorIs(t, cb.Execute(func() error { return errTest }), ErrOpen)
	require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)
	require.ErrorIs(t, cb.Execute(func() error { return nil }), ErrOpen)
}

func TestCircuitBreaker_SuccessDecaysFailureCount(t *testing.T) {
	cb := New(Config{Threshold: 3, Cooldown: time.Minute})
	require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)
	require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)
	require.NoError(t, cb.Execute(func() error { return nil }))
	require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)
	require.NoError(t, cb.Execute(func() error { return nil }))
	require.NoError(t, cb.Execute(func() error { return nil }))
}

func TestNew_Defaults(t *testing.T) {
	cb := New(Config{})
	for range 4 {
		require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)
	}
	require.ErrorIs(t, cb.Execute(func() error { return errTest }), errTest)
	require.ErrorIs(t, cb.Execute(func() error { return nil }), ErrOpen)
}
