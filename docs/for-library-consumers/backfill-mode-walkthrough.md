# Backfill Mode Walkthrough

Backfill processes a fixed, inclusive ledger range and exits. Use it to load
history, to re-extract a window after a bug fix, or to test changes against
known ledgers.

## Running it

```sh
processor --mode=backfill \
  --rpc-url=https://mainnet.sorobanrpc.com \
  --start-ledger=64446600 --end-ledger=64446880 > trades.jsonl
```

Both `--start-ledger` and `--end-ledger` are required, and `--end-ledger` must
not be lower than `--start-ledger`. The range includes both endpoints.

## What a real run looks like

Logs on stderr:

```
processor: 2026/09/16 00:04:57 backfilling ledgers 64446600-64446880 from https://mainnet.sorobanrpc.com
processor: 2026/09/16 00:07:03 processed 281 ledgers, 21101 trade events
```

That run took **2 minutes 6 seconds for 281 ledgers**, about 2.2 ledgers per
second, or roughly 167 events per second, against a public RPC endpoint from a
consumer connection. It produced 21,101 events:

| Venue | Protocol | Events |
| --- | --- | --- |
| `path_payment` | `liquidity_pool` | 14,405 |
| `path_payment` | (order book) | 6,291 |
| `classic_dex` | (order book) | 399 |
| `soroban_amm` | `soroswap` | 6 |

Two things to take from those numbers. Classic liquidity pool fills dominate
mainnet path payment traffic. And Soroswap swaps are rare enough that a short
range may contain none at all, so a window of a few hundred ledgers is not
evidence that AMM extraction is broken.

Throughput is dominated by fetching ledgers, not by transforming them. A local
captive core backend or a closer RPC endpoint changes the number far more than
anything in this package does.

## If it is interrupted

**Backfill is not resumable.** There is no checkpoint file and no cursor. If
you Ctrl+C at ledger 64446750 of a range ending at 64446880, the run stops
cleanly, flushes what it has, and prints its summary, but nothing records where
it stopped.

Restarting the same command starts again at `--start-ledger`, re-fetching and
re-emitting every ledger you already processed. You have two practical options:

- Read the highest `LedgerSequence` out of your own output and restart with `--start-ledger` set to that plus one.
- Split a large backfill into chunks of a few thousand ledgers, run them in sequence, and treat each completed chunk as your checkpoint.

Checkpointing is tracked in
[issue #4](https://github.com/fadesany/Stellar-Trade-Processor/issues/4).

Because output is JSON lines, deduplication after an overlap is
straightforward: a trade is identified by its transaction hash, operation
index, and position within that operation.

## Retention limits

An RPC server only keeps recent ledgers. Public endpoints commonly retain
around a week; archive endpoints keep more. Request a ledger outside the
retention window and the backend returns an error, the run stops, and the log
names the ledger it could not fetch. For deep history you need an archive RPC
provider or a captive core backend reading from history archives.

Check what a server has before starting a long backfill:

```sh
curl -s -X POST https://mainnet.sorobanrpc.com \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"getLatestLedger"}'
```

## Using it as a library

```go
backend := ledgerbackend.NewRPCLedgerBackend(ledgerbackend.RPCLedgerBackendOptions{
	RPCServerURL: rpcURL,
})
src, err := source.NewBackfillSource(ctx, backend, 64446600, 64446880)
if err != nil {
	_ = backend.Close()
	return fmt.Errorf("opening source: %w", err)
}
defer src.Close()

for {
	lcm, err := src.NextLedger(ctx)
	if errors.Is(err, io.EOF) {
		break // range complete
	}
	if err != nil {
		return fmt.Errorf("reading ledger: %w", err)
	}
	// read transactions, transform, write
}
```

The only difference from streaming is the `io.EOF` check: backfill ends, and
streaming does not.

## Error behaviour

Any transformer error stops the run and is reported with the ledger that caused
it:

```
processor: 2026/09/16 00:05:31 error: ledger 64446712: transaction 01c9b5... op 0: soroswap adapter for contract CAG5LRYQ...: event 0: decoding soroswap swap event: missing field to
```

This is deliberate: a decoding bug that silently skips trades is worse than a
stopped backfill. There is no flag to continue past a bad ledger. If you hit
one, that error message is a complete bug report for
[a new issue](https://github.com/fadesany/Stellar-Trade-Processor/issues/new).

## Buffer size

`--buffer-size` sets how many ledgers the RPC backend keeps buffered ahead.
Zero, the default, uses the SDK's own default of 10. Raising it can help on a
high-latency link at the cost of memory; it does not change results.
