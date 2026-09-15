package classic

import (
	"strings"
	"testing"

	"github.com/fadesany/Stellar-Trade-Processor/event"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func strictSendOp(source *xdr.MuxedAccount) xdr.Operation {
	return xdr.Operation{
		SourceAccount: source,
		Body:          xdr.OperationBody{Type: xdr.OperationTypePathPaymentStrictSend, PathPaymentStrictSendOp: &xdr.PathPaymentStrictSendOp{}},
	}
}

func strictReceiveOp() xdr.Operation {
	return xdr.Operation{
		Body: xdr.OperationBody{Type: xdr.OperationTypePathPaymentStrictReceive, PathPaymentStrictReceiveOp: &xdr.PathPaymentStrictReceiveOp{}},
	}
}

func strictSendResult(claims ...xdr.ClaimAtom) xdr.OperationResult {
	return xdr.OperationResult{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypePathPaymentStrictSend,
			PathPaymentStrictSendResult: &xdr.PathPaymentStrictSendResult{
				Code:    xdr.PathPaymentStrictSendResultCodePathPaymentStrictSendSuccess,
				Success: &xdr.PathPaymentStrictSendResultSuccess{Offers: claims},
			},
		},
	}
}

func strictReceiveResult(claims ...xdr.ClaimAtom) xdr.OperationResult {
	return xdr.OperationResult{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypePathPaymentStrictReceive,
			PathPaymentStrictReceiveResult: &xdr.PathPaymentStrictReceiveResult{
				Code:    xdr.PathPaymentStrictReceiveResultCodePathPaymentStrictReceiveSuccess,
				Success: &xdr.PathPaymentStrictReceiveResultSuccess{Offers: claims},
			},
		},
	}
}

func TestTransformPathPayments(t *testing.T) {
	usdc := creditAsset("USDC", 0x09)
	eurt := creditAsset("EURT", 0x07)
	sender := muxed(0x02)

	usdcEvent := event.Asset{Type: event.AssetTypeCreditAlphanum4, Code: "USDC", Issuer: address(0x09)}
	eurtEvent := event.Asset{Type: event.AssetTypeCreditAlphanum4, Code: "EURT", Issuer: address(0x07)}
	nativeEvent := event.Asset{Type: event.AssetTypeNative}

	trade := func(venue event.Venue, opIndex int, account, protocol string, baseAsset, counterAsset event.Asset, baseAmt, counterAmt, price, counterAccount string) event.TradeEvent {
		return event.TradeEvent{
			Venue:          venue,
			Protocol:       protocol,
			BaseAsset:      baseAsset,
			CounterAsset:   counterAsset,
			BaseAmount:     baseAmt,
			CounterAmount:  counterAmt,
			Price:          price,
			Account:        account,
			CounterAccount: counterAccount,
			LedgerSequence: testLedger,
			TxHash:         testTxHash.HexString(),
			OpIndex:        opIndex,
			ClosedAt:       testClosedAt,
		}
	}

	tests := []struct {
		name    string
		tx      ingest.LedgerTransaction
		want    []event.TradeEvent
		wantErr string
	}{
		{
			name: "strict send multi-hop USDC -> XLM -> EURT emits one event per hop",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{strictSendOp(&sender)},
				[]xdr.OperationResult{strictSendResult(
					// Hop 1: offer owner 0x03 sells 40 XLM for the sender's 10 USDC.
					orderBookClaim(0x03, nativeAsset(), 40_0000000, usdc, 10_0000000),
					// Hop 2: liquidity pool sells 9 EURT for the 40 XLM.
					poolClaim(eurt, 9_0000000, nativeAsset(), 40_0000000),
				)},
			),
			want: []event.TradeEvent{
				trade(event.VenuePathPayment, 0, address(0x02), "", usdcEvent, nativeEvent, "10.0000000", "40.0000000", "4.0000000", address(0x03)),
				trade(event.VenuePathPayment, 0, address(0x02), ProtocolLiquidityPool, nativeEvent, eurtEvent, "40.0000000", "9.0000000", "0.2250000", ""),
			},
		},
		{
			name: "strict receive single hop uses transaction source",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{strictReceiveOp()},
				[]xdr.OperationResult{strictReceiveResult(
					orderBookClaim(0x04, usdc, 5_0000000, eurt, 4_6000000),
				)},
			),
			want: []event.TradeEvent{
				trade(event.VenuePathPayment, 0, address(0x01), "", eurtEvent, usdcEvent, "4.6000000", "5.0000000", "1.0869565", address(0x04)),
			},
		},
		{
			name: "offer and path payment in one transaction keep distinct venues",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{manageSellOp(nil), strictSendOp(nil)},
				[]xdr.OperationResult{
					manageSellResult(orderBookClaim(0x05, usdc, 1_0000000, nativeAsset(), 2_0000000)),
					strictSendResult(orderBookClaim(0x06, eurt, 3_0000000, usdc, 3_0000000)),
				},
			),
			want: []event.TradeEvent{
				trade(event.VenueClassicDEX, 0, address(0x01), "", nativeEvent, usdcEvent, "2.0000000", "1.0000000", "0.5000000", address(0x05)),
				trade(event.VenuePathPayment, 1, address(0x01), "", usdcEvent, eurtEvent, "3.0000000", "3.0000000", "1.0000000", address(0x06)),
			},
		},
		{
			name: "path payment without crossed offers yields no events",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{strictSendOp(nil)},
				[]xdr.OperationResult{strictSendResult()},
			),
			want: nil,
		},
		{
			name: "malformed: strict send success body missing",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{strictSendOp(nil)},
				[]xdr.OperationResult{{Code: xdr.OperationResultCodeOpInner, Tr: &xdr.OperationResultTr{
					Type: xdr.OperationTypePathPaymentStrictSend,
					PathPaymentStrictSendResult: &xdr.PathPaymentStrictSendResult{
						Code: xdr.PathPaymentStrictSendResultCodePathPaymentStrictSendUnderfunded,
					},
				}}},
			),
			wantErr: "path payment strict send result code -2 in successful transaction",
		},
		{
			name: "malformed: strict receive result arm missing",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{strictReceiveOp()},
				[]xdr.OperationResult{{Code: xdr.OperationResultCodeOpInner, Tr: &xdr.OperationResultTr{
					Type: xdr.OperationTypePathPaymentStrictReceive,
				}}},
			),
			wantErr: "path payment strict receive result is missing",
		},
		{
			name: "malformed: unknown claim atom type in path",
			tx: buildTx(xdr.TransactionResultCodeTxSuccess,
				[]xdr.Operation{strictSendOp(nil)},
				[]xdr.OperationResult{strictSendResult(xdr.ClaimAtom{Type: xdr.ClaimAtomType(99)})},
			),
			wantErr: "unknown claim atom type 99",
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
