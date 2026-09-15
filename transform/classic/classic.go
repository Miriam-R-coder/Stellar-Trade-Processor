// Package classic derives trade events from classic Stellar DEX operations:
// offer management operations and path payments.
package classic

import (
	"fmt"
	"math/big"
	"time"

	"github.com/fadesany/Stellar-Trade-Processor/event"
	"github.com/stellar/go-stellar-sdk/amount"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// ProtocolLiquidityPool is the TradeEvent.Protocol value for fills against a
// classic liquidity pool rather than an orderbook offer.
const ProtocolLiquidityPool = "liquidity_pool"

// Transformer extracts trades from classic offer and path payment operations.
// It is stateless and safe for concurrent use.
type Transformer struct{}

// NewTransformer returns a classic Transformer.
func NewTransformer() *Transformer {
	return &Transformer{}
}

// Transform returns one TradeEvent per claimed offer or liquidity pool fill
// produced by the classic trading operations in tx. Failed transactions yield
// no events.
func (t *Transformer) Transform(tx ingest.LedgerTransaction, ledgerSeq uint32, closedAt time.Time) ([]event.TradeEvent, error) {
	if !tx.Successful() {
		return nil, nil
	}
	txHash := tx.Hash.HexString()

	ops := tx.Envelope.Operations()
	results, ok := tx.Result.OperationResults()
	if !ok {
		return nil, fmt.Errorf("transaction %s: missing operation results", txHash)
	}
	if len(results) != len(ops) {
		return nil, fmt.Errorf("transaction %s: %d operations but %d results", txHash, len(ops), len(results))
	}

	txSource := tx.Envelope.SourceAccount().ToAccountId().Address()

	var events []event.TradeEvent
	for i, op := range ops {
		claims, venue, err := claimsForOperation(op, results[i])
		if err != nil {
			return nil, fmt.Errorf("transaction %s op %d: %w", txHash, i, err)
		}
		if len(claims) == 0 {
			continue
		}

		account := txSource
		if op.SourceAccount != nil {
			account = op.SourceAccount.ToAccountId().Address()
		}
		template := event.TradeEvent{
			Venue:          venue,
			Account:        account,
			LedgerSequence: ledgerSeq,
			TxHash:         txHash,
			OpIndex:        i,
			ClosedAt:       closedAt,
		}

		for j, claim := range claims {
			ev, ok, err := tradeFromClaim(template, claim)
			if err != nil {
				return nil, fmt.Errorf("transaction %s op %d claim %d: %w", txHash, i, j, err)
			}
			if ok {
				events = append(events, ev)
			}
		}
	}
	return events, nil
}

// claimsForOperation returns the claim atoms produced by op, and the venue
// they should be reported under. Operations that cannot trade return no
// claims and no error.
func claimsForOperation(op xdr.Operation, result xdr.OperationResult) ([]xdr.ClaimAtom, event.Venue, error) {
	switch op.Body.Type {
	case xdr.OperationTypeManageSellOffer,
		xdr.OperationTypeManageBuyOffer,
		xdr.OperationTypeCreatePassiveSellOffer,
		xdr.OperationTypePathPaymentStrictSend,
		xdr.OperationTypePathPaymentStrictReceive:
	default:
		return nil, "", nil
	}

	tr, err := innerResult(op, result)
	if err != nil {
		return nil, "", err
	}

	switch op.Body.Type {
	case xdr.OperationTypePathPaymentStrictSend, xdr.OperationTypePathPaymentStrictReceive:
		claims, err := pathPaymentClaims(tr)
		if err != nil {
			return nil, "", err
		}
		return claims, event.VenuePathPayment, nil

	// Union arms are checked for nil directly: the generated Get* accessors
	// dereference the arm pointer and would panic on malformed input.
	case xdr.OperationTypeManageSellOffer:
		res := tr.ManageSellOfferResult
		if res == nil {
			return nil, "", fmt.Errorf("manage sell offer result is missing")
		}
		if res.Code != xdr.ManageSellOfferResultCodeManageSellOfferSuccess || res.Success == nil {
			return nil, "", fmt.Errorf("manage sell offer result code %d in successful transaction", res.Code)
		}
		return res.Success.OffersClaimed, event.VenueClassicDEX, nil

	case xdr.OperationTypeManageBuyOffer:
		res := tr.ManageBuyOfferResult
		if res == nil {
			return nil, "", fmt.Errorf("manage buy offer result is missing")
		}
		if res.Code != xdr.ManageBuyOfferResultCodeManageBuyOfferSuccess || res.Success == nil {
			return nil, "", fmt.Errorf("manage buy offer result code %d in successful transaction", res.Code)
		}
		return res.Success.OffersClaimed, event.VenueClassicDEX, nil

	default: // xdr.OperationTypeCreatePassiveSellOffer
		res := tr.CreatePassiveSellOfferResult
		if res == nil {
			return nil, "", fmt.Errorf("create passive sell offer result is missing")
		}
		if res.Code != xdr.ManageSellOfferResultCodeManageSellOfferSuccess || res.Success == nil {
			return nil, "", fmt.Errorf("create passive sell offer result code %d in successful transaction", res.Code)
		}
		return res.Success.OffersClaimed, event.VenueClassicDEX, nil
	}
}

// innerResult validates that result is an inner result matching op's type.
func innerResult(op xdr.Operation, result xdr.OperationResult) (xdr.OperationResultTr, error) {
	if result.Code != xdr.OperationResultCodeOpInner {
		return xdr.OperationResultTr{}, fmt.Errorf("operation result code %d is not opINNER", result.Code)
	}
	if result.Tr == nil {
		return xdr.OperationResultTr{}, fmt.Errorf("operation result has no inner result")
	}
	if result.Tr.Type != op.Body.Type {
		return xdr.OperationResultTr{}, fmt.Errorf("operation type %s does not match result type %s", op.Body.Type, result.Tr.Type)
	}
	return *result.Tr, nil
}

// tradeFromClaim fills template with the trade described by claim, seen from
// the taker's perspective: the taker gives the asset the claimed party bought
// and receives the asset the claimed party sold. Claims with a zero amount on
// either side carry no price and are skipped (ok is false).
func tradeFromClaim(template event.TradeEvent, claim xdr.ClaimAtom) (event.TradeEvent, bool, error) {
	ev := template
	switch claim.Type {
	case xdr.ClaimAtomTypeClaimAtomTypeV0:
		if claim.V0 == nil {
			return event.TradeEvent{}, false, fmt.Errorf("v0 claim atom has no body")
		}
		ev.CounterAccount = claim.SellerId().Address()
	case xdr.ClaimAtomTypeClaimAtomTypeOrderBook:
		if claim.OrderBook == nil {
			return event.TradeEvent{}, false, fmt.Errorf("orderbook claim atom has no body")
		}
		ev.CounterAccount = claim.SellerId().Address()
	case xdr.ClaimAtomTypeClaimAtomTypeLiquidityPool:
		if claim.LiquidityPool == nil {
			return event.TradeEvent{}, false, fmt.Errorf("liquidity pool claim atom has no body")
		}
		ev.Protocol = ProtocolLiquidityPool
	default:
		return event.TradeEvent{}, false, fmt.Errorf("unknown claim atom type %d", claim.Type)
	}

	given := int64(claim.AmountBought())
	received := int64(claim.AmountSold())
	if given < 0 || received < 0 {
		return event.TradeEvent{}, false, fmt.Errorf("negative claim amounts: bought %d, sold %d", given, received)
	}
	if given == 0 || received == 0 {
		return event.TradeEvent{}, false, nil
	}

	base, err := convertAsset(claim.AssetBought())
	if err != nil {
		return event.TradeEvent{}, false, fmt.Errorf("converting bought asset: %w", err)
	}
	counter, err := convertAsset(claim.AssetSold())
	if err != nil {
		return event.TradeEvent{}, false, fmt.Errorf("converting sold asset: %w", err)
	}

	ev.BaseAsset = base
	ev.CounterAsset = counter
	ev.BaseAmount = amount.StringFromInt64(given)
	ev.CounterAmount = amount.StringFromInt64(received)
	ev.Price = new(big.Rat).SetFrac(big.NewInt(received), big.NewInt(given)).FloatString(7)
	return ev, true, nil
}

// convertAsset maps a classic xdr.Asset to an event.Asset, returning an error
// instead of panicking on malformed unions.
func convertAsset(a xdr.Asset) (event.Asset, error) {
	switch a.Type {
	case xdr.AssetTypeAssetTypeNative:
	case xdr.AssetTypeAssetTypeCreditAlphanum4:
		if a.AlphaNum4 == nil || a.AlphaNum4.Issuer.Ed25519 == nil {
			return event.Asset{}, fmt.Errorf("malformed credit_alphanum4 asset")
		}
	case xdr.AssetTypeAssetTypeCreditAlphanum12:
		if a.AlphaNum12 == nil || a.AlphaNum12.Issuer.Ed25519 == nil {
			return event.Asset{}, fmt.Errorf("malformed credit_alphanum12 asset")
		}
	default:
		return event.Asset{}, fmt.Errorf("unknown asset type %d", a.Type)
	}

	var typ, code, issuer string
	if err := a.Extract(&typ, &code, &issuer); err != nil {
		return event.Asset{}, fmt.Errorf("extracting asset: %w", err)
	}
	return event.Asset{Type: typ, Code: code, Issuer: issuer}, nil
}
