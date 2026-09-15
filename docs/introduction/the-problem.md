# The Problem

Stellar has a standard shape for token transfer data. The Token Transfer
Processor takes payments, path payment transfers, mints, burns, clawbacks and
the Soroban token equivalents, and gives every consumer the same event type. If
you index transfers, you do not have to know how each operation encodes its
result.

Nothing does that for trades.

A trade can reach the ledger in several different shapes. A `ManageSellOffer`
result carries a list of claimed offer atoms, and each atom is a fill against
one counterparty. A path payment carries the same kind of list, except the
atoms are hops in a route, and the trader is on one side of every hop. A claim
atom can be an order book claim, with a seller account, or a liquidity pool
claim, with no account at all. Soroban swaps are not operation results at all:
they are contract events, and every AMM defines its own event schema, its own
field names, and its own way of expressing a multi-hop route.

So every indexer, wallet, and analytics tool that wants trade data writes this
parsing again. The same claim atom walk, the same decision about which side is
base and which is counter, the same handling of the liquidity pool case, the
same per-protocol contract event decoding. It is duplicated work, it is easy to
get subtly wrong, and the errors are quiet: a reversed base and counter still
produces a plausible looking row.

There is also a correctness trap that shows up in practice. The generated XDR
accessors in the Go SDK dereference union arms directly, so a malformed result
body crashes the process rather than returning an error. A parser written
quickly against well-formed mainnet data will panic the first time it meets
something unexpected. This package checks those arms explicitly and returns
errors instead, and the tests exercise that path.

## This design was proposed by a maintainer, not invented here

In [stellar · Discussion #1716, "Unified Golang SDK for Stellar"](https://github.com/orgs/stellar/discussions/1716),
opened by `mollykarcher` on 1 April 2025, maintainer `sreuland` raised exactly
this pattern for future processors:

> define generic interfaces for processor and event in the proposed Golang SDK
> to promote consistent approach for consuming network data with stream
> oriented, event driven processor implementations which follow typical
> roles(source, sink, transformer)

That is the structure this repository implements, applied to trade data:
`Source` for ledger input, `Transformer` for deriving events, `Sink` for
output, and a single standardized event type across every venue.
