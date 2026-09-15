// Package event defines the standardized trade event schema emitted by the
// Stellar Trade Processor, independent of the venue a trade executed on.
package event

import "time"

// Venue identifies the execution venue of a trade.
type Venue string

const (
	// VenueClassicDEX is a fill produced by a classic offer operation
	// (ManageSellOffer, ManageBuyOffer, CreatePassiveSellOffer).
	VenueClassicDEX Venue = "classic_dex"
	// VenuePathPayment is a fill produced while routing a classic path payment
	// (PathPaymentStrictSend, PathPaymentStrictReceive).
	VenuePathPayment Venue = "path_payment"
	// VenueSorobanAMM is a swap executed by a Soroban AMM contract.
	VenueSorobanAMM Venue = "soroban_amm"
)

// Asset type identifiers used in Asset.Type.
const (
	AssetTypeNative           = "native"
	AssetTypeCreditAlphanum4  = "credit_alphanum4"
	AssetTypeCreditAlphanum12 = "credit_alphanum12"
	AssetTypeSorobanToken     = "soroban_token"
)

// Asset describes one side of a trade.
type Asset struct {
	// Type is one of "native", "credit_alphanum4", "credit_alphanum12" or
	// "soroban_token".
	Type string
	// Code is the asset code for credit assets; empty otherwise.
	Code string
	// Issuer is the issuing account for credit assets; empty for native assets
	// and for soroban tokens identified purely by contract address.
	Issuer string
	// ContractAddress is the token contract strkey (C...) for soroban tokens.
	ContractAddress string
}

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
