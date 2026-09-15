# Environment and Configuration

## There are no environment variables

This package reads no environment variables. There is no `.env` file, no
config file, no `STELLAR_RPC_URL`, and no hidden defaults picked up from the
environment. Nothing is silently configured behind your back.

That is a deliberate consequence of what this is: a library plus a thin CLI,
not a hosted service. The CLI is configured entirely by flags, and the library
entirely by constructor arguments.

If you want environment-variable configuration for your deployment, put it in
your own wrapper:

```sh
processor --mode=stream --rpc-url="$STELLAR_RPC_URL"
```

## The CLI configuration surface

Seven flags, and nothing else. Full details in the
[CLI Reference](cli-reference.md):

| Flag | Default |
| --- | --- |
| `--mode` | none, required |
| `--rpc-url` | none, required |
| `--network-passphrase` | mainnet passphrase |
| `--start-ledger` | `0` |
| `--end-ledger` | `0` |
| `--soroswap-router` | mainnet router address |
| `--buffer-size` | `0`, meaning the SDK default of 10 |

Everything is validated before the first network call, and a bad combination
exits 2 with one line on stderr.

## The library configuration surface

Configuration is whatever you pass to the constructors:

| What | Where |
| --- | --- |
| RPC endpoint, HTTP client, buffer size | `ledgerbackend.RPCLedgerBackendOptions` |
| Ledger range | `source.NewBackfillSource(ctx, backend, start, end)` |
| Streaming start | `source.NewStreamingSource(ctx, backend, start)` |
| Network passphrase | `ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(passphrase, lcm)` |
| Which venues you extract | which transformers you put in the slice |
| Which AMMs are decoded | `registry.Register(...)` |
| Soroswap router addresses | `soroban.NewSoroswapAdapter(addresses...)` |
| Where events go | your `sink.Sink` |

## Network selection

Two settings must change together when you move between networks, and getting
one right while missing the other is the usual mistake.

**The passphrase** is used to compute transaction hashes. Reading testnet
ledgers with the mainnet passphrase produces wrong `TxHash` values while
everything else looks correct:

| Network | Passphrase |
| --- | --- |
| Mainnet | `Public Global Stellar Network ; September 2015` |
| Testnet | `Test SDF Network ; September 2015` |

**The Soroswap router address** decides which contract's events are decoded.
The default is the mainnet router,
`CAG5LRYQ5JVEUI5TEID72EYOVX44TTUJT5BQR2J6J77FH65PCCFAJDDH`. On testnet, pass
the testnet router, `CCJUD55AG6W5HAI5LRVNKAE5WDP5XGZBUDS5WNTIVDU7O264UZZE7BRD`,
from soroswap/core's `public/testnet.contracts.json`. Verify it before relying
on it, because a stale address yields zero AMM events and no error.

```sh
processor --mode=stream \
  --rpc-url=<testnet-rpc-url> \
  --network-passphrase="Test SDF Network ; September 2015" \
  --soroswap-router=CCJUD55AG6W5HAI5LRVNKAE5WDP5XGZBUDS5WNTIVDU7O264UZZE7BRD
```

## Authenticated RPC endpoints

The CLI takes a URL and nothing else, so an endpoint whose key lives in the URL
path or query string works as-is. An endpoint that requires a header does not:
there is no `--header` flag. Use the library and pass your own `HttpClient`:

```go
backend := ledgerbackend.NewRPCLedgerBackend(ledgerbackend.RPCLedgerBackendOptions{
	RPCServerURL: rpcURL,
	HttpClient:   &http.Client{Transport: authTransport{key: apiKey}},
})
```

A URL containing a key is a secret. It appears in the startup log line the CLI
writes to stderr, so treat those logs accordingly.

## Persistent state

There is none. No checkpoint file, no cursor, no cache, no temporary
directories. The process writes only to the sink you gave it, which for the CLI
means stdout.

Restarting is therefore entirely up to you: pass `--start-ledger` derived from
your own output. See the
[backfill walkthrough](../for-library-consumers/backfill-mode-walkthrough.md)
and [issue #4](https://github.com/fadesany/Stellar-Trade-Processor/issues/4).

## Resource usage

One ledger is held in memory at a time, plus whatever the backend buffers
(`--buffer-size`, default 10). There is no worker pool and no parallelism, so
one process uses roughly one core. Memory scales with ledger size and buffer
size, not with how long the process has been running.

For higher throughput, run several backfill processes over disjoint ledger
ranges rather than looking for a concurrency flag. There isn't one.
