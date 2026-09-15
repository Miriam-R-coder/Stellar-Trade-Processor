package classic

import (
	"strings"
	"testing"
	"time"

	"github.com/fadesany/Stellar-Trade-Processor/event"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

var (
	testClosedAt = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	testLedger   = uint32(55_000_000)
	testTxHash   = xdr.Hash{0xab, 0xcd}
)

func testKey(b byte) xdr.Uint256 {
	var u xdr.Uint256
	for i := range u {
		u[i] = b
	}
	return u
}

func accountID(b byte) xdr.AccountId {
	u := testKey(b)
	return xdr.AccountId{Type: xdr.PublicKeyTypePublicKeyTypeEd25519, Ed25519: &u}
}

func muxed(b byte) xdr.MuxedAccount {
	u := testKey(b)
	return xdr.MuxedAccount{Type: xdr.CryptoKeyTypeKeyTypeEd25519, Ed25519: &u}
}

func address(b byte) string {
	u := testKey(b)
	return strkey.MustEncode(strkey.VersionByteAccountID, u[:])
}

func nativeAsset() xdr.Asset {
	return xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}
}

func creditAsset(code string, issuer byte) xdr.Asset {
	if len(code) <= 4 {
		var c xdr.AssetCode4
		copy(c[:], code)
		return xdr.Asset{
			Type:      xdr.AssetTypeAssetTypeCreditAlphanum4,
			AlphaNum4: &xdr.AlphaNum4{AssetCode: c, Issuer: accountID(issuer)},
		}
	}
	var c xdr.AssetCode12
	copy(c[:], code)
	return xdr.Asset{
		Type:       xdr.AssetTypeAssetTypeCreditAlphanum12,
		AlphaNum12: &xdr.AlphaNum12{AssetCode: c, Issuer: accountID(issuer)},
	}
}

func orderBookClaim(seller byte, sold xdr.Asset, amountSold int64, bought xdr.Asset, amountBought int64) xdr.ClaimAtom {
	return xdr.ClaimAtom{
		Type: xdr.ClaimAtomTypeClaimAtomTypeOrderBook,
		OrderBook: &xdr.ClaimOfferAtom{
			SellerId:     accountID(seller),
			OfferId:      42,
			AssetSold:    sold,
			AmountSold:   xdr.Int64(amountSold),
			AssetBought:  bought,
			AmountBought: xdr.Int64(amountBought),
		},
	}
}

func poolClaim(sold xdr.Asset, amountSold int64, bought xdr.Asset, amountBought int64) xdr.ClaimAtom {
	return xdr.ClaimAtom{
		Type: xdr.ClaimAtomTypeClaimAtomTypeLiquidityPool,
		LiquidityPool: &xdr.ClaimLiquidityAtom{
			LiquidityPoolId: xdr.PoolId{0x01},
			AssetSold:       sold,
			AmountSold:      xdr.Int64(amountSold),
			AssetBought:     bought,
			AmountBought:    xdr.Int64(amountBought),
		},
	}
}

func claimV0(seller byte, sold xdr.Asset, amountSold int64, bought xdr.Asset, amountBought int64) xdr.ClaimAtom {
	return xdr.ClaimAtom{
		Type: xdr.ClaimAtomTypeClaimAtomTypeV0,
		V0: &xdr.ClaimOfferAtomV0{
			SellerEd25519: testKey(seller),
			OfferId:       42,
			AssetSold:     sold,
			AmountSold:    xdr.Int64(amountSold),
			AssetBought:   bought,
			AmountBought:  xdr.Int64(amountBought),
		},
	}
}

func manageSellOp(source *xdr.MuxedAccount) xdr.Operation {
	return xdr.Operation{
		SourceAccount: source,
		Body:          xdr.OperationBody{Type: xdr.OperationTypeManageSellOffer, ManageSellOfferOp: &xdr.ManageSellOfferOp{}},
	}
}

func manageBuyOp() xdr.Operation {
	return xdr.Operation{
		Body: xdr.OperationBody{Type: xdr.OperationTypeManageBuyOffer, ManageBuyOfferOp: &xdr.ManageBuyOfferOp{}},
	}
}

func passiveSellOp() xdr.Operation {
	return xdr.Operation{
		Body: xdr.OperationBody{Type: xdr.OperationTypeCreatePassiveSellOffer, CreatePassiveSellOfferOp: &xdr.CreatePassiveSellOfferOp{}},
	}
}

