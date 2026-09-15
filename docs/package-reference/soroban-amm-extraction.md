# Soroban AMM Extraction and the Protocol Registry

Soroban swaps are not operation results. They are contract events, and each AMM
defines its own event schema. That is why this side of the package has a
registry: the transformer handles the parts common to every protocol, and an
adapter handles one protocol's event format.

**v1 registers exactly one adapter: Soroswap.** The registry is an extension
point. It is not support for other AMMs, and nothing else is decoded today.

## What the transformer does

`soroban.Transformer` walks the transaction's operations and, for each
`InvokeHostFunction` operation:

1. Reads that operation's contract events, checking the meta shape first so a malformed transaction returns an error instead of panicking. Meta v3 keeps events under `SorobanMeta`; meta v4 keeps them per operation.
2. Drops events that are not contract events or have no contract ID.
3. Groups the rest by the emitting contract address (a `C...` strkey), keeping first-seen order.
4. Asks the registry for an adapter matching each contract address. No match means the events are ignored.
5. Calls the adapter and collects its events.

Failed transactions produce nothing. An adapter error is wrapped with the
transaction hash, the operation index, the adapter name, and the contract
address.

## The ProtocolAdapter interface

```go
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
```

Adapters receive a `TxContext` because a contract event alone does not know
where it came from:

```go
// TxContext carries the transaction-level metadata an adapter needs to build
// complete TradeEvents from contract events alone.
type TxContext struct {
	TxHash         string
	LedgerSequence uint32
	ClosedAt       time.Time
	OpIndex        int
	SourceAccount  string
}
```

## The Registry

```go
// Registry holds the protocol adapters available to the Soroban transformer.
// It is intended to be populated once at startup and treated as read-only
// afterwards; Register is not safe to call concurrently with AdapterFor.
type Registry struct {
	adapters []ProtocolAdapter
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
```

Wiring it up takes two lines:

```go
registry := soroban.NewRegistry()
registry.Register(soroban.NewSoroswapAdapter())
```

`AdapterFor` is a linear scan. With one adapter that is irrelevant; with fifty
it would still be cheap next to XDR decoding.

## SoroswapAdapter

The adapter decodes the Soroswap router's swap event, verified against
[`soroswap/core` `contracts/router/src/event.rs`](https://github.com/soroswap/core/blob/main/contracts/router/src/event.rs):

- **Topics:** `("SoroswapRouter", "swap")`. The first topic comes from a Rust `&str`, which soroban-sdk encodes as an `ScString`; the adapter accepts a symbol there too.
- **Data:** `SwapEvent { path: Vec<Address>, amounts: Vec<i128>, to: Address }`, encoded as a map keyed by field-name symbols. The adapter looks fields up by name, never by position.
- **Semantics:** `amounts[i]` is the amount of `path[i]` at step `i`. `amounts[0]` is the input, the last element is the output. Both `swap_exact_tokens_for_tokens` and `swap_tokens_for_exact_tokens` emit this same event.

A path of N tokens is N-1 hops, and the adapter emits one `TradeEvent` per hop:

```go
for hop := 0; hop+1 < len(swap.path); hop++ {
	baseAmount, counterAmount := swap.amounts[hop], swap.amounts[hop+1]
	trades = append(trades, event.TradeEvent{
		Venue:         event.VenueSorobanAMM,
		Protocol:      ProtocolSoroswap,
		BaseAsset:     event.Asset{Type: event.AssetTypeSorobanToken, ContractAddress: swap.path[hop]},
		CounterAsset:  event.Asset{Type: event.AssetTypeSorobanToken, ContractAddress: swap.path[hop+1]},
		BaseAmount:    baseAmount.String(),
		CounterAmount: counterAmount.String(),
		Price:         new(big.Rat).SetFrac(counterAmount, baseAmount).FloatString(7),
		Account:       swap.to,
		// ... plus LedgerSequence, TxHash, OpIndex, ClosedAt from TxContext
	})
}
```

`Account` is the swap's `to` address. `CounterAccount` is empty, because the
counterparty is a pool.

By default the adapter matches the mainnet router,
`CAG5LRYQ5JVEUI5TEID72EYOVX44TTUJT5BQR2J6J77FH65PCCFAJDDH`, taken from
soroswap/core's `public/mainnet.contracts.json`. Pass addresses to
`NewSoroswapAdapter("C...")` for testnet or a custom deployment.

## A real decoded swap

Mainnet ledger 64446708, transaction
`01c9b5324f26294b579a76befd0f3de5f770cdc49473f983e083f328b411a2ea`. The path
had three tokens, so two events came out:

| Hop | Base token | Base amount | Counter token | Counter amount | Price |
| --- | --- | --- | --- | --- | --- |
| 1 | `CD25MNVT...MYTVG5JY` | 3138770434 | `CCW67TSZ...EO7SJMI75` | 14146404 | 0.0045070 |
| 2 | `CCW67TSZ...EO7SJMI75` | 14146404 | `CAS3J7GY...VH34XOWMA` | 80061031 | 5.6594617 |

Hop 1's output is hop 2's input, which is what makes them one route rather than
two unrelated trades.

## Errors and limits

`ExtractTrades` returns an error when the event body is not version 0, when the
data is not a map, when a map key is not a symbol, when `path`, `amounts` or
`to` is missing, when `path` has fewer than two entries, when `path` and
`amounts` differ in length, when a path element is not a contract address, and
when an amount is not a positive `i128`. Events whose topics do not match a
router swap are skipped silently, which is how `add`, `remove` and token
transfer events in the same operation are ignored.

Current limits worth knowing:

- **Amounts are raw.** Token decimals are not read, so a token with 6 decimals and one with 7 are not directly comparable, and `Price` is a ratio of raw units ([issue #3](https://github.com/fadesany/Stellar-Trade-Processor/issues/3)).
- **Router-only.** Swaps that call a pair contract directly emit only the pair's event, which carries no token addresses, so they are not decoded ([issue #2](https://github.com/fadesany/Stellar-Trade-Processor/issues/2)).
- **One adapter.** Adding another means writing it; see [Adding a New AMM Protocol Adapter](../for-contributors/adding-a-new-amm-protocol-adapter.md) and [issue #1](https://github.com/fadesany/Stellar-Trade-Processor/issues/1).
