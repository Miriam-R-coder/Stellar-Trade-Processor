// Package source provides ledger sources that feed the trade processor.
package source

import (
	"context"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Source yields ledgers to be processed, one at a time, in order.
type Source interface {
	// NextLedger blocks until the next ledger is available (streaming mode)
	// or returns io.EOF once the bounded range is exhausted (backfill mode).
	NextLedger(ctx context.Context) (xdr.LedgerCloseMeta, error)
	// Close releases the underlying ledger backend.
	Close() error
}