func manageSellResult(claims ...xdr.ClaimAtom) xdr.OperationResult {
	return xdr.OperationResult{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypeManageSellOffer,
			ManageSellOfferResult: &xdr.ManageSellOfferResult{
				Code:    xdr.ManageSellOfferResultCodeManageSellOfferSuccess,
				Success: &xdr.ManageOfferSuccessResult{OffersClaimed: claims},
			},
		},
	}
}

func manageBuyResult(claims ...xdr.ClaimAtom) xdr.OperationResult {
	return xdr.OperationResult{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypeManageBuyOffer,
			ManageBuyOfferResult: &xdr.ManageBuyOfferResult{
				Code:    xdr.ManageBuyOfferResultCodeManageBuyOfferSuccess,
				Success: &xdr.ManageOfferSuccessResult{OffersClaimed: claims},
			},
		},
	}
}

func passiveSellResult(claims ...xdr.ClaimAtom) xdr.OperationResult {
	return xdr.OperationResult{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypeCreatePassiveSellOffer,
			CreatePassiveSellOfferResult: &xdr.ManageSellOfferResult{
				Code:    xdr.ManageSellOfferResultCodeManageSellOfferSuccess,
				Success: &xdr.ManageOfferSuccessResult{OffersClaimed: claims},
			},
		},
	}
}

// buildTx assembles a LedgerTransaction with source account 0x01.
func buildTx(code xdr.TransactionResultCode, ops []xdr.Operation, results []xdr.OperationResult) ingest.LedgerTransaction {
	return ingest.LedgerTransaction{
		Hash: testTxHash,
		Envelope: xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTx,
			V1: &xdr.TransactionV1Envelope{
				Tx: xdr.Transaction{SourceAccount: muxed(0x01), Operations: ops},
			},
		},
		Result: xdr.TransactionResultPair{
			TransactionHash: testTxHash,
			Result: xdr.TransactionResult{
				Result: xdr.TransactionResultResult{Code: code, Results: &results},
			},
		},
	}
}

