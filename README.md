# Stellar Trade Processor

[![CI](https://github.com/fadesany/Stellar-Trade-Processor/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/fadesany/Stellar-Trade-Processor/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go 1.25+](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white)](go.mod)

A Go library and CLI that derives **standardized trade events** — classic DEX
offer fills, path payments, and Soroban AMM swaps — from Stellar ledger data.
It is a sibling to Stellar's
[Token Transfer Processor](https://github.com/stellar/go-stellar-sdk/tree/main/processors/token_transfer)
and follows the same `go-stellar-sdk` ingest conventions: ledgers come from a
`ledgerbackend.LedgerBackend`, transactions are read with
`ingest.LedgerTransactionReader`, and every trade is emitted as one
`event.TradeEvent`, whatever venue it executed on.

## The Problem

The Token Transfer Processor standardizes **token transfers**, but nothing
standardizes **trade and swap extraction**. Every indexer, wallet, and
analytics tool that wants trade data reimplements the same parsing: pulling
claim atoms out of `ManageSellOffer` results, walking path payment hops,
decoding each AMM's contract events, and reconciling all of it into one shape.
That work is duplicated, easy to get subtly wrong, and rarely tested against
malformed input.

The gap and this design are not invented here. In
[stellar · Discussion #1716, "Unified Golang SDK for Stellar"](https://github.com/orgs/stellar/discussions/1716)
(opened by `mollykarcher`), maintainer `sreuland` proposed exactly this
pattern — to

> define generic interfaces for processor and event in the proposed Golang SDK
> to promote consistent approach for consuming network data with stream
> oriented, event driven processor implementations which follow typical
> roles(source, sink, transformer)

This repository is a concrete implementation of those roles for trade data:
`Source` → `Transformer` → `Sink`, with one standardized event type.

## What v1 covers

| Venue | Source | Status |
|---|---|---|
| `classic_dex` | Claim atoms in `ManageSellOffer`, `ManageBuyOffer`, `CreatePassiveSellOffer` results | Fully supported |
| `path_payment` | Claim atoms in `PathPaymentStrictSend`, `PathPaymentStrictReceive` results | Fully supported, one event per hop |
| `soroban_amm` | Soroswap router `swap` events | **Soroswap only** |

Classic fills against liquidity pools (claim atoms of type `LiquidityPool`)
are included under the `classic_dex` or `path_payment` venue with
`Protocol: "liquidity_pool"` and an empty `CounterAccount`.

**Soroswap is the only Soroban AMM decoded in v1.** Soroban protocols plug in
through `soroban.Registry` and the `soroban.ProtocolAdapter` interface. That
registry is an *extension point* for future adapters — Soroswap is currently
the single registered adapter, and no other AMM is supported today.

Out of scope for v1: a hosted API, a database layer, a UI, sinks beyond JSON
lines, and adapters for other AMMs.

## Quick Start

Requires Go 1.25 or newer (the minimum required by `github.com/stellar/go-stellar-sdk`).

```sh
go get github.com/fadesany/Stellar-Trade-Processor
```

Wire a `BackfillSource` → transformers → `JSONLineSink`. This is the wiring
from [`examples/basic/main.go`](examples/basic/main.go):

```go
// 1. Source: a bounded range of ledgers from Stellar RPC.
backend := ledgerbackend.NewRPCLedgerBackend(ledgerbackend.RPCLedgerBackendOptions{RPCServerURL: rpcURL})
src, err := source.NewBackfillSource(ctx, backend, start, end)
if err != nil {
    _ = backend.Close()
    return fmt.Errorf("opening source: %w", err)
}
defer src.Close()

// 2. Transformers: classic DEX/path payments, and Soroban AMMs via the
// protocol registry. Soroswap is the only adapter shipped in v1.
registry := soroban.NewRegistry()
registry.Register(soroban.NewSoroswapAdapter())
transformers := []transform.Transformer{
    classic.NewTransformer(),
    soroban.NewTransformer(registry),
}

// 3. Sink: JSON lines on stdout. Implement sink.Sink to send events elsewhere.
out := sink.NewJSONLineSink(os.Stdout)
defer out.Close()
```

Each ledger from `src.NextLedger(ctx)` is read with
`ingest.NewLedgerTransactionReaderFromLedgerCloseMeta`, every transaction is
passed to each transformer's `Transform`, and the resulting events go to
`out.Write`. Run the complete example with:

```sh
go run ./examples/basic --rpc-url=https://mainnet.sorobanrpc.com \
  --start-ledger=64446600 --end-ledger=64446700
```

## CLI usage

The CLI reads ledgers from a
[Stellar RPC](https://developers.stellar.org/docs/data/apis/rpc/providers)
endpoint and writes one JSON object per trade to stdout; logs go to stderr.

```sh
go install github.com/fadesany/Stellar-Trade-Processor/cmd/processor@latest
```

Backfill a bounded, inclusive ledger range:

```sh
processor --mode=backfill \
  --rpc-url=https://mainnet.sorobanrpc.com \
  --start-ledger=64446600 --end-ledger=64446880 > trades.jsonl
```

Stream live ledgers, starting from the latest closed ledger (Ctrl+C to stop):

```sh
processor --mode=stream --rpc-url=https://mainnet.sorobanrpc.com
```

| Flag | Default | Description |
|---|---|---|
| `--mode` | (required) | `stream` or `backfill` |
| `--rpc-url` | (required) | Stellar RPC endpoint |
| `--start-ledger` | latest ledger in stream mode | First ledger; required for backfill |
| `--end-ledger` | | Last ledger, inclusive; backfill only |
| `--network-passphrase` | mainnet passphrase | Use `Test SDF Network ; September 2015` for testnet |
| `--soroswap-router` | mainnet router `CAG5LRYQ5JVEUI5TEID72EYOVX44TTUJT5BQR2J6J77FH65PCCFAJDDH` | Comma-separated router contract addresses |
| `--buffer-size` | SDK default | RPC backend ledger buffer size |

Backfill ranges must fall within the RPC server's ledger retention window. Any
transformer error stops the run and reports the ledger it occurred in.

## TradeEvent schema

Every trade, from any venue, is emitted as one `event.TradeEvent`, seen from
the perspective of `Account`.

| Field | Type | Meaning |
|---|---|---|
| `Venue` | `event.Venue` | `classic_dex`, `path_payment`, or `soroban_amm` |
| `Protocol` | `string` | Empty for orderbook fills; `liquidity_pool` for classic pool fills; `soroswap` for Soroswap swaps |
| `BaseAsset` | `event.Asset` | The asset `Account` gave up |
| `CounterAsset` | `event.Asset` | The asset `Account` received |
| `BaseAmount` | `string` | Amount of `BaseAsset` given, decimal string — never a float |
| `CounterAmount` | `string` | Amount of `CounterAsset` received, decimal string |
| `Price` | `string` | `CounterAmount / BaseAmount`, decimal string truncated to 7 places |
| `Account` | `string` | Whose trade this is: the operation source for classic trades, the swap's `to` address for Soroswap |
| `CounterAccount` | `string` | The other party; empty for AMM and liquidity pool fills |
| `LedgerSequence` | `uint32` | Ledger containing the trade |
| `TxHash` | `string` | Hex-encoded transaction hash |
| `OpIndex` | `int` | Index of the operation within the transaction |
| `ClosedAt` | `time.Time` | Ledger close time |

`event.Asset`:

| Field | Type | Meaning |
|---|---|---|
| `Type` | `string` | `native`, `credit_alphanum4`, `credit_alphanum12`, or `soroban_token` |
| `Code` | `string` | Asset code for credit assets; empty otherwise |
| `Issuer` | `string` | Issuing account for credit assets; empty otherwise |
| `ContractAddress` | `string` | Token contract `C...` strkey for `soroban_token` |

Example output:

```json
{"Venue":"path_payment","Protocol":"","BaseAsset":{"Type":"native","Code":"","Issuer":"","ContractAddress":""},"CounterAsset":{"Type":"credit_alphanum12","Code":"BITCOIN","Issuer":"GDRR...KUMI","ContractAddress":""},"BaseAmount":"15.0000000","CounterAmount":"0.1250000","Price":"0.0083333","Account":"GB74...7FNB","CounterAccount":"GDRW...WITT","LedgerSequence":64446884,"TxHash":"84179ee4...","OpIndex":0,"ClosedAt":"2026-09-15T23:03:10Z"}
```

Notes on semantics:

- Classic amounts are 7-decimal fixed-point strings. Soroswap amounts are the
  raw `i128` integers in each token's smallest unit, and `Price` is the ratio
  of those raw amounts, because token decimals are not present in the event.
- Amounts and prices are always computed with integer or `big.Rat` arithmetic;
  no floating point is used anywhere.
- Only successful transactions produce events.
- A `Transformer` never panics on malformed input — it returns an error.

### Soroswap event schema

The adapter decodes the router event published by
[`soroswap/core` `contracts/router/src/event.rs`](https://github.com/soroswap/core/blob/main/contracts/router/src/event.rs):
topics `("SoroswapRouter", "swap")`, data
`SwapEvent { path: Vec<Address>, amounts: Vec<i128>, to: Address }`, where
`amounts[i]` is the amount of `path[i]` at each step. It is emitted by both
`swap_exact_tokens_for_tokens` and `swap_tokens_for_exact_tokens`, and one
`TradeEvent` is emitted per hop.

## Packages

| Package | Contents |
|---|---|
| `event` | `TradeEvent`, `Asset`, `Venue` |
| `source` | `Source` interface, `BackfillSource`, `StreamingSource` |
| `sink` | `Sink` interface, `JSONLineSink` |
| `transform` | `Transformer` interface |
| `transform/classic` | Offer fill and path payment extraction |
| `transform/soroban` | Soroban transformer, `Registry`, `ProtocolAdapter`, `SoroswapAdapter` |

## Known limitations

- Swaps that call a Soroswap pair contract directly, without the router, are
  not captured: the pair's own `swap` event carries no token addresses.
- Soroswap amounts are not scaled by token decimals, and Stellar Asset
  Contract tokens are not mapped back to their classic asset code and issuer.
- Classic liquidity pool fills do not record the pool ID.
- Streaming keeps no cursor; resume a stopped stream with `--start-ledger`.

## Contributing

Contributions are welcome.

1. **Find something to work on.** Browse the
   [open issues](https://github.com/fadesany/Stellar-Trade-Processor/issues),
   or start with one labelled
   [`good first issue`](https://github.com/fadesany/Stellar-Trade-Processor/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22).
   Issues are also labelled `complexity:low`, `complexity:medium`, or
   `complexity:high`. Comment on an issue before starting so work isn't
   duplicated.
2. **Branch naming.** Use `feat/...`, `fix/...`, `docs/...`, `test/...`, or
   `refactor/...` — for example `feat/aqua-adapter`.
3. **Commit format.** [Conventional commits](https://www.conventionalcommits.org/):
   `type(scope): description`, e.g. `feat(transform/soroban): add Aqua adapter`.

**Pull request checklist:**

- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` is clean
- [ ] `go test ./...` passes, with table-driven tests covering new behaviour
- [ ] New transformer or adapter code has a malformed-input test proving it
      returns an error rather than panicking
- [ ] `gofmt` applied, and exported identifiers have doc comments
- [ ] No floating point in any amount or price path

CI runs build, vet, and `go test ./... -race -cover` on every push and pull
request to `main`.

## Security

Please report vulnerabilities privately through the repository's **Security**
tab, not in a public issue. See [SECURITY.md](SECURITY.md) for what is in
scope and what to expect. This code is **unaudited**.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).

## Contributors

[![Contributors](https://contrib.rocks/image?repo=fadesany/Stellar-Trade-Processor)](https://github.com/fadesany/Stellar-Trade-Processor/graphs/contributors)
