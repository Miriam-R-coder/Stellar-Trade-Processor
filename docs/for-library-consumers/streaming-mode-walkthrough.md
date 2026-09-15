# Streaming Mode Walkthrough

Streaming follows the network: the processor waits for each new ledger to
close, extracts trades from it, writes them, and moves on. It runs until you
stop it or something fails.

## Running it

```sh
processor --mode=stream --rpc-url=https://mainnet.sorobanrpc.com
```

With no `--start-ledger`, the CLI asks the RPC server for its latest ledger and
starts there. To start from a specific ledger instead:

```sh
processor --mode=stream \
  --rpc-url=https://mainnet.sorobanrpc.com \
  --start-ledger=64446884
```

`--end-ledger` is rejected in stream mode.

## What the output looks like

Logs go to stderr, one line at startup:

```
processor: 2026/09/16 00:03:14 streaming ledgers from 64446884 via https://mainnet.sorobanrpc.com
```

Trades go to stdout, one JSON object per line, so you can pipe stdout without
log lines mixing in:

```json
{"Venue":"path_payment","Protocol":"","BaseAsset":{"Type":"native","Code":"","Issuer":"","ContractAddress":""},"CounterAsset":{"Type":"credit_alphanum12","Code":"BITCOIN","Issuer":"GDRRAQK2L22YKWPQMNU7QGNQ4KL3SM6ML3WWQH62UQSZ4W7NOX3JKUMI","ContractAddress":""},"BaseAmount":"15.0000000","CounterAmount":"0.1250000","Price":"0.0083333","Account":"GB74WSCTZCC6EQD5X263ZXDANEOZRFBBBZOPIKQCFZ3HKQ4LQEXN7FNB","CounterAccount":"GDRWZVWNWEMQNWDBOS3TC4D6MPYVUIG2FSOHOGRZTB6AVVQZSYFRWITT","LedgerSequence":64446884,"TxHash":"84179ee4019a0d39c6116e2c91b34373b6a336fadd272fd8936fa8c2750e29b4","OpIndex":0,"ClosedAt":"2026-09-15T23:03:10Z"}
```

A 45-second run against mainnet starting at ledger 64446884 produced 570
events: 12 classic DEX fills, 143 path payment fills against order book offers,
and 415 path payment fills against classic liquidity pools. Mainnet closes a
ledger roughly every 5 seconds, so expect output in bursts of one ledger at a
time rather than a steady trickle.

## Stopping it

Ctrl+C (or `SIGTERM`) cancels the context. The current ledger finishes, the
loop exits, the sink is flushed and closed, and a summary line is written:

```
processor: 2026/09/16 00:04:02 processed 9 ledgers, 570 trade events
```

Exit status is 0. Events for a ledger are written before the next ledger is
fetched, so a clean stop never leaves a half-written ledger.

## Waiting versus failing

These two are different, and worth separating.

**Waiting for the next ledger is normal.** When the requested ledger has not
closed yet, the RPC backend polls every 2 seconds until it appears. This is not
an error and produces no log output.

**A failed RPC call ends the run.** There is **no automatic reconnect and no
retry loop** in this package. If the RPC server returns an error, times out, or
the connection drops, the error propagates up, the CLI logs it and exits 1:

```
processor: 2026/09/16 00:12:44 error: reading next ledger: getting ledger 64446902: ...
```

Restart with `--start-ledger` set to the last ledger you saw, plus one. The
processor keeps no cursor and no checkpoint file, so that number has to come
from your own output or your own bookkeeping.

If you need resilience today, supervise the process. A systemd unit with
`Restart=on-failure`, or a container restart policy, plus a wrapper that reads
the last `LedgerSequence` you stored and passes it as `--start-ledger`, is
enough. Automatic reconnect is not implemented.

## Using it as a library

Streaming through the library is the same loop as backfill, with a different
source:

```go
src, err := source.NewStreamingSource(ctx, backend, startLedger)
```

`NextLedger` blocks until the ledger is available and never returns `io.EOF`,
so the loop ends only on error or context cancellation. Pass a context you can
cancel, and handle cancellation explicitly:

```go
lcm, err := src.NextLedger(ctx)
if err != nil {
	if ctx.Err() != nil {
		return nil // shutting down, not a failure
	}
	return fmt.Errorf("reading next ledger: %w", err)
}
```

One detail specific to the RPC backend: it refuses to report its latest ledger
before a range is prepared, so `NewStreamingSource` with `start` of 0 does not
work with it. Resolve the starting ledger first, which is what the CLI does:

```go
latest, err := rpcclient.NewClient(rpcURL, nil).GetLatestLedger(ctx)
if err != nil {
	return fmt.Errorf("getting latest ledger from %s: %w", rpcURL, err)
}
src, err := source.NewStreamingSource(ctx, backend, latest.Sequence)
```

## Current limits

- No automatic reconnect or retry after an RPC failure.
- No checkpoint: restarting means telling it where to start.
- One transformer error ends the run. There is no flag to skip a bad ledger and continue.
- Ledgers are processed one at a time, in order. There is no parallelism.
