// Command basic shows how to wire the Stellar Trade Processor as a library:
// a backfill source over a Stellar RPC endpoint, the classic and Soroban
// transformers (with the Soroswap adapter registered), and the JSON line sink.
//
//	go run ./examples/basic --rpc-url=<url> --start=<ledger> --end=<ledger>
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
	start := flag.Uint("start", 0, "first ledger (required)")
	end := flag.Uint("end", 0, "last ledger, inclusive (required)")
	flag.Parse()
	if *rpcURL == "" || *start == 0 || *end < *start {
		fmt.Fprintln(os.Stderr, "usage: basic --rpc-url=<url> --start=<ledger> --end=<ledger>")
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
