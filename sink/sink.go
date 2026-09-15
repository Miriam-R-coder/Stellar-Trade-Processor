// Package sink defines destinations for derived trade events.
package sink

import "github.com/fadesany/Stellar-Trade-Processor/event"

// Sink receives batches of trade events. Implementations decide how events
// are persisted or forwarded.
type Sink interface {
	// Write delivers a batch of events. An empty batch is valid.
	Write(events []event.TradeEvent) error
	// Close flushes and releases any resources held by the sink.
	Close() error
}
