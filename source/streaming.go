package source

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// StreamingSource yields ledgers from a live backend indefinitely, blocking
// until each next ledger closes.
type StreamingSource struct {
	backend ledgerbackend.LedgerBackend
	next    uint32
}

// NewStreamingSource prepares backend for an unbounded range and returns a
// Source that follows the network. If start is 0, streaming begins at the
// backend's latest closed ledger. The source takes ownership of backend and
// closes it on Close.
//
// Backends that require PrepareRange before GetLatestLedgerSequence (such as
// the RPC backend) should be given an explicit start ledger.
func NewStreamingSource(ctx context.Context, backend ledgerbackend.LedgerBackend, start uint32) (*StreamingSource, error) {
	if start == 0 {
		latest, err := backend.GetLatestLedgerSequence(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting latest ledger sequence: %w", err)
		}
		start = latest
	}
	if err := backend.PrepareRange(ctx, ledgerbackend.UnboundedRange(start)); err != nil {
		return nil, fmt.Errorf("preparing unbounded range from %d: %w", start, err)
	}
	return &StreamingSource{backend: backend, next: start}, nil
}

// NextLedger blocks until the next ledger is available and returns it. It
// only returns an error when the backend fails or ctx is cancelled.
func (s *StreamingSource) NextLedger(ctx context.Context) (xdr.LedgerCloseMeta, error) {
	lcm, err := s.backend.GetLedger(ctx, s.next)
	if err != nil {
		return xdr.LedgerCloseMeta{}, fmt.Errorf("getting ledger %d: %w", s.next, err)
	}
	s.next++
	return lcm, nil
}

// Close closes the underlying ledger backend.
func (s *StreamingSource) Close() error {
	if err := s.backend.Close(); err != nil {
		return fmt.Errorf("closing ledger backend: %w", err)
	}
	return nil
}
