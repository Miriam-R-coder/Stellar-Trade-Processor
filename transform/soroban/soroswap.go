package soroban

import (
	"fmt"
	"math/big"

	"github.com/fadesany/Stellar-Trade-Processor/event"
	"github.com/stellar/go-stellar-sdk/amount"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	// ProtocolSoroswap is the TradeEvent.Protocol value for Soroswap trades.
	ProtocolSoroswap = "soroswap"

	// SoroswapMainnetRouter is the Soroswap router contract on Stellar mainnet,
	// from soroswap/core public/mainnet.contracts.json.
	SoroswapMainnetRouter = "CAG5LRYQ5JVEUI5TEID72EYOVX44TTUJT5BQR2J6J77FH65PCCFAJDDH"

	soroswapRouterTopic = "SoroswapRouter"
	soroswapSwapTopic   = "swap"
)

// SoroswapAdapter decodes swaps from the Soroswap router's swap event.
//
// The router (soroswap/core contracts/router/src/event.rs) publishes
// topics ("SoroswapRouter", symbol "swap") with data
//
//	SwapEvent { path: Vec<Address>, amounts: Vec<i128>, to: Address }
//
// encoded as an ScMap keyed by field-name symbols. amounts[i] is the amount of
// token path[i] at step i: amounts[0] is the input and the last element the
// output. The event is emitted by both swap_exact_tokens_for_tokens and
// swap_tokens_for_exact_tokens.
//
// Swaps that invoke a Soroswap pair directly, bypassing the router, emit only
// the pair's swap event, which carries no token addresses, and are not
// decoded by this adapter.
type SoroswapAdapter struct {
	routers map[string]struct{}
}

// NewSoroswapAdapter returns an adapter matching the given router contract
// addresses. With no arguments it matches SoroswapMainnetRouter.
func NewSoroswapAdapter(routers ...string) *SoroswapAdapter {
	if len(routers) == 0 {
		routers = []string{SoroswapMainnetRouter}
	}
	set := make(map[string]struct{}, len(routers))
	for _, r := range routers {
		set[r] = struct{}{}
	}
	return &SoroswapAdapter{routers: set}
}

// Name returns "soroswap".
func (a *SoroswapAdapter) Name() string {
	return ProtocolSoroswap
}

// Matches reports whether contractAddress is a configured Soroswap router.
func (a *SoroswapAdapter) Matches(contractAddress string) bool {
	_, ok := a.routers[contractAddress]
	return ok
}

// ExtractTrades emits one TradeEvent per hop of every router swap event.
// Amounts are the raw i128 token amounts in each token's smallest unit, and
// Price is the ratio of those raw amounts, because token decimals are not
// available from the event. Non-swap router events are ignored.
func (a *SoroswapAdapter) ExtractTrades(tx TxContext, events []xdr.ContractEvent) ([]event.TradeEvent, error) {
	var trades []event.TradeEvent
	for i, ev := range events {
		if ev.Body.V != 0 || ev.Body.V0 == nil {
			return nil, fmt.Errorf("event %d: unsupported contract event body version %d", i, ev.Body.V)
		}
		if !isRouterSwap(ev.Body.V0.Topics) {
			continue
		}

		swap, err := decodeSwapEvent(ev.Body.V0.Data)
		if err != nil {
			return nil, fmt.Errorf("event %d: decoding soroswap swap event: %w", i, err)
		}

		for hop := 0; hop+1 < len(swap.path); hop++ {
			baseAmount, counterAmount := swap.amounts[hop], swap.amounts[hop+1]
			trades = append(trades, event.TradeEvent{
				Venue:          event.VenueSorobanAMM,
				Protocol:       ProtocolSoroswap,
				BaseAsset:      event.Asset{Type: event.AssetTypeSorobanToken, ContractAddress: swap.path[hop]},
				CounterAsset:   event.Asset{Type: event.AssetTypeSorobanToken, ContractAddress: swap.path[hop+1]},
				BaseAmount:     baseAmount.String(),
				CounterAmount:  counterAmount.String(),
				Price:          new(big.Rat).SetFrac(counterAmount, baseAmount).FloatString(7),
				Account:        swap.to,
				LedgerSequence: tx.LedgerSequence,
				TxHash:         tx.TxHash,
				OpIndex:        tx.OpIndex,
				ClosedAt:       tx.ClosedAt,
			})
		}
	}
	return trades, nil
}

// isRouterSwap reports whether topics identify a Soroswap router swap event.
// The first topic is published from a Rust &str, which soroban-sdk encodes as
// an ScString; a Symbol is accepted as well.
func isRouterSwap(topics []xdr.ScVal) bool {
	if len(topics) != 2 {
		return false
	}
	first, ok := stringOrSymbol(topics[0])
	if !ok || first != soroswapRouterTopic {
		return false
	}
	second, ok := stringOrSymbol(topics[1])
	return ok && second == soroswapSwapTopic
}

