# What is Stellar Trade Processor

Stellar Trade Processor is a Go library that reads Stellar ledger data and
produces standardized trade events from it. It handles three kinds of trade:
classic DEX offer fills (from `ManageSellOffer`, `ManageBuyOffer` and
`CreatePassiveSellOffer`), path payments (`PathPaymentStrictSend` and
`PathPaymentStrictReceive`, one event per hop), and Soroban AMM swaps, which in
v1 means Soroswap. Whatever the venue, you get the same struct back:
`event.TradeEvent`, with the assets, the amounts, the price, the accounts
involved, and where in the chain it happened. A small CLI ships with it for
people who want JSON lines rather than a Go dependency.

It is a sibling to Stellar's official
[Token Transfer Processor](https://github.com/stellar/go-stellar-sdk/tree/main/processors/token_transfer)
(TTP), which does the equivalent job for token transfers: TTP turns payments,
mints, burns and clawbacks into one standardized transfer event, and this
package does the same for trades. The conventions are deliberately the same as
the `go-stellar-sdk` ingest SDK. Ledgers arrive from a
`ledgerbackend.LedgerBackend`, transactions are read with
`ingest.LedgerTransactionReader`, and processing a ledger means walking its
transactions and handing each one to a transformer.

The whole package is a library plus a thin CLI. There is no database, no
server, no hosted API, and no UI. You decide where the events go by
implementing one interface with two methods.
