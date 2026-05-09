package idempotency

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFormatInterval guards against accidental change in Postgres interval format.
func TestFormatInterval(t *testing.T) {
	got := formatInterval(24 * time.Hour)
	require.Contains(t, got, "s")
}