func stringOrSymbol(v xdr.ScVal) (string, bool) {
	switch v.Type {
	case xdr.ScValTypeScvString:
		if v.Str == nil {
			return "", false
		}
		return string(*v.Str), true
	case xdr.ScValTypeScvSymbol:
		if v.Sym == nil {
			return "", false
		}
		return string(*v.Sym), true
	default:
		return "", false
	}
}

type swapEvent struct {
	path    []string
	amounts []*big.Int
	to      string
}

// decodeSwapEvent parses the SwapEvent map. Fields are looked up by name, not
// position. Union arms are checked for nil directly because the generated
// ScVal accessors dereference them.
func decodeSwapEvent(data xdr.ScVal) (swapEvent, error) {
	if data.Type != xdr.ScValTypeScvMap || data.Map == nil || *data.Map == nil {
		return swapEvent{}, fmt.Errorf("data is %s, want map", data.Type)
	}

	fields := make(map[string]xdr.ScVal, len(**data.Map))
	for _, entry := range **data.Map {
		if entry.Key.Type != xdr.ScValTypeScvSymbol || entry.Key.Sym == nil {
			return swapEvent{}, fmt.Errorf("map key is %s, want symbol", entry.Key.Type)
		}
		fields[string(*entry.Key.Sym)] = entry.Val
	}

	pathVal, ok := fields["path"]
	if !ok {
		return swapEvent{}, fmt.Errorf("missing field path")
	}
	amountsVal, ok := fields["amounts"]
	if !ok {
		return swapEvent{}, fmt.Errorf("missing field amounts")
	}
	toVal, ok := fields["to"]
	if !ok {
		return swapEvent{}, fmt.Errorf("missing field to")
	}

	pathVec, err := vec(pathVal)
	if err != nil {
		return swapEvent{}, fmt.Errorf("field path: %w", err)
	}
	amountsVec, err := vec(amountsVal)
	if err != nil {
		return swapEvent{}, fmt.Errorf("field amounts: %w", err)
	}
	if len(pathVec) < 2 {
		return swapEvent{}, fmt.Errorf("path has %d tokens, want at least 2", len(pathVec))
	}
	if len(pathVec) != len(amountsVec) {
		return swapEvent{}, fmt.Errorf("path has %d tokens but amounts has %d entries", len(pathVec), len(amountsVec))
	}

	out := swapEvent{
		path:    make([]string, len(pathVec)),
		amounts: make([]*big.Int, len(amountsVec)),
	}
	for i, v := range pathVec {
		addr, err := scAddress(v, true)
		if err != nil {
			return swapEvent{}, fmt.Errorf("path[%d]: %w", i, err)
		}
		out.path[i] = addr
	}
	for i, v := range amountsVec {
		n, err := positiveI128(v)
		if err != nil {
			return swapEvent{}, fmt.Errorf("amounts[%d]: %w", i, err)
		}
		out.amounts[i] = n
	}
	out.to, err = scAddress(toVal, false)
	if err != nil {
		return swapEvent{}, fmt.Errorf("field to: %w", err)
	}
	return out, nil
}

func vec(v xdr.ScVal) (xdr.ScVec, error) {
	if v.Type != xdr.ScValTypeScvVec || v.Vec == nil || *v.Vec == nil {
		return nil, fmt.Errorf("value is %s, want vec", v.Type)
	}
	return **v.Vec, nil
}

// scAddress renders an address ScVal as a strkey. When contractOnly is set,
// account addresses are rejected.
func scAddress(v xdr.ScVal, contractOnly bool) (string, error) {
	if v.Type != xdr.ScValTypeScvAddress || v.Address == nil {
		return "", fmt.Errorf("value is %s, want address", v.Type)
	}
	addr := *v.Address
	switch addr.Type {
	case xdr.ScAddressTypeScAddressTypeContract:
		if addr.ContractId == nil {
			return "", fmt.Errorf("contract address has no id")
		}
	case xdr.ScAddressTypeScAddressTypeAccount:
		if contractOnly {
			return "", fmt.Errorf("account address where token contract expected")
		}
		if addr.AccountId == nil || addr.AccountId.Ed25519 == nil {
			return "", fmt.Errorf("account address has no key")
		}
	default:
		return "", fmt.Errorf("unsupported address type %s", addr.Type)
	}
	s, err := addr.String()
	if err != nil {
		return "", fmt.Errorf("encoding address: %w", err)
	}
	return s, nil
}

func positiveI128(v xdr.ScVal) (*big.Int, error) {
	if v.Type != xdr.ScValTypeScvI128 || v.I128 == nil {
		return nil, fmt.Errorf("value is %s, want i128", v.Type)
	}
	n, ok := new(big.Int).SetString(amount.String128Raw(*v.I128), 10)
	if !ok {
		return nil, fmt.Errorf("parsing i128 amount")
	}
	if n.Sign() <= 0 {
		return nil, fmt.Errorf("amount %s is not positive", n)
	}
	return n, nil
}
