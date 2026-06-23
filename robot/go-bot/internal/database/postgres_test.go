//go:build unit

package database

import (
	"context"
	"testing"

	"trading/robot/go-bot/internal/config"

	"github.com/stretchr/testify/require"
)

func TestNewDBPool(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name                string
		dbConfig            config.DatabaseConfig
		expectedErrContains string
	}{
		{
			name: "Parse Config Error",
			dbConfig: config.DatabaseConfig{
				Host:    "localhost",
				SSLMode: "invalid-mode",
			},
			expectedErrContains: "failed to parse database config",
		},
		{
			name: "Constructor Success",
			dbConfig: config.DatabaseConfig{
				Host:    "localhost",
				Port:    5432,
				User:    "test_user",
				DBName:  "test_db",
				SSLMode: "disable",
			},
			expectedErrContains: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := NewDBPool(ctx, tc.dbConfig)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				require.Nil(t, pool)
				require.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
				require.NotNil(t, pool)
				pool.Close()
			}
		})
	}
}
