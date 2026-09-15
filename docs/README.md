# Stellar Trade Processor

A Go library and CLI that turns Stellar ledger data into standardized trade
events: classic DEX offer fills, path payment hops, and Soroban AMM swaps, all
emitted as one `event.TradeEvent` type.

Current release: **v0.1.0**. The code is unaudited and pre-1.0, so exported
APIs and event semantics can change without a deprecation period.

## Where to start

| If you want to | Read |
| --- | --- |
| Understand what this does and why it exists | [What is Stellar Trade Processor](introduction/what-is-stellar-trade-processor.md), [The Problem](introduction/the-problem.md) |
| See the data flow end to end | [How It Works](introduction/how-it-works.md) |
| Know every field you will receive | [TradeEvent Schema](package-reference/tradeevent-schema.md) |
| Wire the library into your own program | [Getting Started](for-library-consumers/getting-started.md) |
| Run the CLI against mainnet | [Streaming Mode](for-library-consumers/streaming-mode-walkthrough.md), [Backfill Mode](for-library-consumers/backfill-mode-walkthrough.md) |
| Add support for another AMM | [Adding a New AMM Protocol Adapter](for-contributors/adding-a-new-amm-protocol-adapter.md) |

## What v1 covers

Classic DEX extraction is complete: offer fills from `ManageSellOffer`,
`ManageBuyOffer` and `CreatePassiveSellOffer`, plus both path payment
operations, including fills against classic liquidity pools.

Soroban AMM extraction covers **Soroswap only**. One adapter is registered.
The protocol registry is an extension point for adding more, not a claim that
more already work.

## Known limits

- Swaps that call a Soroswap pair contract directly, without the router, are not captured.
- Soroswap amounts are raw token units; they are not scaled by token decimals.
- An interrupted backfill restarts from the beginning; there is no checkpoint.
- Streaming has no automatic reconnect. A failed RPC call ends the run.

Source: [github.com/fadesany/Stellar-Trade-Processor](https://github.com/fadesany/Stellar-Trade-Processor)
