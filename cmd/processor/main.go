// Command processor extracts standardized trade events from Stellar ledgers
// served by a Stellar RPC endpoint and writes them as JSON lines to stdout.
//
// Backfill a bounded range:
//
//	processor --mode=backfill --rpc-url=<url> --start-ledger=<ledger> --end-ledger=<ledger>
//
// Stream live ledgers (from --start-ledger, or the latest ledger when omitted):
//
//	processor --mode=stream --rpc-url=<url>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fadesany/Stellar-Trade-Processor/event"
	"github.com/fadesany/Stellar-Trade-Processor/sink"
	"github.com/fadesany/Stellar-Trade-Processor/source"
	"github.com/fadesany/Stellar-Trade-Processor/transform"
	"github.com/fadesany/Stellar-Trade-Processor/transform/classic"
	"github.com/fadesany/Stellar-Trade-Processor/transform/soroban"
	"github.com/stellar/go-stellar-sdk/clients/rpcclient"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	modeStream   = "stream"
	modeBackfill = "backfill"
)

type config struct {
	mode              string
	rpcURL            string
	networkPassphrase string
	start             uint
	end               uint
	soroswapRouters   string
	bufferSize        uint
}

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "processor: %v\n", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := log.New(os.Stderr, "processor: ", log.LstdFlags)
	if err := run(ctx, cfg, sink.NewJSONLineSink(os.Stdout), logger); err != nil {
		logger.Printf("error: %v", err)
		os.Exit(1)
	}
}

// parseFlags parses and validates command-line flags.
func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("processor", flag.ContinueOnError)
	fs.StringVar(&cfg.mode, "mode", "", "run mode: stream or backfill (required)")
	fs.StringVar(&cfg.rpcURL, "rpc-url", "", "Stellar RPC endpoint URL (required)")
	fs.StringVar(&cfg.networkPassphrase, "network-passphrase", network.PublicNetworkPassphrase, "network passphrase")
	fs.UintVar(&cfg.start, "start-ledger", 0, "first ledger to process (required for backfill; stream defaults to the latest ledger)")
	fs.UintVar(&cfg.end, "end-ledger", 0, "last ledger to process, inclusive (backfill only)")
	fs.StringVar(&cfg.soroswapRouters, "soroswap-router", soroban.SoroswapMainnetRouter, "comma-separated Soroswap router contract addresses")
	fs.UintVar(&cfg.bufferSize, "buffer-size", 0, "RPC ledger backend buffer size (0 uses the SDK default)")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	const maxUint32 = 1<<32 - 1
	switch {
	case cfg.rpcURL == "":
		return config{}, errors.New("--rpc-url is required")
	case cfg.start > maxUint32 || cfg.end > maxUint32 || cfg.bufferSize > maxUint32:
		return config{}, errors.New("--start-ledger, --end-ledger and --buffer-size must fit in uint32")
	}

	switch cfg.mode {
	case modeBackfill:
		if cfg.start == 0 || cfg.end == 0 {
			return config{}, errors.New("backfill mode requires --start-ledger and --end-ledger")
		}
		if cfg.end < cfg.start {
			return config{}, fmt.Errorf("--end-ledger %d is before --start-ledger %d", cfg.end, cfg.start)
		}
	case modeStream:
		if cfg.end != 0 {
			return config{}, errors.New("--end-ledger is not valid in stream mode")
		}
	default:
		return config{}, fmt.Errorf("--mode must be %q or %q, got %q", modeStream, modeBackfill, cfg.mode)
	}
	return cfg, nil
}

// run wires the source, transformers and sink, and processes ledgers until the
// source is exhausted or ctx is cancelled.
func run(ctx context.Context, cfg config, out sink.Sink, logger *log.Logger) error {
	defer func() {
		if err := out.Close(); err != nil {
			logger.Printf("closing sink: %v", err)
		}
	}()

	src, err := openSource(ctx, cfg, logger)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer func() {
		if err := src.Close(); err != nil {
			logger.Printf("closing source: %v", err)
		}
	}()

	registry := soroban.NewRegistry()
	registry.Register(soroban.NewSoroswapAdapter(splitList(cfg.soroswapRouters)...))
	transformers := []transform.Transformer{
		classic.NewTransformer(),
		soroban.NewTransformer(registry),
	}

	var ledgers, trades int
	for {
		lcm, err := src.NextLedger(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return fmt.Errorf("reading next ledger: %w", err)
		}

		events, err := processLedger(cfg.networkPassphrase, lcm, transformers)
		if err != nil {
			return err
		}
		if err := out.Write(events); err != nil {
			return fmt.Errorf("writing events for ledger %d: %w", lcm.LedgerSequence(), err)
		}
		ledgers++
		trades += len(events)
	}

	logger.Printf("processed %d ledgers, %d trade events", ledgers, trades)
	return nil
}

// openSource builds the RPC-backed source for the configured mode.
func openSource(ctx context.Context, cfg config, logger *log.Logger) (source.Source, error) {
	backend := ledgerbackend.NewRPCLedgerBackend(ledgerbackend.RPCLedgerBackendOptions{
		RPCServerURL: cfg.rpcURL,
		BufferSize:   uint32(cfg.bufferSize),
	})

	var (
		src source.Source
		err error
	)
	switch cfg.mode {
	case modeBackfill:
		logger.Printf("backfilling ledgers %d-%d from %s", cfg.start, cfg.end, cfg.rpcURL)
		src, err = source.NewBackfillSource(ctx, backend, uint32(cfg.start), uint32(cfg.end))
	default: // modeStream
		start := uint32(cfg.start)
		if start == 0 {
			// The RPC backend cannot report its latest ledger before a range is
			// prepared, so ask the RPC server directly.
			latest, lerr := rpcclient.NewClient(cfg.rpcURL, nil).GetLatestLedger(ctx)
			if lerr != nil {
				_ = backend.Close()
				return nil, fmt.Errorf("getting latest ledger from %s: %w", cfg.rpcURL, lerr)
			}
			start = latest.Sequence
		}
		logger.Printf("streaming ledgers from %d via %s", start, cfg.rpcURL)
		src, err = source.NewStreamingSource(ctx, backend, start)
	}
	if err != nil {
		_ = backend.Close()
		return nil, err
	}
	return src, nil
}

// processLedger runs every transformer over each transaction in lcm and
// returns the merged events in transaction order.
func processLedger(passphrase string, lcm xdr.LedgerCloseMeta, transformers []transform.Transformer) ([]event.TradeEvent, error) {
	seq := lcm.LedgerSequence()
	closedAt := lcm.ClosedAt()

	reader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(passphrase, lcm)
	if err != nil {
		return nil, fmt.Errorf("creating transaction reader for ledger %d: %w", seq, err)
	}
	defer reader.Close()

	var events []event.TradeEvent
	for {
		tx, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading transaction in ledger %d: %w", seq, err)
		}
		for _, t := range transformers {
			txEvents, err := t.Transform(tx, seq, closedAt)
			if err != nil {
				return nil, fmt.Errorf("ledger %d: %w", seq, err)
			}
			events = append(events, txEvents...)
		}
	}
	return events, nil
}

// splitList splits a comma-separated flag value, dropping empty entries.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
