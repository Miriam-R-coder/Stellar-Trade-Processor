package soroban

import (
	"fmt"
	"time"

	"github.com/fadesany/Stellar-Trade-Processor/event"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// Transformer extracts Soroban AMM trades by routing each contract's events
// to the ProtocolAdapter registered for that contract address.
type Transformer struct {
	registry *Registry
}

// NewTransformer returns a Soroban Transformer backed by registry. A nil
// registry is treated as empty, producing no events.
func NewTransformer(registry *Registry) *Transformer {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Transformer{registry: registry}
}

// Transform returns the trades decoded from the contract events emitted by
// each InvokeHostFunction operation in tx. Failed transactions yield no
// events. Events from contracts without a matching adapter are ignored.
func (t *Transformer) Transform(tx ingest.LedgerTransaction, ledgerSeq uint32, closedAt time.Time) ([]event.TradeEvent, error) {
	if !tx.Successful() {
		return nil, nil
	}
	txHash := tx.Hash.HexString()
	ops := tx.Envelope.Operations()
	txSource := tx.Envelope.SourceAccount().ToAccountId().Address()

	var trades []event.TradeEvent
	for i, op := range ops {
		if op.Body.Type != xdr.OperationTypeInvokeHostFunction {
			continue
		}

		events, err := operationEvents(tx, i)
		if err != nil {
			return nil, fmt.Errorf("transaction %s op %d: %w", txHash, i, err)
		}
		if len(events) == 0 {
			continue
		}

		account := txSource
		if op.SourceAccount != nil {
			account = op.SourceAccount.ToAccountId().Address()
		}
		ctx := TxContext{
			TxHash:         txHash,
			LedgerSequence: ledgerSeq,
			ClosedAt:       closedAt,
			OpIndex:        i,
			SourceAccount:  account,
		}

		order, byContract, err := groupByContract(events)
		if err != nil {
			return nil, fmt.Errorf("transaction %s op %d: %w", txHash, i, err)
		}
		for _, contract := range order {
			adapter, ok := t.registry.AdapterFor(contract)
			if !ok {
				continue
			}
			extracted, err := adapter.ExtractTrades(ctx, byContract[contract])
			if err != nil {
				return nil, fmt.Errorf("transaction %s op %d: %s adapter for contract %s: %w", txHash, i, adapter.Name(), contract, err)
			}
			trades = append(trades, extracted...)
		}
	}
	return trades, nil
}

// operationEvents returns the contract events emitted by operation opIndex,
// validating the meta shape first so malformed input errors instead of
// panicking inside the SDK accessors.
func operationEvents(tx ingest.LedgerTransaction, opIndex int) ([]xdr.ContractEvent, error) {
	meta := tx.UnsafeMeta
	switch meta.V {
	case 1, 2:
		return nil, nil
	case 3:
		if meta.V3 == nil {
			return nil, fmt.Errorf("transaction meta v3 has no body")
		}
	case 4:
		if meta.V4 == nil {
			return nil, fmt.Errorf("transaction meta v4 has no body")
		}
		if n := len(meta.V4.Operations); n != 0 && opIndex >= n {
			return nil, fmt.Errorf("transaction meta has %d operations, no entry for op %d", n, opIndex)
		}
	default:
		return nil, fmt.Errorf("unsupported transaction meta version %d", meta.V)
	}

	events, err := tx.GetContractEventsForOperation(uint32(opIndex))
	if err != nil {
		return nil, fmt.Errorf("reading contract events: %w", err)
	}
	return events, nil
}

// groupByContract partitions contract-type events by emitting contract strkey,
// returning the addresses in first-seen order. System and diagnostic events,
// and events without a contract ID, are dropped.
func groupByContract(events []xdr.ContractEvent) ([]string, map[string][]xdr.ContractEvent, error) {
	var order []string
	byContract := make(map[string][]xdr.ContractEvent)
	for _, ev := range events {
		if ev.Type != xdr.ContractEventTypeContract || ev.ContractId == nil {
			continue
		}
		id := *ev.ContractId
		address, err := strkey.Encode(strkey.VersionByteContract, id[:])
		if err != nil {
			return nil, nil, fmt.Errorf("encoding contract id: %w", err)
		}
		if _, seen := byContract[address]; !seen {
			order = append(order, address)
		}
		byContract[address] = append(byContract[address], ev)
	}
	return order, byContract, nil
}
