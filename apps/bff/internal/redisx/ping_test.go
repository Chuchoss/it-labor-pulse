package redisx

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

func TestOpenPing(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	p, err := Open("redis://" + mr.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })

	require.NoError(t, p.Ping(context.Background()))
}

func TestOpenInvalidURL(t *testing.T) {
	t.Parallel()

	_, err := Open("://not-a-url")
	require.Error(t, err)
}

func TestOpenWithMissingTLSCAFile(t *testing.T) {
	t.Parallel()

	_, err := OpenWithTLSCA("rediss://:pass@example.com:6380/0", "nonexistent-ca.pem")
	require.Error(t, err)
}

func TestPingDown(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	p, err := Open("redis://" + mr.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })

	mr.Close()
	require.Error(t, p.Ping(context.Background()))
}
