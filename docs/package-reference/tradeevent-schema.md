# TradeEvent Schema

Every trade, from every venue, comes back as one `event.TradeEvent`. This page
matches `event/trade_event.go` as it stands in the repository.

## The struct

```go
// TradeEvent is a single standardized trade, expressed from the perspective
// of Account. Amounts and price are string-encoded fixed-point decimals; no
// floating point values are ever used.
type TradeEvent struct {
	Venue Venue
	// Protocol is empty for orderbook fills, "liquidity_pool" for classic
	// liquidity pool fills, and the AMM protocol name (e.g. "soroswap") for
	// soroban_amm trades.
	Protocol string
	// BaseAsset is the asset Account gave up.
	BaseAsset Asset
	// CounterAsset is the asset Account received.
	CounterAsset Asset
	// BaseAmount is the amount of BaseAsset Account gave up.
	BaseAmount string
	// CounterAmount is the amount of CounterAsset Account received.
	CounterAmount string
	// Price is CounterAmount / BaseAmount as a decimal string.
	Price string
	// Account is the account whose trade this is.
	Account string
	// CounterAccount is the other party, empty when not applicable (e.g. an
	// AMM or liquidity pool).
	CounterAccount string
	LedgerSequence uint32
	TxHash         string
	OpIndex        int
	ClosedAt       time.Time
}
```

## Fields

| Field | Go type | Meaning |
| --- | --- | --- |
| `Venue` | `event.Venue` | Where the trade executed: `classic_dex`, `path_payment`, or `soroban_amm`. |
| `Protocol` | `string` | Empty for order book fills. `liquidity_pool` for classic liquidity pool fills. The AMM name for Soroban trades, which in v1 is only `soroswap`. |
| `BaseAsset` | `event.Asset` | The asset `Account` gave up. |
| `CounterAsset` | `event.Asset` | The asset `Account` received. |
| `BaseAmount` | `string` | Amount of `BaseAsset` given up, as a decimal string. Classic trades use 7 decimal places. Soroswap amounts are raw integers in the token's smallest unit. |
| `CounterAmount` | `string` | Amount of `CounterAsset` received, same encoding as `BaseAmount`. |
| `Price` | `string` | `CounterAmount / BaseAmount`, computed with `big.Rat` and rendered to 7 decimal places. |
| `Account` | `string` | Whose trade this is. The operation source account (or transaction source) for classic trades; the swap's `to` address for Soroswap. |
| `CounterAccount` | `string` | The other party's account. The offer owner for order book fills. Empty for liquidity pool fills and AMM swaps. |
| `LedgerSequence` | `uint32` | Sequence of the ledger the trade closed in. |
| `TxHash` | `string` | Hex-encoded transaction hash. |
| `OpIndex` | `int` | Zero-based index of the operation within the transaction. |
| `ClosedAt` | `time.Time` | Ledger close time, in UTC. |

`TxHash` and `OpIndex` do not uniquely identify an event. One operation that
crosses four offers produces four events with identical `TxHash` and
`OpIndex`. If you need a primary key, add the event's ordinal within the
operation, or use the whole tuple of assets and amounts.

## Asset

```go
// Asset describes one side of a trade.
type Asset struct {
	Type            string
	Code            string
	Issuer          string
	ContractAddress string
}
```

| Field | Go type | Meaning |
| --- | --- | --- |
| `Type` | `string` | `native`, `credit_alphanum4`, `credit_alphanum12`, or `soroban_token`. |
| `Code` | `string` | Asset code for credit assets. Empty for native and for Soroban tokens. |
| `Issuer` | `string` | Issuing account (`G...`) for credit assets. Empty otherwise. |
| `ContractAddress` | `string` | Token contract strkey (`C...`), set only for `soroban_token`. |

A Soroban token that wraps a classic asset through the Stellar Asset Contract
still arrives as `soroban_token` with only `ContractAddress` filled in. Mapping
it back to a code and issuer is not implemented; see
[issue #3](https://github.com/fadesany/Stellar-Trade-Processor/issues/3).

## Constants

```go
const (
	VenueClassicDEX  Venue = "classic_dex"
	VenuePathPayment Venue = "path_payment"
	VenueSorobanAMM  Venue = "soroban_amm"
)

const (
	AssetTypeNative           = "native"
	AssetTypeCreditAlphanum4  = "credit_alphanum4"
	AssetTypeCreditAlphanum12 = "credit_alphanum12"
	AssetTypeSorobanToken     = "soroban_token"
)
```

Two more protocol values are defined by the transformers rather than the event
package: `classic.ProtocolLiquidityPool` (`"liquidity_pool"`) and
`soroban.ProtocolSoroswap` (`"soroswap"`).

## Why amounts are strings

Amounts and prices never pass through a float. Classic amounts start as `int64`
stroops and are rendered by the SDK's `amount.StringFromInt64`. Soroswap
amounts start as `i128` values and are held as `big.Int`. Prices are computed
as `new(big.Rat).SetFrac(...)` and rendered with `FloatString(7)`, which
truncates rather than rounding up.

That truncation is worth remembering when a price is very small. A ratio below
0.00000005 renders as `0.0000000`. The amounts themselves are exact, so
recompute the ratio yourself if you need more precision than seven places.

## JSON encoding

`JSONLineSink` encodes the struct with `encoding/json` and no custom marshaller,
so field names in the output match the Go field names exactly, and `ClosedAt`
is RFC 3339:

```json
{"Venue":"path_payment","Protocol":"","BaseAsset":{"Type":"native","Code":"","Issuer":"","ContractAddress":""},"CounterAsset":{"Type":"credit_alphanum12","Code":"BITCOIN","Issuer":"GDRRAQK2L22YKWPQMNU7QGNQ4KL3SM6ML3WWQH62UQSZ4W7NOX3JKUMI","ContractAddress":""},"BaseAmount":"15.0000000","CounterAmount":"0.1250000","Price":"0.0083333","Account":"GB74WSCTZCC6EQD5X263ZXDANEOZRFBBBZOPIKQCFZ3HKQ4LQEXN7FNB","CounterAccount":"GDRWZVWNWEMQNWDBOS3TC4D6MPYVUIG2FSOHOGRZTB6AVVQZSYFRWITT","LedgerSequence":64446884,"TxHash":"84179ee4019a0d39c6116e2c91b34373b6a336fadd272fd8936fa8c2750e29b4","OpIndex":0,"ClosedAt":"2026-09-15T23:03:10Z"}
```

That event is real: ledger 64446884 on mainnet, an account paying 15 XLM along
a path and receiving 0.125 BITCOIN.
