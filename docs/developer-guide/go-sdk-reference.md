# Go SDK Reference

Integration patterns that come up repeatedly. Each one is complete enough to
paste into a file and adapt.

## Import paths

```go
import (
	"github.com/fadesany/Stellar-Trade-Processor/event"
	"github.com/fadesany/Stellar-Trade-Processor/sink"
	"github.com/fadesany/Stellar-Trade-Processor/source"
	"github.com/fadesany/Stellar-Trade-Processor/transform"
	"github.com/fadesany/Stellar-Trade-Processor/transform/classic"
	"github.com/fadesany/Stellar-Trade-Processor/transform/soroban"
)
```

## Writing a custom Sink

`Sink` has two methods, and no dependency on the ingest SDK. This one batches
events in memory and flushes when the batch is full or `Close` is called,
which is the shape most databases and message queues want:

```go
// BatchSink buffers events and flushes them in fixed-size batches.
type BatchSink struct {
	batchSize int
	pending   []event.TradeEvent
	flush     func([]event.TradeEvent) error
}

// NewBatchSink returns a Sink that calls flush every batchSize events.
func NewBatchSink(batchSize int, flush func([]event.TradeEvent) error) *BatchSink {
	if batchSize < 1 {
		batchSize = 1
	}
	return &BatchSink{batchSize: batchSize, flush: flush}
}

// Write appends events and flushes whole batches.
func (s *BatchSink) Write(events []event.TradeEvent) error {
	s.pending = append(s.pending, events...)
	for len(s.pending) >= s.batchSize {
		batch := s.pending[:s.batchSize]
		if err := s.flush(batch); err != nil {
			return fmt.Errorf("flushing batch of %d: %w", len(batch), err)
		}
		s.pending = s.pending[s.batchSize:]
	}
	return nil
}

// Close flushes whatever is left.
func (s *BatchSink) Close() error {
	if len(s.pending) == 0 {
		return nil
	}
	if err := s.flush(s.pending); err != nil {
		return fmt.Errorf("flushing final batch of %d: %w", len(s.pending), err)
	}
	s.pending = nil
	return nil
}
```

Using it:

```go
out := NewBatchSink(500, func(batch []event.TradeEvent) error {
	return insertRows(ctx, db, batch) // your code
})
defer out.Close()
```

Two things to get right. **Always call `Close`**, or the last partial batch is
lost. And if your sink can be retried, make writes idempotent: a trade is
identified by `TxHash`, `OpIndex`, and its position within that operation,
since one operation can produce several events with the same hash and index.

## Filtering by asset pair

Filtering is ordinary Go over the returned slice. Assets compare cleanly
because `event.Asset` is a comparable struct:

```go
var (
	xlm  = event.Asset{Type: event.AssetTypeNative}
	usdc = event.Asset{
		Type:   event.AssetTypeCreditAlphanum4,
		Code:   "USDC",
		Issuer: "GAHJHC4RQLDUZUFZDCSSE4MUZC4QYL3ELSSQX5M6Y2NSU5DROUKQKZVN",
	}
)

// involvesPair reports whether ev trades a and b in either direction.
func involvesPair(ev event.TradeEvent, a, b event.Asset) bool {
	return (ev.BaseAsset == a && ev.CounterAsset == b) ||
		(ev.BaseAsset == b && ev.CounterAsset == a)
}

func filterPair(events []event.TradeEvent, a, b event.Asset) []event.TradeEvent {
	var out []event.TradeEvent
	for _, ev := range events {
		if involvesPair(ev, a, b) {
			out = append(out, ev)
		}
	}
	return out
}
```

Match on the issuer, not just the code. Anyone can issue an asset called
`USDC`, and comparing codes alone silently mixes them together.

Wrapping a sink is the tidiest place to filter, because it works for both the
CLI-style loop and your own:

```go
// FilteringSink passes through only events satisfying keep.
type FilteringSink struct {
	inner sink.Sink
	keep  func(event.TradeEvent) bool
}

func (s *FilteringSink) Write(events []event.TradeEvent) error {
	kept := events[:0:0]
	for _, ev := range events {
		if s.keep(ev) {
			kept = append(kept, ev)
		}
	}
	return s.inner.Write(kept)
}

func (s *FilteringSink) Close() error { return s.inner.Close() }
```

Other filters worth knowing: `ev.Venue == event.VenueSorobanAMM` for AMM trades
only, `ev.Protocol == "liquidity_pool"` for classic pool fills, and
`ev.Account == myAccount || ev.CounterAccount == myAccount` for one account's
activity.

## Combining classic and Soroban transformers

Both implement `transform.Transformer`, so a slice is all the composition
needed:

```go
registry := soroban.NewRegistry()
registry.Register(soroban.NewSoroswapAdapter())

transformers := []transform.Transformer{
	classic.NewTransformer(),
	soroban.NewTransformer(registry),
}
```

Then run every transformer over every transaction:

```go
// eventsFromLedger returns all trades in one ledger, in transaction order.
func eventsFromLedger(
	passphrase string,
	lcm xdr.LedgerCloseMeta,
	transformers []transform.Transformer,
) ([]event.TradeEvent, error) {
	seq := lcm.LedgerSequence()
	closedAt := lcm.ClosedAt()

	reader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(passphrase, lcm)
	if err != nil {
		return nil, fmt.Errorf("creating transaction reader for ledger %d: %w", seq, err)
	}
	defer reader.Close()

	var events []event.TradeEvent
	for {
		tx, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading transaction in ledger %d: %w", seq, err)
		}
		for _, t := range transformers {
			txEvents, err := t.Transform(tx, seq, closedAt)
			if err != nil {
				return nil, fmt.Errorf("ledger %d: %w", seq, err)
			}
			events = append(events, txEvents...)
		}
	}
	return events, nil
}
```

That is `processLedger` from `cmd/processor/main.go`, which is the same
function the CLI uses.

Both transformers are stateless, so one instance can serve many ledgers. They
are safe to reuse; the `Registry` is read-only after startup. If you process
ledgers concurrently, give each goroutine its own `ingest.LedgerTransactionReader`,
since a reader is not safe to share.

Wiring only one transformer is fine. Drop the Soroban one and you skip contract
event decoding entirely, which is worth doing if you only care about classic
markets.

## Choosing a ledger backend

Anything implementing `ledgerbackend.LedgerBackend` works. The RPC backend
needs no local infrastructure:

```go
backend := ledgerbackend.NewRPCLedgerBackend(ledgerbackend.RPCLedgerBackendOptions{
	RPCServerURL: "https://mainnet.sorobanrpc.com",
	BufferSize:   20,               // 0 uses the SDK default of 10
	HttpClient:   &http.Client{},   // optional: timeouts, auth headers, proxies
})
```

`HttpClient` is the hook for an endpoint that needs an API key header or a
custom timeout. The CLI does not expose it, so an authenticated endpoint either
carries its key in the URL or needs the library path.

For deep history beyond an RPC server's retention window, use a captive core
backend from the ingest SDK instead. The rest of the pipeline does not change.
