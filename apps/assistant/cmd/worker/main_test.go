package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDurationEnvAcceptsResponsiveWorkerInterval(t *testing.T) {
	t.Setenv("ASSISTANT_WORKER_INTERVAL", "5s")

	require.Equal(t, 5*time.Second, durationEnv("ASSISTANT_WORKER_INTERVAL", 30*time.Minute))
}

func TestDurationEnvRejectsUnsafeOrInvalidInterval(t *testing.T) {
	for _, value := range []string{"invalid", "0s", "500ms"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("ASSISTANT_WORKER_INTERVAL", value)

			require.Equal(t, 30*time.Minute, durationEnv("ASSISTANT_WORKER_INTERVAL", 30*time.Minute))
		})
	}
}
