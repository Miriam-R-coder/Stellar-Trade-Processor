# Understanding the Registry Extension Point

## Why it exists

Classic trades have one encoding, defined by the protocol. Soroban trades have
as many encodings as there are AMMs, and new ones ship without warning.

Without a seam, supporting a second AMM means editing the transformer: another
branch in the decoding path, another set of contract addresses in the same
file, and every consumer waiting for a release of this package before they can
index a protocol they care about. The alternative most people reach for is
forking, and a fork stops receiving fixes the day it is made.

The registry keeps protocol-specific decoding out of the transformer. The
transformer does the work that is the same for every protocol: find
`InvokeHostFunction` operations, read their contract events safely, drop
non-contract events, group what remains by emitting contract, and attach
transaction metadata. An adapter does the work that differs: recognise its own
event and turn it into trades.

That split is why adding an AMM touches one new file plus one `Register` call,
and why you can register your own adapter from your own codebase without
sending a PR at all:

```go
registry := soroban.NewRegistry()
registry.Register(soroban.NewSoroswapAdapter())
registry.Register(myCompany.NewInternalAMMAdapter()) // your code, your repo
transformer := soroban.NewTransformer(registry)
```

`ProtocolAdapter` is an ordinary exported interface, so nothing about that
requires changes here.

## How selection works

`Matches` is a predicate over the contract address, so it can be an exact set
lookup, a prefix test, or anything else. `SoroswapAdapter` uses a set:

```go
func (a *SoroswapAdapter) Matches(contractAddress string) bool {
	_, ok := a.routers[contractAddress]
	return ok
}
```

`AdapterFor` walks adapters in registration order and returns the first match.
Registration order is therefore precedence: if two adapters claim the same
address, the one registered first wins, and the second never sees those events.

Events are grouped by emitting contract before dispatch, so an adapter only
ever receives events from one contract, from one operation. An adapter cannot
see another contract's events in the same transaction.

## What the registry does not do

Being clear about the limits, because "pluggable" gets read as more than it is:

- **One adapter is registered in v1.** Soroswap. The pattern is proven by tests that register it and by a test asserting an unregistered contract's events are ignored, but no second protocol ships today.
- **Registration is code-level only.** There is no plugin loading, no configuration file listing adapters, no `-buildmode=plugin`, and no runtime discovery. Adding an adapter means compiling it in.
- **`Register` is not concurrency-safe.** Build the registry at startup and treat it as read-only. There is no mutex, and `Register` racing with `AdapterFor` is a data race.
- **No adapter versioning.** An adapter either decodes an event or errors. If a protocol changes its event schema, the adapter must handle both shapes itself; the registry has no notion of protocol versions or of which ledger range an adapter applies to.
- **Selection is by contract address only.** An adapter cannot register by event topic, or claim "any contract emitting this topic". If you do not know the addresses, you cannot match.
- **No fallback adapter.** Events from unmatched contracts are dropped silently. That is deliberate, since most contract events are not trades, but it does mean a wrong address in configuration produces zero output and no warning.
- **One adapter per contract.** The first match wins; there is no chaining or merging of results from several adapters for the same contract.

## A consequence worth planning for

Because matching is by address and unmatched contracts are silently ignored,
a Soroswap router redeployment at a new address stops producing events with no
error message. Nothing in the package watches for that.

If you run this in production, the practical check is a liveness signal on your
side: alert when the count of `soroban_amm` events over a period drops to zero
while classic events keep flowing. The
[backfill numbers](../for-library-consumers/backfill-mode-walkthrough.md) give a
sense of the base rate. Six Soroswap swaps in 281 ledgers, against 21,095
classic events, so choose a window long enough that zero is genuinely unusual.
