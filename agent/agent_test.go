package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time assertion that Agent is a valid user of conduit.Conduit.
// (Agent itself does not implement Conduit, but it orchestrates them.)

// mockConduit is a test double for conduit.Conduit.
type mockConduit struct {
	block time.Duration
	err   error
}

func (m *mockConduit) Run(ctx context.Context) error {
	select {
	case <-time.After(m.block):
		return m.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestNew(t *testing.T) {
	// We cannot create a real *session.Manager without more dependencies,
	// so use nil for this test (Agent only stores the reference).
	a := New(nil)
	require.NotNil(t, a)
	assert.Nil(t, a.mgr)
}

func TestAgent_Add(t *testing.T) {
	a := New(nil)
	c := &mockConduit{}
	a.Add(c)
	require.Len(t, a.conduits, 1)
	assert.Equal(t, c, a.conduits[0])
}

func TestAgent_Run_NoConduits(t *testing.T) {
	a := New(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := a.Run(ctx)
	assert.NoError(t, err)
}

func TestAgent_Run_Success(t *testing.T) {
	a := New(nil)
	a.Add(&mockConduit{block: 50 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := a.Run(ctx)
	assert.NoError(t, err)
}

func TestAgent_Run_MultipleSuccess(t *testing.T) {
	a := New(nil)
	a.Add(&mockConduit{block: 50 * time.Millisecond})
	a.Add(&mockConduit{block: 100 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := a.Run(ctx)
	assert.NoError(t, err)
}

func TestAgent_Run_ContextCancel(t *testing.T) {
	a := New(nil)
	a.Add(&mockConduit{block: 5 * time.Second})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := a.Run(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestAgent_Run_ErrorPropagates(t *testing.T) {
	a := New(nil)
	a.Add(&mockConduit{block: 50 * time.Millisecond, err: errors.New("boom")})
	a.Add(&mockConduit{block: 5 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := a.Run(ctx)
	assert.EqualError(t, err, "boom")
}

// immediateErrorConduit returns an error immediately without waiting.
type immediateErrorConduit struct {
	err error
}

func (i *immediateErrorConduit) Run(ctx context.Context) error {
	return i.err
}

// cancellationObserverConduit blocks until the context is cancelled and
// records whether it observed the cancellation.
type cancellationObserverConduit struct {
	observed *atomic.Bool
}

func (c *cancellationObserverConduit) Run(ctx context.Context) error {
	<-ctx.Done()
	c.observed.Store(true)
	return ctx.Err()
}

func TestAgent_Run_CancellationPropagation(t *testing.T) {
	var observed atomic.Bool

	quickErr := &immediateErrorConduit{err: errors.New("quick error")}
	observer := &cancellationObserverConduit{observed: &observed}

	a := New(nil)
	a.Add(quickErr)
	a.Add(observer)

	err := a.Run(context.Background())
	require.Error(t, err)
	assert.Equal(t, "quick error", err.Error())
	assert.True(t, observed.Load(), "observer conduit should have observed context cancellation")
}

func TestAgent_Run_ConcurrentTiming(t *testing.T) {
	start := time.Now()

	slow := &mockConduit{block: 200 * time.Millisecond}
	fast := &mockConduit{block: 100 * time.Millisecond}

	a := New(nil)
	a.Add(slow)
	a.Add(fast)

	err := a.Run(context.Background())
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, elapsed, 250*time.Millisecond, "conduits should run concurrently, not serially")
	assert.GreaterOrEqual(t, elapsed, 190*time.Millisecond, "should wait for slowest conduit")
}
