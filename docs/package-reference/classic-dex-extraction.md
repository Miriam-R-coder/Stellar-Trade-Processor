# Classic DEX Extraction

`classic.Transformer` handles five operation types:

- `ManageSellOffer`
- `ManageBuyOffer`
- `CreatePassiveSellOffer`
- `PathPaymentStrictSend`
- `PathPaymentStrictReceive`

Everything else returns no claims and no error, so a transaction full of
payments and trustline changes costs one type switch per operation.

## What a claim atom is

When an offer operation crosses existing offers, the operation result carries
`OffersClaimed`, a list of claim atoms. Each atom is one fill against one
counterparty. An operation that crosses four offers produces four atoms.

Each atom is written from the **claimed** party's point of view. It says what
that party sold and what it bought:

| Atom field | Meaning |
| --- | --- |
| `AssetSold` / `AmountSold` | What the offer owner (or pool) gave up |
| `AssetBought` / `AmountBought` | What the offer owner (or pool) received |
| `SellerId` | The offer owner's account (order book atoms only) |

The transformer flips this to the taker's perspective, because the taker is the
account whose trade we are reporting:

```go
given := int64(claim.AmountBought())
received := int64(claim.AmountSold())
```

So `BaseAsset` is the atom's `AssetBought` and `CounterAsset` is its
`AssetSold`. Get this backwards and every price in your dataset is inverted,
which is the single most common way to break trade extraction.

There are three atom types, and all three are handled:

| Claim atom type | `Protocol` | `CounterAccount` |
| --- | --- | --- |
| `ClaimAtomTypeV0` | `""` | Seller account |
| `ClaimAtomTypeOrderBook` | `""` | Seller account |
| `ClaimAtomTypeLiquidityPool` | `"liquidity_pool"` | `""` (empty) |

The pool ID is not currently recorded on the event.

## Worked example: an offer fill

Real trade, mainnet ledger 64446884, transaction
`d3295ef2ce2bba5834c85ce8e76851ec323d25f4496357a56c75e2532c363aaf`.

Account `GAWA5KLU...C7POVZH` submits a `ManageSellOffer`. It crosses one offer
owned by `GCHGOYKE...G3Q34KZVN`. The claim atom says that owner sold 0.0001000
USDC and bought 0.0000009 XLM.

Flipping to the taker's side: the taker gave 0.0000009 XLM and received
0.0001000 USDC. Price is 0.0001000 / 0.0000009 = 111.1111111 after truncation
to seven places.

```go
event.TradeEvent{
	Venue:          event.VenueClassicDEX,
	Protocol:       "",
	BaseAsset:      event.Asset{Type: "native"},
	CounterAsset:   event.Asset{Type: "credit_alphanum4", Code: "USDC", Issuer: "GAHJHC4RQLDUZUFZDCSSE4MUZC4QYL3ELSSQX5M6Y2NSU5DROUKQKZVN"},
	BaseAmount:     "0.0000009",
	CounterAmount:  "0.0001000",
	Price:          "111.1111111",
	Account:        "GAWA5KLUWNBRJM5L7HF466SJPTAOM7CXRTT7PRRWQ5BVFQKAWC7POVZH",
	CounterAccount: "GCHGOYKETAR24WQ4KRIWHHMJ4LIXN2MD6J4QPG5JSWZWVRSG3Q34KZVN",
	LedgerSequence: 64446884,
	TxHash:         "d3295ef2ce2bba5834c85ce8e76851ec323d25f4496357a56c75e2532c363aaf",
	OpIndex:        0,
	ClosedAt:       time.Date(2026, 9, 15, 23, 3, 10, 0, time.UTC),
}
```

## What a path payment is

A path payment moves value from a send asset to a destination asset by crossing
the order book (or pools) along a route. `PathPaymentStrictSend` fixes the
amount sent; `PathPaymentStrictReceive` fixes the amount received. Both results
carry an `Offers` list, which holds the same claim atoms as an offer fill.

Each atom is one **hop**. A route of `USDC → XLM → EURT` crosses two markets
and produces two atoms, so the transformer emits two events with the same
`TxHash` and `OpIndex`, both with `Venue: path_payment`.

## Worked example: a two-hop path payment

These numbers come from the repository's own test suite
(`transform/classic/pathpayment_test.go`), where they are asserted exactly.

A sender pays 10 USDC and the route runs `USDC → XLM → EURT`:

- **Hop 1** is an order book claim: the offer owner sells 40 XLM and buys the sender's 10 USDC.
- **Hop 2** is a liquidity pool claim: the pool sells 9 EURT and buys the 40 XLM.

Two events come out:

```go
// Hop 1: gave 10 USDC, received 40 XLM, price 4.0000000
event.TradeEvent{
	Venue:          event.VenuePathPayment,
	Protocol:       "",
	BaseAsset:      event.Asset{Type: "credit_alphanum4", Code: "USDC", Issuer: "..."},
	CounterAsset:   event.Asset{Type: "native"},
	BaseAmount:     "10.0000000",
	CounterAmount:  "40.0000000",
	Price:          "4.0000000",
	CounterAccount: "<offer owner>",
}

// Hop 2: gave 40 XLM, received 9 EURT, price 0.2250000, no counter account
event.TradeEvent{
	Venue:          event.VenuePathPayment,
	Protocol:       "liquidity_pool",
	BaseAsset:      event.Asset{Type: "native"},
	CounterAsset:   event.Asset{Type: "credit_alphanum4", Code: "EURT", Issuer: "..."},
	BaseAmount:     "40.0000000",
	CounterAmount:  "9.0000000",
	Price:          "0.2250000",
	CounterAccount: "",
}
```

The chaining is visible in the amounts: hop 1's output, 40 XLM, is hop 2's
input. A path payment between two identical assets crosses nothing, produces no
atoms, and emits no events.

## Rules the transformer applies

- **Failed transactions produce nothing.** `tx.Successful()` is checked first.
- **`Account` is the operation source**, falling back to the transaction source when the operation does not set one. Muxed accounts resolve to the underlying `G...` address.
- **Zero-amount atoms are skipped.** An atom with 0 on either side has no meaningful price, so it is dropped rather than emitting a division by zero.
- **Negative amounts are an error**, not a skip.
- **Amounts use `amount.StringFromInt64`**, giving the usual 7-decimal Stellar representation.
- **Prices use `big.Rat`** and `FloatString(7)`.

## Errors rather than panics

The generated XDR accessors in the Go SDK dereference union arms without
checking them, so calling `GetManageSellOfferResult()` on a result whose body is
missing crashes the process. This transformer checks the pointers itself:

```go
case xdr.OperationTypeManageSellOffer:
	res := tr.ManageSellOfferResult
	if res == nil {
		return nil, "", fmt.Errorf("manage sell offer result is missing")
	}
	if res.Code != xdr.ManageSellOfferResultCodeManageSellOfferSuccess || res.Success == nil {
		return nil, "", fmt.Errorf("manage sell offer result code %d in successful transaction", res.Code)
	}
	return res.Success.OffersClaimed, event.VenueClassicDEX, nil
```

The same applies to claim atom bodies and to asset unions. Conditions that
return an error include: an operation result that is not `opINNER`, a result
type that does not match its operation, a missing success body, an operation and
result count mismatch, an unknown claim atom type, and a credit asset with no
issuer key.
