# How to Contribute

Contributions are welcome. This page expands on the Contributing section of the
README with more detail on picking work that fits what you want to do.

## Finding something to work on

Start at the
[open issues](https://github.com/fadesany/Stellar-Trade-Processor/issues).
Every issue has a Summary, Acceptance Criteria as checkboxes, and the Tech
Stack involved, so you can judge scope before committing your time.

Comment on an issue before you start, so two people do not write the same
adapter in parallel.

### Labels

| Label | Meaning |
| --- | --- |
| [`good first issue`](https://github.com/fadesany/Stellar-Trade-Processor/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22) | Self-contained, no deep Stellar knowledge needed |
| `complexity:low` | Small and well scoped; touches one package |
| `complexity:medium` | Some design work; may add a flag or a new file |
| `complexity:high` | Significant design work; protocol research or cross-cutting changes |
| `enhancement` | New capability |
| `documentation` | Docs only |

### Picking by what you want to do

**New to Go, or new to this codebase.** Take a `good first issue`. Both current
ones are table-driven test work, which is the fastest way to learn how the
package handles XDR:
[#6, unit tests for the source and sink packages](https://github.com/fadesany/Stellar-Trade-Processor/issues/6)
needs no Stellar knowledge at all, and
[#7, classic transformer edge cases](https://github.com/fadesany/Stellar-Trade-Processor/issues/7)
teaches you claim atoms by testing them.

**Comfortable in Go, want a feature.** The `complexity:medium` issues are
self-contained features:
[#4, backfill checkpointing](https://github.com/fadesany/Stellar-Trade-Processor/issues/4)
is pure standard library, and
[#5, a Postgres sink example](https://github.com/fadesany/Stellar-Trade-Processor/issues/5)
is a worked example of implementing the `Sink` interface.

**You want to write a protocol adapter.** That is
[#1, a second AMM adapter](https://github.com/fadesany/Stellar-Trade-Processor/issues/1).
Read
[Adding a New AMM Protocol Adapter](../for-contributors/adding-a-new-amm-protocol-adapter.md)
first. The hard part is not Go, it is verifying the protocol's event schema
against its actual contract source. Budget most of your time for that.

**You like decoding and edge cases.**
[#2, Soroswap swaps that bypass the router](https://github.com/fadesany/Stellar-Trade-Processor/issues/2)
and
[#3, token decimals and SAC mapping](https://github.com/fadesany/Stellar-Trade-Processor/issues/3)
both need ledger state lookups on top of event decoding.

**You like performance work.**
[#8, benchmarking streaming against backfill](https://github.com/fadesany/Stellar-Trade-Processor/issues/8)
establishes a baseline before anyone optimises anything.

Something not on the list is fine too. Open an issue describing it before
writing much code, especially for anything that changes the `TradeEvent`
schema, since that affects every consumer.

## Branches

Name the branch for what it does:

```
feat/aqua-adapter
fix/path-payment-nil-arm
docs/streaming-walkthrough
test/source-unit-tests
refactor/claim-atom-mapping
```

## Commits

[Conventional commits](https://www.conventionalcommits.org/), `type(scope): description`:

```
feat(transform/soroban): add Aqua adapter
fix(transform/classic): guard nil result union arms instead of panicking
test(source,sink): cover BackfillSource and JSONLineSink
docs(package-reference): TradeEvent schema
```

Types in use: `feat`, `fix`, `test`, `docs`, `refactor`, `ci`, `chore`. The
scope is the package path. Keep one logical change per commit, so a commit that
adds an adapter and reformats an unrelated file should be two commits.

## Before opening a PR

Run what CI runs:

```sh
go build ./...
go vet ./...
go test ./... -race -cover
gofmt -l .        # must print nothing
```

### Checklist

- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` is clean
- [ ] `go test ./... -race` passes
- [ ] New behaviour has table-driven tests
- [ ] New transformer or adapter code has a malformed-input test proving it returns an error rather than panicking
- [ ] `gofmt` applied, exported identifiers have doc comments
- [ ] Errors wrapped with `%w` and stating what was attempted
- [ ] No floating point in any amount or price path
- [ ] No claim of AMM support beyond what is actually registered
- [ ] Docs updated if behaviour or flags changed, including these pages

CI runs the same commands on every push and pull request to `main`, in a job
named `test`.

## Rules that are not negotiable

These come from the problems this package exists to avoid:

1. **No floats for money.** Amounts and prices use string-encoded decimals, `big.Int`, or `big.Rat`. A `float64` in an amount path will be sent back.
2. **Transformers never panic.** Check union arms for nil before access; the SDK's generated accessors do not. Return an error instead.
3. **Do not guess a contract's event schema.** Verify against source or official docs and cite it in the PR.
4. **Do not overstate support.** One adapter is registered. The registry is an extension point, and the docs say so; keep it that way.

## Review

Expect review comments on error messages and test coverage of malformed input.
Those are the two places where this code has already been wrong once: the
path payment tests caught a real crash, where the SDK accessors dereferenced a
nil result body instead of reporting a missing one. That fix is
`fix(transform/classic): guard nil result union arms instead of panicking`.

## Reporting bugs and vulnerabilities

Ordinary bugs go in the
[issue tracker](https://github.com/fadesany/Stellar-Trade-Processor/issues/new).
Useful details: the version or commit, the ledger sequence and network, and the
full error message, which already names the transaction and operation.

Security issues do not go in a public issue. Use the repository's Security tab
and **Report a vulnerability**. See
[SECURITY.md](https://github.com/fadesany/Stellar-Trade-Processor/blob/main/SECURITY.md)
for what is in scope, and note that the project is unaudited and
solo-maintained, with best-effort response times and no SLA.
