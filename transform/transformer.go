// Package transform defines the Transformer contract used to derive trade
// events from ledger transactions.
package transform

import (
	"time"

	"github.com/fadesany/Stellar-Trade-Processor/event"
	"github.com/stellar/go-stellar-sdk/ingest"
)

// Transformer derives zero or more TradeEvents from a single transaction
// within a ledger. Called once per transaction by the runner.
type Transformer interface {
	Transform(tx ingest.LedgerTransaction, ledgerSeq uint32, closedAt time.Time) ([]event.TradeEvent, error)
}
