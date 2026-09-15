# Local Setup

## Requirements

- **Go 1.25.0 or newer.** That is the `go` directive in `go.mod`, which follows the minimum required by `github.com/stellar/go-stellar-sdk`.
- **Git.**
- **A Stellar RPC endpoint**, only for running against live data. Building and testing need no network beyond the initial module download, because every test builds its XDR fixtures in memory.

Check your Go version:

```sh
go version
```

## Clone and build

```sh
git clone https://github.com/fadesany/Stellar-Trade-Processor.git
cd Stellar-Trade-Processor
go mod download
go build ./...
```

`go mod download` pulls the ingest SDK and its dependencies. It is a large
tree, so the first run takes a while; later builds are cached.

## Test

```sh
go test ./...
```

Expect `transform/classic` and `transform/soroban` to report `ok`, and the
other packages to report `[no test files]`. Adding tests for `source` and
`sink` is [issue #6](https://github.com/fadesany/Stellar-Trade-Processor/issues/6),
a good first contribution.

Run what CI runs before opening a PR:

```sh
go build ./...
go vet ./...
go test ./... -race -cover
```

The race detector needs a C toolchain. If `-race` fails to link on your
machine, run the plain `go test ./...` locally and let CI cover the race build.

Useful variations while working:

```sh
go test ./transform/soroban/ -run TestTransformSoroswap -v   # one test, verbose
go test ./... -count=1                                       # bypass the test cache
gofmt -l .                                                   # list unformatted files; should print nothing
```

## Run the CLI from source

```sh
go run ./cmd/processor --mode=backfill \
  --rpc-url=https://mainnet.sorobanrpc.com \
  --start-ledger=64446600 --end-ledger=64446610
```

A ten-ledger range is enough to confirm everything works end to end. Or build a
binary:

```sh
go build -o processor ./cmd/processor
./processor --mode=stream --rpc-url=https://mainnet.sorobanrpc.com
```

`.gitignore` already covers `/processor` and `*.exe`.

## Repository layout

```
cmd/processor/        CLI entrypoint
event/                TradeEvent and Asset types
source/               Source interface, BackfillSource, StreamingSource
sink/                 Sink interface, JSONLineSink
transform/            Transformer interface
transform/classic/    Offer fill and path payment extraction
transform/soroban/    Soroban transformer, Registry, SoroswapAdapter
examples/basic/       Runnable library wiring example
docs/                 This documentation site
.github/workflows/    CI
```

## CI

`.github/workflows/ci.yml` runs on every push and pull request to `main`. The
job is named `test`, checks out, sets up Go from `go-version-file: go.mod`, then
runs build, vet, and `go test ./... -race -cover`. A vet warning or a failing
test fails the job.

## Coding conventions

- Every exported identifier has a doc comment.
- Errors are wrapped with `%w` and say what was being attempted: `fmt.Errorf("parsing offer result: %w", err)`.
- No `panic()` outside `main` startup failures. Transformers return errors.
- No floats in any amount, price, or ratio. Use string-encoded decimals, `big.Int`, or `big.Rat`.
- Tests are table-driven and include at least one malformed-input case per transformer path.
- No global mutable state apart from `Registry`, which is a container built once at startup.
