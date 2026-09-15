# CLI Reference

The CLI lives at `cmd/processor`. It reads ledgers from a Stellar RPC endpoint,
extracts trades, and writes them as JSON lines to stdout. Logs go to stderr.

```sh
go install github.com/fadesany/Stellar-Trade-Processor/cmd/processor@latest
```

## Synopsis

```sh
processor --mode=backfill --rpc-url=<url> --start-ledger=<ledger> --end-ledger=<ledger>
processor --mode=stream   --rpc-url=<url> [--start-ledger=<ledger>]
```

## Flags

Every flag accepted by `cmd/processor`, with its default:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--mode` | string | none, required | `stream` or `backfill`. |
| `--rpc-url` | string | none, required | Stellar RPC endpoint URL. |
| `--network-passphrase` | string | `Public Global Stellar Network ; September 2015` | Network passphrase; used to compute transaction hashes. Testnet is `Test SDF Network ; September 2015`. |
| `--start-ledger` | uint | `0` | First ledger to process. Required for backfill. In stream mode, `0` means start at the RPC server's latest ledger. |
| `--end-ledger` | uint | `0` | Last ledger, inclusive. Backfill only; rejected in stream mode. |
| `--soroswap-router` | string | `CAG5LRYQ5JVEUI5TEID72EYOVX44TTUJT5BQR2J6J77FH65PCCFAJDDH` | Comma-separated Soroswap router contract addresses to match. |
| `--buffer-size` | uint | `0` | RPC ledger backend buffer size. `0` uses the SDK default of 10. |

Go's flag package accepts `--flag=value` and `--flag value`, and a single dash
works too (`-mode=stream`). `--help` prints the same list.

## Validation

Checked before any network call:

- `--rpc-url` must be set.
- `--mode` must be exactly `stream` or `backfill`.
- `--start-ledger`, `--end-ledger` and `--buffer-size` must fit in a `uint32`.
- Backfill requires both `--start-ledger` and `--end-ledger`, and `--end-ledger` must not be below `--start-ledger`.
- Stream mode rejects `--end-ledger`.

A validation failure prints one line to stderr and exits 2:

```
processor: backfill mode requires --start-ledger and --end-ledger
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Range completed (backfill), or the process was interrupted cleanly (stream). |
| `1` | A runtime error: RPC failure, a transformer error, or a sink write failure. The error is logged first. |
| `2` | Invalid flags, or `--help`. |

## Output

stdout carries one JSON object per trade, flushed after each ledger. stderr
carries a startup line and a summary line:

```
processor: 2026/09/16 00:04:57 backfilling ledgers 64446600-64446880 from https://mainnet.sorobanrpc.com
processor: 2026/09/16 00:07:03 processed 281 ledgers, 21101 trade events
```

Because the two streams are separate, `> trades.jsonl` captures only events.

## Examples

Backfill a range to a file:

```sh
processor --mode=backfill \
  --rpc-url=https://mainnet.sorobanrpc.com \
  --start-ledger=64446600 --end-ledger=64446880 > trades.jsonl
```

Stream from the latest ledger, filtering to Soroswap swaps with `jq`:

```sh
processor --mode=stream --rpc-url=https://mainnet.sorobanrpc.com \
  | jq -c 'select(.Protocol == "soroswap")'
```

Count events by venue after a backfill:

```sh
jq -r '.Venue' trades.jsonl | sort | uniq -c
```

Run against testnet, with both the passphrase and the router address changed:

```sh
processor --mode=stream \
  --rpc-url=<testnet-rpc-url> \
  --network-passphrase="Test SDF Network ; September 2015" \
  --soroswap-router=<testnet-router-address>
```

Look the testnet router address up in soroswap/core's published contract list
rather than assuming it; the default value is the mainnet router, and a wrong
address silently yields no AMM events.

## Signals

`SIGINT` (Ctrl+C) and `SIGTERM` cancel the run. The ledger in flight finishes,
the sink is flushed and closed, the summary line is written, and the exit code
is 0.

## Behaviour worth knowing before you script it

- **No reconnect.** An RPC error ends the run with exit 1. Supervise the process if you need it to come back.
- **No checkpoint.** Nothing records progress. Restarting means passing `--start-ledger` yourself, derived from your own output.
- **Fail fast.** One transformer error stops everything; there is no flag to skip a bad ledger.
- **One ledger at a time.** No parallelism, and output stays in ledger order.
