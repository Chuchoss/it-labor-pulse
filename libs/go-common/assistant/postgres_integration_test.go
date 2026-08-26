//go:build integration

package assistant

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
)

func TestPostgresPreferencesRoundTripAcrossConnections(t *testing.T) {
	_ = godotenv.Load("../../../.env")
	databaseURL := os.Getenv("ASSISTANT_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("ASSISTANT_TEST_DATABASE_URL or DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn1, err := pgx.Connect(ctx, databaseURL)
	require.NoError(t, err)
	defer conn1.Close(context.Background())
	conn2, err := pgx.Connect(ctx, databaseURL)
	require.NoError(t, err)
	defer conn2.Close(context.Background())

	subject := fmt.Sprintf("assistant-integration-%d", time.Now().UnixNano())
	repo1 := NewPostgresRepository(conn1)
	userID, err := repo1.EnsureUser(ctx, subject)
	require.NoError(t, err)
	defer func() {
		_, _ = conn1.Exec(context.Background(), `DELETE FROM assistant_users WHERE id = $1::uuid`, userID)
	}()

	want := PreferenceRecord{
		Note:         "synthetic integration profile",
		HardCriteria: map[string]any{"approved_roles": []any{"backend"}},
		SoftCriteria: map[string]any{},
		Weights:      map[string]float64{"salary": 1},
	}
	saved, err := repo1.SavePreferences(ctx, userID, "integration-request", want)
	require.NoError(t, err)
	require.Equal(t, 1, saved.Version)
	require.NotEmpty(t, saved.ID)

	loaded, err := NewPostgresRepository(conn2).CurrentPreferences(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, saved.Version, loaded.Version)
	require.Equal(t, want.Note, loaded.Note)
	require.Equal(t, want.HardCriteria, loaded.HardCriteria)
	require.Equal(t, want.SoftCriteria, loaded.SoftCriteria)
	require.Equal(t, want.Weights, loaded.Weights)
}
