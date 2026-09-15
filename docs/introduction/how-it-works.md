# How It Works

The pipeline has three roles and one event type.

1. A **Source** hands you one ledger at a time, in order. `BackfillSource`
   walks a fixed range and stops. `StreamingSource` follows the network and
   keeps going.
2. The ledger is read into transactions with the ingest SDK's
   `ingest.LedgerTransactionReader`.
3. Each transaction goes to every **Transformer** you wired up.
   `classic.Transformer` looks at operation results. `soroban.Transformer`
   looks at contract events.
4. A transformer returns zero, one, or many **TradeEvent** values. A
   transaction with no trades returns none. A failed transaction returns none.
   Malformed input returns an error rather than a panic.
5. The events go to a **Sink**. `JSONLineSink` writes one JSON object per line.

Nothing is buffered between ledgers. Events for ledger N are written before
ledger N+1 is fetched, so output order matches ledger order.

## A classic offer fill, start to finish

Here is a real one from mainnet ledger 64446884.

An account submits a `ManageSellOffer`. It crosses an existing offer, and the
operation result contains a claimed offer atom. The atom says the offer owner,
`GCHGOYKETAR24WQ4KRIWHHMJ4LIXN2MD6J4QPG5JSWZWVRSG3Q34KZVN`, sold 0.0001000
USDC and bought 0.0000009 XLM.

The transformer reads that from the taker's side. What the offer owner bought
is what the taker gave up, so XLM becomes the base asset. What the offer owner
sold is what the taker received, so USDC becomes the counter asset. The price
is counter divided by base, computed with `big.Rat` and rendered to seven
decimal places.

```json
{
  "Venue": "classic_dex",
  "Protocol": "",
  "BaseAsset": {"Type": "native", "Code": "", "Issuer": "", "ContractAddress": ""},
  "CounterAsset": {"Type": "credit_alphanum4", "Code": "USDC", "Issuer": "GAHJHC4RQLDUZUFZDCSSE4MUZC4QYL3ELSSQX5M6Y2NSU5DROUKQKZVN", "ContractAddress": ""},
  "BaseAmount": "0.0000009",
  "CounterAmount": "0.0001000",
  "Price": "111.1111111",
  "Account": "GAWA5KLUWNBRJM5L7HF466SJPTAOM7CXRTT7PRRWQ5BVFQKAWC7POVZH",
  "CounterAccount": "GCHGOYKETAR24WQ4KRIWHHMJ4LIXN2MD6J4QPG5JSWZWVRSG3Q34KZVN",
  "LedgerSequence": 64446884,
  "TxHash": "d3295ef2ce2bba5834c85ce8e76851ec323d25f4496357a56c75e2532c363aaf",
  "OpIndex": 0,
  "ClosedAt": "2026-09-15T23:03:10Z"
}
```

`Account` is the operation's source account, or the transaction source when the
operation does not set its own. `CounterAccount` is the offer owner. If the
same operation had crossed four offers, you would get four events, all with the
same `TxHash` and `OpIndex`.

## A Soroswap swap, start to finish

Here is a real multi-hop swap from mainnet ledger 64446708, transaction
`01c9b5324f26294b579a76befd0f3de5f770cdc49473f983e083f328b411a2ea`.

The transaction invokes the Soroswap router. The router emits one contract
event with topics `("SoroswapRouter", "swap")` and a data map holding `path`
(three token contracts), `amounts` (three i128 values), and `to`. The Soroban
transformer groups the operation's contract events by emitting contract,
notices the router address is registered, and hands the events to
`SoroswapAdapter`.

The path has three tokens, so there are two hops, and the adapter emits two
events. The first hop's output amount, 14146404, is the second hop's input
amount, which is how you can tell they are legs of one route:

```json
{"Venue":"soroban_amm","Protocol":"soroswap","BaseAsset":{"Type":"soroban_token","Code":"","Issuer":"","ContractAddress":"CD25MNVTZDL4Y3XBCPCJXGXATV5WUHHOWMYFF4YBEGU5FCPGMYTVG5JY"},"CounterAsset":{"Type":"soroban_token","Code":"","Issuer":"","ContractAddress":"CCW67TSZV3SSS2HXMBQ5JFGCKJNXKZM7UQUWUZPUTHXSTZLEO7SJMI75"},"BaseAmount":"3138770434","CounterAmount":"14146404","Price":"0.0045070","Account":"GD3WMHKBP4YDNXSLG3CT2SKNFDJDAM5CF2IZDD4BFVSHIHJQ3KFELMUT","CounterAccount":"","LedgerSequence":64446708,"TxHash":"01c9b5324f26294b579a76befd0f3de5f770cdc49473f983e083f328b411a2ea","OpIndex":0,"ClosedAt":"2026-09-15T22:46:51Z"}
{"Venue":"soroban_amm","Protocol":"soroswap","BaseAsset":{"Type":"soroban_token","Code":"","Issuer":"","ContractAddress":"CCW67TSZV3SSS2HXMBQ5JFGCKJNXKZM7UQUWUZPUTHXSTZLEO7SJMI75"},"CounterAsset":{"Type":"soroban_token","Code":"","Issuer":"","ContractAddress":"CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA"},"BaseAmount":"14146404","CounterAmount":"80061031","Price":"5.6594617","Account":"GD3WMHKBP4YDNXSLG3CT2SKNFDJDAM5CF2IZDD4BFVSHIHJQ3KFELMUT","CounterAccount":"","LedgerSequence":64446708,"TxHash":"01c9b5324f26294b579a76befd0f3de5f770cdc49473f983e083f328b411a2ea","OpIndex":0,"ClosedAt":"2026-09-15T22:46:51Z"}
```

Two differences from the classic case are worth noticing. `CounterAccount` is
empty, because the counterparty is a pool, not an account. And the amounts are
raw integers in each token's smallest unit, not decimal-scaled, because the
event does not carry token decimals. `Price` is the ratio of those raw amounts,
so it is only directly comparable to a classic price when both tokens use seven
decimals.

## What this looks like at volume

A backfill of 281 mainnet ledgers (64446600 to 64446880) produced 21,101 trade
events: 399 classic DEX fills, 6,291 path payment fills against order book
offers, 14,405 path payment fills against classic liquidity pools, and 6
Soroswap swaps. Liquidity pool fills dominating the mix is normal for mainnet
traffic in that window.
