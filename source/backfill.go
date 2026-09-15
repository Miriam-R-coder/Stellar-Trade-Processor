package source

import (
	"context"
	"fmt"
	"io"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// BackfillSource yields every ledger in a bounded, inclusive range and then
// returns io.EOF.
type BackfillSource struct {
	backend ledgerbackend.LedgerBackend
	next    uint32
	end     uint32
	done    bool
}

// NewBackfillSource prepares backend for the inclusive range [start, end] and
// returns a Source that walks it in order. The source takes ownership of
// backend and closes it on Close.
func NewBackfillSource(ctx context.Context, backend ledgerbackend.LedgerBackend, start, end uint32) (*BackfillSource, error) {
	if start == 0 {
		return nil, fmt.Errorf("backfill start ledger must be greater than 0")
	}
	if end < start {
		return nil, fmt.Errorf("backfill end ledger %d is before start ledger %d", end, start)
	}
	if err := backend.PrepareRange(ctx, ledgerbackend.BoundedRange(start, end)); err != nil {
		return nil, fmt.Errorf("preparing bounded range [%d, %d]: %w", start, end, err)
	}
	return &BackfillSource{backend: backend, next: start, end: end}, nil
}

// NextLedger returns the next ledger in the range, or io.EOF once the final
// ledger has been returned.
func (s *BackfillSource) NextLedger(ctx context.Context) (xdr.LedgerCloseMeta, error) {
	if s.done {
		return xdr.LedgerCloseMeta{}, io.EOF
	}
	lcm, err := s.backend.GetLedger(ctx, s.next)
	if err != nil {
		return xdr.LedgerCloseMeta{}, fmt.Errorf("getting ledger %d: %w", s.next, err)
	}
	if s.next == s.end {
		s.done = true
	} else {
		s.next++
	}
	return lcm, nil
}

// Close closes the underlying ledger backend.
func (s *BackfillSource) Close() error {
	if err := s.backend.Close(); err != nil {
		return fmt.Errorf("closing ledger backend: %w", err)
	}
	return nil
}
