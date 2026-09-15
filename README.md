# Stellar Trade Processor

A Go library and thin CLI that derives standardized trade events from Stellar
ledger data. It is a sibling to Stellar's
[Token Transfer Processor](https://github.com/stellar/go-stellar-sdk/tree/main/processors/token_transfer)
and follows the same `go-stellar-sdk` ingest conventions: ledgers come from a
`ledgerbackend.LedgerBackend`, transactions are read with
`ingest.LedgerTransactionReader`, and every trade is emitted as one
`event.TradeEvent`.

## What v1 extracts

| Venue | Source | Notes |
|---|---|---|
| `classic_dex` | Claimed offers in `ManageSellOffer`, `ManageBuyOffer`, `CreatePassiveSellOffer` results | One event per claim atom |
| `path_payment` | Claimed offers in `PathPaymentStrictSend`, `PathPaymentStrictReceive` results | One event per hop (claim atom) |
| `soroban_amm` | Soroswap router `swap` events | One event per hop of the swap path |

Classic fills against liquidity pools (claim atoms of type `LiquidityPool`) are
included under the `classic_dex` or `path_payment` venue with
`Protocol: "liquidity_pool"` and an empty `CounterAccount`.

Soroswap is the **only** Soroban AMM decoded in v1. Soroban protocols are
plugged in through `soroban.Registry` and the `soroban.ProtocolAdapter`
interface; that registry is an extension point for future adapters, and
Soroswap is currently the single adapter registered.

Out of scope for v1: a hosted API, a database layer, a UI, sinks other than
JSON lines, and adapters for other AMMs.

## Install

Requires Go 1.25 or newer (the minimum required by `github.com/stellar/go-stellar-sdk`).

```sh
# CLI
go install github.com/fadesany/Stellar-Trade-Processor/cmd/processor@latest

# Library
go get github.com/fadesany/Stellar-Trade-Processor
```

## CLI usage

The CLI reads ledgers from a [Stellar RPC](https://developers.stellar.org/docs/data/apis/rpc/providers)
endpoint and writes one JSON object per trade to stdout. Logs go to stderr.

Backfill a bounded, inclusive range:

```sh
processor --mode=backfill \
  --rpc-url=https://mainnet.sorobanrpc.com \
  --start=64446600 --end=64446700 > trades.jsonl
```

Stream live ledgers (from `--start`, or from the latest ledger when omitted);
stop with Ctrl+C:

```sh
processor --mode=stream --rpc-url=https://mainnet.sorobanrpc.com
```

| Flag | Default | Description |
|---|---|---|
| `--mode` | (required) | `stream` or `backfill` |
| `--rpc-url` | (required) | Stellar RPC endpoint |
| `--network-passphrase` | mainnet passphrase | Use `Test SDF Network ; September 2015` for testnet |
| `--start` | latest ledger in stream mode | First ledger; required for backfill |
| `--end` | | Last ledger, inclusive; backfill only |
| `--soroswap-router` | mainnet router `CAG5LRYQ5JVEUI5TEID72EYOVX44TTUJT5BQR2J6J77FH65PCCFAJDDH` | Comma-separated router contract addresses (override for testnet) |
| `--buffer-size` | SDK default | RPC backend ledger buffer size |

Backfill ranges must fall within the RPC server's ledger retention window.
Any transformer error stops the run and reports the ledger it occurred in.

## Library usage

```go
backend := ledgerbackend.NewRPCLedgerBackend(ledgerbackend.RPCLedgerBackendOptions{
    RPCServerURL: "https://mainnet.sorobanrpc.com",
})
src, err := source.NewBackfillSource(ctx, backend, start, end) // or source.NewStreamingSource
if err != nil {
    return err
}
defer src.Close()

registry := soroban.NewRegistry()
registry.Register(soroban.NewSoroswapAdapter()) // mainnet router by default
transformers := []transform.Transformer{
    classic.NewTransformer(),
    soroban.NewTransformer(registry),
}

out := sink.NewJSONLineSink(os.Stdout) // implement sink.Sink for Kafka, a DB, etc.
defer out.Close()

for {
    lcm, err := src.NextLedger(ctx)
    if errors.Is(err, io.EOF) {
        break
    }
    // read transactions with ingest.NewLedgerTransactionReaderFromLedgerCloseMeta,
    // call each transformer's Transform, and pass the events to out.Write
}
```

A complete, runnable version is in [`examples/basic`](examples/basic/main.go),
and the CLI wiring is in [`cmd/processor`](cmd/processor/main.go).

### Packages

| Package | Contents |
|---|---|
| `event` | `TradeEvent`, `Asset`, `Venue` |
| `source` | `Source` interface, `BackfillSource`, `StreamingSource` |
| `sink` | `Sink` interface, `JSONLineSink` |
| `transform` | `Transformer` interface |
| `transform/classic` | Offer fill and path payment extraction |
| `transform/soroban` | Soroban transformer, `Registry`, `ProtocolAdapter`, `SoroswapAdapter` |

## Event semantics

```json
{"Venue":"path_payment","Protocol":"","BaseAsset":{"Type":"native","Code":"","Issuer":"","ContractAddress":""},"CounterAsset":{"Type":"credit_alphanum12","Code":"BITCOIN","Issuer":"GDRR...KUMI","ContractAddress":""},"BaseAmount":"15.0000000","CounterAmount":"0.1250000","Price":"0.0083333","Account":"GB74...7FNB","CounterAccount":"GDRW...WITT","LedgerSequence":64446884,"TxHash":"84179ee4...","OpIndex":0,"ClosedAt":"2026-09-15T23:03:10Z"}
```

- **Perspective.** Each event is seen from `Account`: `BaseAsset`/`BaseAmount`
  is what `Account` gave, `CounterAsset`/`CounterAmount` is what it received,
  and `Price = CounterAmount / BaseAmount`.
- **Classic trades.** `Account` is the operation's source account (or the
  transaction source), i.e. the taker. `CounterAccount` is the offer owner for
  orderbook fills. Amounts are 7-decimal fixed-point strings.
- **Soroswap trades.** `Account` is the swap's `to` address from the router
  event. Assets are `soroban_token` identified by `ContractAddress` only.
  Amounts are the raw i128 integers in each token's smallest unit, and `Price`
  is the ratio of those raw amounts, because token decimals are not present in
  the event.
- **No floats.** Amounts and prices are always decimal strings computed with
  integer or `big.Rat` arithmetic; prices are truncated to 7 decimal places.
- Only successful transactions produce events.

### Soroswap event schema

The adapter decodes the router event published by
[`soroswap/core` `contracts/router/src/event.rs`](https://github.com/soroswap/core/blob/main/contracts/router/src/event.rs):
topics `("SoroswapRouter", "swap")`, data
`SwapEvent { path: Vec<Address>, amounts: Vec<i128>, to: Address }`, where
`amounts[i]` is the amount of `path[i]` at each step. It is emitted by both
`swap_exact_tokens_for_tokens` and `swap_tokens_for_exact_tokens`.

## Known limitations

- Swaps that call a Soroswap pair contract directly, without the router, are
  not captured: the pair's own `swap` event carries no token addresses.
- Soroswap amounts are not scaled by token decimals, and Stellar Asset
  Contract tokens are not mapped back to their classic asset code and issuer.
- Streaming uses the RPC backend's polling. The processor keeps no cursor, so
  resume a stopped stream by passing `--start`.

## Development

```sh
go vet ./...
go test ./...
```

Transformer tests are table-driven over hand-built XDR fixtures and include
malformed inputs, which must return errors rather than panic.
