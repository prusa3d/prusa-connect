package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// An approved update keeps a call the operator took away out, keeps one the
// operator left in, and adds one the release before did not ask for, which
// the update then says it gave (ADR-20261027-pq5 rule 2).
func TestAnUpdateKeepsACallTheOperatorTookAwayOut(t *testing.T) {
	t.Parallel()

	const (
		kept    = "com.github.acme.slicer"
		removed = "com.github.acme.stats"
		fresh   = "com.github.acme.queue"
	)

	given, added := keptLines(
		[]string{kept, removed, fresh}, // the new release asks
		[]string{kept, removed},        // the release before asked
		[]string{kept},                 // the declaration holds
		nil,
		callRequest,
	)
	require.Equal(t, []string{kept, fresh}, given)
	require.Equal(t, []string{callRequest(fresh)}, added)

	// The operator took every call away. The update gives none back, and the
	// host still starts a module that requires routes.call.
	given, added = keptLines([]string{kept}, []string{kept}, nil, nil, callRequest)
	require.Empty(t, given)
	require.Empty(t, added)
}
