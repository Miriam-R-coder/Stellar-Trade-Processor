// Package soroban derives trade events from Soroban AMM contract events.
// Protocol-specific decoding lives behind the ProtocolAdapter interface;
// adapters are registered once at startup in a Registry.
package soroban

import (
	"time"

	"github.com/fadesany/Stellar-Trade-Processor/event"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// TxContext carries the transaction-level metadata an adapter needs to build
// complete TradeEvents from contract events alone.
type TxContext struct {
	// TxHash is the hex-encoded transaction hash.
	TxHash string
	// LedgerSequence is the sequence of the ledger containing the transaction.
	LedgerSequence uint32
	// ClosedAt is the ledger close time.
	ClosedAt time.Time
	// OpIndex is the index of the InvokeHostFunction operation that emitted
	// the events.
	OpIndex int
	// SourceAccount is the strkey of the operation's source account (or the
	// transaction source when the operation does not override it).
	SourceAccount string
}

// ProtocolAdapter maps one AMM protocol's contract events to TradeEvents.
// Adapters are selected by the address of the contract that emitted the
// events.
type ProtocolAdapter interface {
	// Name returns the protocol identifier written to TradeEvent.Protocol.
	Name() string
	// Matches reports whether events emitted by contractAddress (a C... strkey)
	// belong to this protocol.
	Matches(contractAddress string) bool
	// ExtractTrades decodes the trades described by events, all of which were
	// emitted by one matching contract within a single operation. Events that
	// are not trades are ignored; malformed trade events return an error.
	ExtractTrades(tx TxContext, events []xdr.ContractEvent) ([]event.TradeEvent, error)
}

// Registry holds the protocol adapters available to the Soroban transformer.
// It is intended to be populated once at startup and treated as read-only
// afterwards; Register is not safe to call concurrently with AdapterFor.
type Registry struct {
	adapters []ProtocolAdapter
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds an adapter. Adapters are consulted in registration order, so
// the first registered adapter matching a contract address wins. A nil
// adapter is ignored.
func (r *Registry) Register(a ProtocolAdapter) {
	if a == nil {
		return
	}
	r.adapters = append(r.adapters, a)
}

// AdapterFor returns the first registered adapter whose Matches reports true
// for contractAddress.
func (r *Registry) AdapterFor(contractAddress string) (ProtocolAdapter, bool) {
	for _, a := range r.adapters {
		if a.Matches(contractAddress) {
			return a, true
		}
	}
	return nil, false
}
