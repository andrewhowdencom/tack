package agent

import (
	"context"
	"errors"
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
