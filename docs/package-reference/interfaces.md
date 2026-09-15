# Source, Sink, and Transformer Interfaces

Three interfaces hold the pipeline together. Each is small enough to implement
in one file.

## Source

```go
// Source yields ledgers to be processed, one at a time, in order.
type Source interface {
	// NextLedger blocks until the next ledger is available (streaming mode)
	// or returns io.EOF once the bounded range is exhausted (backfill mode).
	NextLedger(ctx context.Context) (xdr.LedgerCloseMeta, error)
	// Close releases the underlying ledger backend.
	Close() error
}
```

Two implementations ship: `BackfillSource` and `StreamingSource`. Both wrap a
`ledgerbackend.LedgerBackend` from the ingest SDK, take ownership of it, and
close it in `Close`.

### Implementation example: BackfillSource

`NewBackfillSource` validates the range, prepares it on the backend, and
returns a source that walks it:

```go
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
```

`NextLedger` returns each ledger in turn, and `io.EOF` after the last one has
been handed out:

```go
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
```

`StreamingSource` is the same shape without the end bound: it prepares an
unbounded range and increments forever. If `start` is 0 it asks the backend for
its latest ledger, which works for backends that allow that before
`PrepareRange`. The RPC backend does not, so the CLI resolves the starting
ledger through the RPC client instead.

## Sink

```go
// Sink receives batches of trade events. Implementations decide how events
// are persisted or forwarded.
type Sink interface {
	// Write delivers a batch of events. An empty batch is valid.
	Write(events []event.TradeEvent) error
	// Close flushes and releases any resources held by the sink.
	Close() error
}
```

**`JSONLineSink` is the only Sink shipped in v1.** For Kafka, Postgres, S3, an
HTTP endpoint, or anything else, you write your own. That is two methods, and
the interface has no dependency on the ingest SDK.

### Implementation example: JSONLineSink

```go
// NewJSONLineSink returns a sink writing JSON lines to w. If w is nil, events
// are written to os.Stdout. Output is flushed after every Write.
func NewJSONLineSink(w io.Writer) *JSONLineSink {
	if w == nil {
		w = os.Stdout
	}
	buf := bufio.NewWriter(w)
	return &JSONLineSink{buf: buf, enc: json.NewEncoder(buf), dst: w}
}

// Write encodes each event as one JSON line and flushes the batch.
func (s *JSONLineSink) Write(events []event.TradeEvent) error {
	for i := range events {
		if err := s.enc.Encode(&events[i]); err != nil {
			return fmt.Errorf("encoding trade event %s op %d: %w", events[i].TxHash, events[i].OpIndex, err)
		}
	}
	if err := s.buf.Flush(); err != nil {
		return fmt.Errorf("flushing json lines: %w", err)
	}
	return nil
}
```

`Close` flushes, and closes the destination only when it is an `io.Closer` that
is not `os.Stdout` or `os.Stderr`, so passing stdout in does not close the
process's own output stream.

## Transformer

```go
// Transformer derives zero or more TradeEvents from a single transaction
// within a ledger. Called once per transaction by the runner.
type Transformer interface {
	Transform(tx ingest.LedgerTransaction, ledgerSeq uint32, closedAt time.Time) ([]event.TradeEvent, error)
}
```

The contract that matters: **a Transformer returns an error rather than
panicking**, whatever the input. Malformed XDR, a result union with a missing
body, an unknown claim atom type, a contract event whose data is not the shape
the protocol documents: all of these produce errors. Every transformer in the
repository has table-driven tests covering malformed cases for exactly this
reason.

`classic.Transformer` and `soroban.Transformer` are separate types on purpose.
They read different parts of a transaction, and merging them would put contract
event decoding and operation result parsing in one object.

### Implementation example: classic.Transformer

```go
// Transformer extracts trades from classic offer and path payment operations.
// It is stateless and safe for concurrent use.
type Transformer struct{}

// NewTransformer returns a classic Transformer.
func NewTransformer() *Transformer {
	return &Transformer{}
}
```

`Transform` skips failed transactions, pairs operations with their results, and
turns every claim atom into an event:

```go
func (t *Transformer) Transform(tx ingest.LedgerTransaction, ledgerSeq uint32, closedAt time.Time) ([]event.TradeEvent, error) {
	if !tx.Successful() {
		return nil, nil
	}
	txHash := tx.Hash.HexString()

	ops := tx.Envelope.Operations()
	results, ok := tx.Result.OperationResults()
	if !ok {
		return nil, fmt.Errorf("transaction %s: missing operation results", txHash)
	}
	if len(results) != len(ops) {
		return nil, fmt.Errorf("transaction %s: %d operations but %d results", txHash, len(ops), len(results))
	}
	// ... one event per claim atom, see Classic DEX Extraction
}
```

`soroban.Transformer` has the same signature but takes a `*Registry` at
construction: `soroban.NewTransformer(registry)`.

## Composing them

The runner calls every transformer on every transaction and concatenates the
results:

```go
transformers := []transform.Transformer{
	classic.NewTransformer(),
	soroban.NewTransformer(registry),
}
```

Order within a transaction follows the slice order, so classic events precede
Soroban events for the same transaction. Across transactions, order follows the
ledger.
