# Getting Started

## Install

Go 1.25 or newer, matching the `go` directive in `go.mod`, which follows the
minimum required by `github.com/stellar/go-stellar-sdk`.

```sh
go get github.com/fadesany/Stellar-Trade-Processor
```

You also need a Stellar RPC endpoint. The examples below use
`https://mainnet.sorobanrpc.com`, listed on Stellar's
[RPC providers page](https://developers.stellar.org/docs/data/apis/rpc/providers).
Any endpoint works, including your own.

## A complete program

This is `examples/basic/main.go` from the repository, unabridged. It backfills a
ledger range and prints one JSON line per trade.

```go
// Command basic shows how to wire the Stellar Trade Processor as a library:
// a backfill source over a Stellar RPC endpoint, the classic and Soroban
// transformers (with the Soroswap adapter registered), and the JSON line sink.
//
//	go run ./examples/basic --rpc-url=<url> --start-ledger=<ledger> --end-ledger=<ledger>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/fadesany/Stellar-Trade-Processor/sink"
	"github.com/fadesany/Stellar-Trade-Processor/source"
	"github.com/fadesany/Stellar-Trade-Processor/transform"
	"github.com/fadesany/Stellar-Trade-Processor/transform/classic"
	"github.com/fadesany/Stellar-Trade-Processor/transform/soroban"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/network"
)

func main() {
	rpcURL := flag.String("rpc-url", "", "Stellar RPC endpoint URL (required)")
	start := flag.Uint("start-ledger", 0, "first ledger (required)")
	end := flag.Uint("end-ledger", 0, "last ledger, inclusive (required)")
	flag.Parse()
	if *rpcURL == "" || *start == 0 || *end < *start {
		fmt.Fprintln(os.Stderr, "usage: basic --rpc-url=<url> --start-ledger=<ledger> --end-ledger=<ledger>")
		os.Exit(2)
	}

	if err := run(context.Background(), *rpcURL, uint32(*start), uint32(*end)); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, rpcURL string, start, end uint32) error {
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

	for {
		lcm, err := src.NextLedger(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading ledger: %w", err)
		}

		reader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(network.PublicNetworkPassphrase, lcm)
		if err != nil {
			return fmt.Errorf("reading ledger %d: %w", lcm.LedgerSequence(), err)
		}
		for {
			tx, err := reader.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				reader.Close()
				return fmt.Errorf("reading transaction: %w", err)
			}
			for _, t := range transformers {
				events, err := t.Transform(tx, lcm.LedgerSequence(), lcm.ClosedAt())
				if err != nil {
					reader.Close()
					return fmt.Errorf("transforming transaction: %w", err)
				}
				if err := out.Write(events); err != nil {
					reader.Close()
					return fmt.Errorf("writing events: %w", err)
				}
			}
		}
		reader.Close()
	}
}
```

Run it:

```sh
go run ./examples/basic \
  --rpc-url=https://mainnet.sorobanrpc.com \
  --start-ledger=64446600 --end-ledger=64446700
```

## The four moving parts

**The backend** comes from the ingest SDK. `NewRPCLedgerBackend` talks to a
Stellar RPC server. Anything implementing `ledgerbackend.LedgerBackend` works,
including captive core if you run one.

**The source** owns the backend. `NewBackfillSource` prepares a bounded range
and returns `io.EOF` when it is done. Swap in `source.NewStreamingSource(ctx,
backend, start)` to follow the network instead; the loop above does not change,
because streaming simply never returns `io.EOF`.

**The transformers** are called once per transaction each. Wire only the ones
you need: dropping `soroban.NewTransformer(registry)` from the slice skips
contract event decoding entirely, and dropping `classic.NewTransformer()` gives
you AMM swaps only.

**The sink** is where your own code usually goes. `JSONLineSink` is the only
implementation shipped. See the
[Go SDK Reference](../developer-guide/go-sdk-reference.md) for a custom `Sink`.

## Passphrases matter

`ingest.NewLedgerTransactionReaderFromLedgerCloseMeta` takes the network
passphrase and uses it to compute transaction hashes. Pass
`network.TestNetworkPassphrase` when reading testnet ledgers, or every `TxHash`
in your output will be wrong while everything else looks fine.

For testnet you also need the right Soroswap router address, since
`NewSoroswapAdapter()` with no arguments matches the mainnet router only:

```go
// Testnet router, from soroswap/core public/testnet.contracts.json
registry.Register(soroban.NewSoroswapAdapter("CCJUD55AG6W5HAI5LRVNKAE5WDP5XGZBUDS5WNTIVDU7O264UZZE7BRD"))
```

Deployments get replaced. Check the address against soroswap/core's published
contract list before relying on it: a router address that no longer matches
produces no AMM events and no error.

## What you get back

Each `Transform` call returns a slice that is usually empty. Most transactions
are not trades. A transaction that crosses several offers returns several
events, and the slice is returned in ledger order, so writing them straight
through preserves ordering.