func TestTransformOfferFills(t *testing.T) {
	usdc := creditAsset("USDC", 0x09)
	longAsset := creditAsset("LONGASSET", 0x08)
	opSource := muxed(0x02)

	usdcEvent := event.Asset{Type: event.AssetTypeCreditAlphanum4, Code: "USDC", Issuer: address(0x09)}
	nativeEvent := event.Asset{Type: event.AssetTypeNative}
	longEvent := event.Asset{Type: event.AssetTypeCreditAlphanum12, Code: "LONGASSET", Issuer: address(0x08)}

	base := func(opIndex int, account string) event.TradeEvent {
		return event.TradeEvent{
			Venue:          event.VenueClassicDEX,
			Account:        account,
			LedgerSequence: testLedger,
			TxHash:         testTxHash.HexString(),
			OpIndex:        opIndex,
			ClosedAt:       testClosedAt,
		}
	}

	withTrade := func(ev event.TradeEvent, protocol string, baseAsset, counterAsset event.Asset, baseAmt, counterAmt, price, counterAccount string) event.TradeEvent {
		ev.Protocol = protocol
		ev.BaseAsset = baseAsset
		ev.CounterAsset = counterAsset
		ev.BaseAmount = baseAmt
		ev.CounterAmount = counterAmt
		ev.Price = price
		ev.CounterAccount = counterAccount
		return ev
	}

	tests := []struct {
		name    string
		tx      ingest.LedgerTransaction
		want    []event.TradeEvent
		wantErr string
	}{
		{
			name: "manage sell offer fills two orderbook offers",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil)},
				[]xdr.OperationResult{manageSellResult(
					// Seller 0x03 sold 100 USDC for 250 XLM given by the taker.
					orderBookClaim(0x03, usdc, 100_0000000, nativeAsset(), 250_0000000),
					// Seller 0x04 sold 3 USDC for 7.5 XLM.
					orderBookClaim(0x04, usdc, 3_0000000, nativeAsset(), 7_5000000),
				)},
			),
			want: []event.TradeEvent{
				withTrade(base(0, address(0x01)), "", nativeEvent, usdcEvent, "250.0000000", "100.0000000", "0.4000000", address(0x03)),
				withTrade(base(0, address(0x01)), "", nativeEvent, usdcEvent, "7.5000000", "3.0000000", "0.4000000", address(0x04)),
			},
		},
		{
			name: "operation source account overrides transaction source",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(&opSource)},
				[]xdr.OperationResult{manageSellResult(
					orderBookClaim(0x03, nativeAsset(), 1_0000000, longAsset, 3_0000000),
				)},
			),
			want: []event.TradeEvent{
				withTrade(base(0, address(0x02)), "", longEvent, nativeEvent, "3.0000000", "1.0000000", "0.3333333", address(0x03)),
			},
		},
		{
			name: "passive sell offer fills liquidity pool",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{passiveSellOp()},
				[]xdr.OperationResult{passiveSellResult(
					poolClaim(usdc, 20_0000000, nativeAsset(), 50_0000000),
				)},
			),
			want: []event.TradeEvent{
				withTrade(base(0, address(0x01)), ProtocolLiquidityPool, nativeEvent, usdcEvent, "50.0000000", "20.0000000", "0.4000000", ""),
			},
		},
		{
			name: "manage buy offer skips zero-amount claim and keeps op index",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil), manageBuyOp()},
				[]xdr.OperationResult{
					manageSellResult(),
					manageBuyResult(
						orderBookClaim(0x05, usdc, 0, nativeAsset(), 0),
						orderBookClaim(0x06, usdc, 2_0000000, nativeAsset(), 1_0000000),
					),
				},
			),
			want: []event.TradeEvent{
				withTrade(base(1, address(0x01)), "", nativeEvent, usdcEvent, "1.0000000", "2.0000000", "2.0000000", address(0x06)),
			},
		},
		{
			name: "failed transaction yields no events",
			tx: buildTx(xdr.TransactionResultCodeTxFailed,
				[]xdr.Operation{manageSellOp(nil)},
				[]xdr.OperationResult{manageSellResult(
					orderBookClaim(0x03, usdc, 1, nativeAsset(), 1),
				)},
			),
			want: nil,
		},
		{
			name: "non-trading operation yields no events",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{{Body: xdr.OperationBody{Type: xdr.OperationTypeBumpSequence, BumpSequenceOp: &xdr.BumpSequenceOp{}}}},
				[]xdr.OperationResult{{Code: xdr.OperationResultCodeOpInner, Tr: &xdr.OperationResultTr{
					Type:          xdr.OperationTypeBumpSequence,
					BumpSeqResult: &xdr.BumpSequenceResult{Code: xdr.BumpSequenceResultCodeBumpSequenceSuccess},
				}}},
			),
			want: nil,
		},
		{
			name: "malformed: result type does not match operation",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil)},
				[]xdr.OperationResult{manageBuyResult()},
			),
			wantErr: "does not match result type",
		},
		{
			name: "malformed: operation result is not opINNER",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil)},
				[]xdr.OperationResult{{Code: xdr.OperationResultCodeOpBadAuth}},
			),
			wantErr: "is not opINNER",
		},
		{
			name: "malformed: success result missing",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil)},
				[]xdr.OperationResult{{Code: xdr.OperationResultCodeOpInner, Tr: &xdr.OperationResultTr{
					Type:                  xdr.OperationTypeManageSellOffer,
					ManageSellOfferResult: &xdr.ManageSellOfferResult{Code: xdr.ManageSellOfferResultCodeManageSellOfferMalformed},
				}}},
			),
			wantErr: "in successful transaction",
		},
		{
			name: "malformed: orderbook claim atom without body",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil)},
				[]xdr.OperationResult{manageSellResult(xdr.ClaimAtom{Type: xdr.ClaimAtomTypeClaimAtomTypeOrderBook})},
			),
			wantErr: "orderbook claim atom has no body",
		},
		{
			name: "malformed: credit asset without body",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil)},
				[]xdr.OperationResult{manageSellResult(
					orderBookClaim(0x03, xdr.Asset{Type: xdr.AssetTypeAssetTypeCreditAlphanum4}, 1, nativeAsset(), 1),
				)},
			),
			wantErr: "malformed credit_alphanum4 asset",
		},
		{
			name: "malformed: operation and result counts differ",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil), manageSellOp(nil)},
				[]xdr.OperationResult{manageSellResult()},
			),
			wantErr: "2 operations but 1 results",
		},
		{
			name: "claim atom type V0 is covered",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil)},
				[]xdr.OperationResult{manageSellResult(
					claimV0(0x03, usdc, 100_0000000, nativeAsset(), 250_0000000),
				)},
			),
			want: []event.TradeEvent{
				withTrade(base(0, address(0x01)), "", nativeEvent, usdcEvent, "250.0000000", "100.0000000", "0.4000000", address(0x03)),
			},
		},
		{
			name: "muxed operation source account resolves to underlying address",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(&xdr.MuxedAccount{
					Type: xdr.CryptoKeyTypeKeyTypeMuxedEd25519,
					Med25519: &xdr.MuxedAccountMed25519{
						Id:      12345,
						Ed25519: testKey(0x02),
					},
				})},
				[]xdr.OperationResult{manageSellResult(
					orderBookClaim(0x03, nativeAsset(), 1_0000000, longAsset, 3_0000000),
				)},
			),
			want: []event.TradeEvent{
				withTrade(base(0, address(0x02)), "", longEvent, nativeEvent, "3.0000000", "1.0000000", "0.3333333", address(0x03)),
			},
		},
		{
			name: "fee bump transaction attributes events to inner source account",
			tx: ingest.LedgerTransaction{
				Hash: testTxHash,
				Envelope: xdr.TransactionEnvelope{
					Type: xdr.EnvelopeTypeEnvelopeTypeTxFeeBump,
					FeeBump: &xdr.FeeBumpTransactionEnvelope{
						Tx: xdr.FeeBumpTransaction{
							FeeSource: muxed(0x99),
							InnerTx: xdr.FeeBumpTransactionInnerTx{
								Type: xdr.EnvelopeTypeEnvelopeTypeTx,
								V1: &xdr.TransactionV1Envelope{
									Tx: xdr.Transaction{
										SourceAccount: muxed(0x01),
										Operations:    []xdr.Operation{manageSellOp(nil)},
									},
								},
							},
						},
					},
				},
				Result: xdr.TransactionResultPair{
					TransactionHash: testTxHash,
					Result: xdr.TransactionResult{
						Result: xdr.TransactionResultResult{
							Code:    xdr.TransactionResultCodeTxSuccess,
							Results: &[]xdr.OperationResult{manageSellResult(orderBookClaim(0x03, usdc, 100_0000000, nativeAsset(), 250_0000000))},
						},
					},
				},
			},
			want: []event.TradeEvent{
				withTrade(base(0, address(0x01)), "", nativeEvent, usdcEvent, "250.0000000", "100.0000000", "0.4000000", address(0x03)),
			},
		},
		{
			name: "path payment with identical assets and no claims emits no events",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{{
					Body: xdr.OperationBody{
						Type: xdr.OperationTypePathPaymentStrictReceive,
						PathPaymentStrictReceiveOp: &xdr.PathPaymentStrictReceiveOp{
							SendAsset:   nativeAsset(),
							DestAsset:   nativeAsset(),
							DestAmount:  100_0000000,
							SendMax:     100_0000000,
							Destination: muxed(0x02),
						},
					},
				}},
				[]xdr.OperationResult{{
					Code: xdr.OperationResultCodeOpInner,
					Tr: &xdr.OperationResultTr{
						Type: xdr.OperationTypePathPaymentStrictReceive,
						PathPaymentStrictReceiveResult: &xdr.PathPaymentStrictReceiveResult{
							Code:    xdr.PathPaymentStrictReceiveResultCodePathPaymentStrictReceiveSuccess,
							Success: &xdr.PathPaymentStrictReceiveResultSuccess{Offers: nil},
						},
					},
				}},
			),
			want: nil,
		},
		{
			name: "operation result index out of bounds returns error without panic",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil)},
				[]xdr.OperationResult{},
			),
			wantErr: "operations but 0 results",
		},
	}

	tr := NewTransformer()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tr.Transform(tc.tx, testLedger, testClosedAt)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil with events %+v", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertEvents(t, got, tc.want)
		})
	}
}

func assertEvents(t *testing.T, got, want []event.TradeEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d\ngot:  %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d mismatch\ngot:  %+v\nwant: %+v", i, got[i], want[i])
		}
	}
}
