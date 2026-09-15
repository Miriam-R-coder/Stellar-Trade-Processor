# Adding a New AMM Protocol Adapter

Adding an AMM means writing one type that implements `ProtocolAdapter` and
registering it. You do not touch the transformer, the source, the sink, or the
event schema. Use `transform/soroban/soroswap.go` as the template and
`transform/soroban/soroswap_test.go` as the test template.

[Issue #1](https://github.com/fadesany/Stellar-Trade-Processor/issues/1) tracks
an Aqua adapter if you want a concrete one to pick up.

## Step 1: verify the event schema before writing any code

This is the step people skip, and it is the one that produces silently wrong
data. Do not infer field names from another AMM, from a block explorer's
rendering, or from memory.

Find the protocol's contract source or current developer docs, and establish:

- The **topics** the swap event publishes, in order, and their types. A Rust `&str` literal becomes an `ScString`; `symbol_short!("swap")` becomes an `ScSymbol`. Both appear in practice, so check which one the contract uses.
- The **data payload**: field names, types, and whether it is a map, a vector, or a single value. A `#[contracttype]` struct with named fields becomes a map keyed by field-name symbols.
- Which **amounts** the event carries, and whether they are input or output amounts, gross or net of fees.
- How **multi-hop routes** are represented: one event per hop, or one event listing the whole path.
- Which contract **emits** it: a router, a pair, or both. If only a pair emits it, check whether the event names the tokens. The Soroswap pair event does not, which is why pair-direct swaps are unsupported today.
- The **deployed addresses**, from the protocol's own published list.

For Soroswap this meant reading `contracts/router/src/event.rs` in soroswap/core
and `public/mainnet.contracts.json` for the address. The adapter's doc comment
records what was found, and your PR should cite your sources the same way.

## Step 2: implement the interface

```go
type ProtocolAdapter interface {
	Name() string
	Matches(contractAddress string) bool
	ExtractTrades(tx TxContext, events []xdr.ContractEvent) ([]event.TradeEvent, error)
}
```

A minimal skeleton, following `SoroswapAdapter`:

```go
const ProtocolAqua = "aqua"

type AquaAdapter struct {
	contracts map[string]struct{}
}

// NewAquaAdapter returns an adapter matching the given contract addresses.
// With no arguments it matches the mainnet deployment.
func NewAquaAdapter(contracts ...string) *AquaAdapter {
	if len(contracts) == 0 {
		contracts = []string{AquaMainnetRouter}
	}
	set := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		set[c] = struct{}{}
	}
	return &AquaAdapter{contracts: set}
}

func (a *AquaAdapter) Name() string { return ProtocolAqua }

func (a *AquaAdapter) Matches(contractAddress string) bool {
	_, ok := a.contracts[contractAddress]
	return ok
}
```

Keep addresses configurable. Hard-coding one mainnet address makes the adapter
untestable against testnet.

## Step 3: decode defensively

Two rules, both enforced by the tests:

**Never let the SDK's generated accessors touch a nil arm.** `ScVal.MustMap()`
and friends dereference the pointer and crash the process. Check the field
yourself:

```go
if data.Type != xdr.ScValTypeScvMap || data.Map == nil || *data.Map == nil {
	return swapEvent{}, fmt.Errorf("data is %s, want map", data.Type)
}
```

**Look fields up by name, not position.** Map ordering is a host detail, not
part of the contract's interface:

```go
fields := make(map[string]xdr.ScVal, len(**data.Map))
for _, entry := range **data.Map {
	if entry.Key.Type != xdr.ScValTypeScvSymbol || entry.Key.Sym == nil {
		return swapEvent{}, fmt.Errorf("map key is %s, want symbol", entry.Key.Type)
	}
	fields[string(*entry.Key.Sym)] = entry.Val
}
```

Ignore events you do not understand. Events that are not swaps must be skipped
silently, not treated as errors: token transfer events and liquidity events
show up in the same operation.

## Step 4: build the events

Fill in transaction metadata from `TxContext`, and follow the conventions the
rest of the package uses:

- One `TradeEvent` per hop. A path of N tokens produces N-1 events.
- `BaseAsset` is what the trader gave; `CounterAsset` is what they received.
- `Venue` is `event.VenueSorobanAMM`, `Protocol` is your `Name()`.
- `CounterAccount` is empty when the counterparty is a pool.
- No floats. Amounts are `big.Int` rendered with `String()`; prices are `new(big.Rat).SetFrac(counter, base).FloatString(7)`.
- Reject non-positive amounts rather than emitting a zero or negative trade.

## Step 5: register it

```go
registry := soroban.NewRegistry()
registry.Register(soroban.NewSoroswapAdapter())
registry.Register(soroban.NewAquaAdapter())
```

Add a CLI flag for the addresses, matching `--soroswap-router`, so operators
can point it at testnet.

## Step 6: write table-driven tests

The existing tests build XDR fixtures by hand, with no network access. Reuse
the helpers in `soroswap_test.go`: `scSymbol`, `scString`, `scContract`,
`scAccount`, `scI128`, `scVec`, `scMap`, `contractEvent`, `metaV3`, `metaV4`
and `invokeTx`.

Cover at least:

- A **successful single-hop swap**, asserting every field of the resulting event.
- A **multi-hop swap**, asserting one event per hop and that hop N's output equals hop N+1's input.
- An **event from an unregistered contract**, which must be ignored.
- A **non-swap event from your contract**, which must be ignored.
- At least three **malformed inputs**: a length mismatch, a missing field, and a wrong value type. Each must return an error, and the test must assert the error text, not just that an error happened.

A malformed case that panics fails the suite, which is the point.

## Step 7: update the docs

- Add the protocol to the venue table in `README.md` and in `docs/README.md`.
- Add it to [Soroban AMM Extraction](../package-reference/soroban-amm-extraction.md).
- Do not describe the package as supporting protocols that are not registered.

## PR checklist

- [ ] Event schema verified against contract source or official docs, linked in the PR
- [ ] Addresses configurable, mainnet default exported as a constant
- [ ] No floats in any amount or price path
- [ ] Nil arms checked before every union access
- [ ] Fields looked up by name
- [ ] Table-driven tests including malformed inputs that return errors
- [ ] `go build ./...`, `go vet ./...`, `go test ./... -race` all clean
- [ ] Docs updated, with no claim of support beyond what is registered
