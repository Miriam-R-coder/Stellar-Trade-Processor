package soroban

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
	testTxHash   = xdr.Hash{0x5e, 0xed}
)

const (
	routerByte = 0xaa
	otherByte  = 0xbb
	tokenA     = 0x0a
	tokenB     = 0x0b
	tokenC     = 0x0c
)

func fill32(b byte) [32]byte {
	var out [32]byte
	for i := range out {
		out[i] = b
	}
	return out
}

func contractAddress(b byte) string {
	id := fill32(b)
	return strkey.MustEncode(strkey.VersionByteContract, id[:])
}

func accountAddress(b byte) string {
	key := fill32(b)
	return strkey.MustEncode(strkey.VersionByteAccountID, key[:])
}

func scSymbol(s string) xdr.ScVal {
	sym := xdr.ScSymbol(s)
	return xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &sym}
}

func scString(s string) xdr.ScVal {
	str := xdr.ScString(s)
	return xdr.ScVal{Type: xdr.ScValTypeScvString, Str: &str}
}

func scContract(b byte) xdr.ScVal {
	id := xdr.ContractId(fill32(b))
	return xdr.ScVal{Type: xdr.ScValTypeScvAddress, Address: &xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: &id,
	}}
}

func scAccount(b byte) xdr.ScVal {
	key := xdr.Uint256(fill32(b))
	return xdr.ScVal{Type: xdr.ScValTypeScvAddress, Address: &xdr.ScAddress{
		Type:      xdr.ScAddressTypeScAddressTypeAccount,
		AccountId: &xdr.AccountId{Type: xdr.PublicKeyTypePublicKeyTypeEd25519, Ed25519: &key},
	}}
}

func scI128(v int64) xdr.ScVal {
	hi := xdr.Int64(0)
	if v < 0 {
		hi = -1
	}
	return xdr.ScVal{Type: xdr.ScValTypeScvI128, I128: &xdr.Int128Parts{Hi: hi, Lo: xdr.Uint64(uint64(v))}}
}

func scVec(vals ...xdr.ScVal) xdr.ScVal {
	vec := xdr.ScVec(vals)
	ptr := &vec
	return xdr.ScVal{Type: xdr.ScValTypeScvVec, Vec: &ptr}
}

func scMap(entries ...xdr.ScMapEntry) xdr.ScVal {
	m := xdr.ScMap(entries)
	ptr := &m
	return xdr.ScVal{Type: xdr.ScValTypeScvMap, Map: &ptr}
}

func entry(key string, val xdr.ScVal) xdr.ScMapEntry {
	return xdr.ScMapEntry{Key: scSymbol(key), Val: val}
}

// swapData builds the SwapEvent map with keys in the sorted order the Soroban
// host uses (amounts, path, to).
func swapData(path []xdr.ScVal, amounts []xdr.ScVal, to xdr.ScVal) xdr.ScVal {
	return scMap(
		entry("amounts", scVec(amounts...)),
		entry("path", scVec(path...)),
		entry("to", to),
	)
}

func routerSwapTopics() []xdr.ScVal {
	return []xdr.ScVal{scString("SoroswapRouter"), scSymbol("swap")}
}

func contractEvent(emitter byte, topics []xdr.ScVal, data xdr.ScVal) xdr.ContractEvent {
	id := xdr.ContractId(fill32(emitter))
	return xdr.ContractEvent{
		ContractId: &id,
		Type:       xdr.ContractEventTypeContract,
		Body: xdr.ContractEventBody{
			V:  0,
			V0: &xdr.ContractEventV0{Topics: topics, Data: data},
		},
	}
}

func metaV4(events ...xdr.ContractEvent) xdr.TransactionMeta {
	return xdr.TransactionMeta{V: 4, V4: &xdr.TransactionMetaV4{
		Operations: []xdr.OperationMetaV2{{Events: events}},
	}}
}

func metaV3(events ...xdr.ContractEvent) xdr.TransactionMeta {
	return xdr.TransactionMeta{V: 3, V3: &xdr.TransactionMetaV3{
		SorobanMeta: &xdr.SorobanTransactionMeta{Events: events},
	}}
}

func invokeTx(code xdr.TransactionResultCode, opType xdr.OperationType, meta xdr.TransactionMeta) ingest.LedgerTransaction {
	source := xdr.Uint256(fill32(0x01))
	op := xdr.Operation{Body: xdr.OperationBody{Type: opType}}
	if opType == xdr.OperationTypeInvokeHostFunction {
		op.Body.InvokeHostFunctionOp = &xdr.InvokeHostFunctionOp{}
	} else {
		op.Body.BumpSequenceOp = &xdr.BumpSequenceOp{}
	}
	results := []xdr.OperationResult{}
	return ingest.LedgerTransaction{
		Hash: testTxHash,
		Envelope: xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTx,
			V1: &xdr.TransactionV1Envelope{Tx: xdr.Transaction{
				SourceAccount: xdr.MuxedAccount{Type: xdr.CryptoKeyTypeKeyTypeEd25519, Ed25519: &source},
				Operations:    []xdr.Operation{op},
			}},
		},
		Result: xdr.TransactionResultPair{
			TransactionHash: testTxHash,
			Result: xdr.TransactionResult{
				Result: xdr.TransactionResultResult{Code: code, Results: &results},
			},
		},
		UnsafeMeta: meta,
	}
}

func hop(base, counter byte, baseAmount, counterAmount, price string, to string) event.TradeEvent {
	return event.TradeEvent{
		Venue:          event.VenueSorobanAMM,
		Protocol:       ProtocolSoroswap,
		BaseAsset:      event.Asset{Type: event.AssetTypeSorobanToken, ContractAddress: contractAddress(base)},
		CounterAsset:   event.Asset{Type: event.AssetTypeSorobanToken, ContractAddress: contractAddress(counter)},
		BaseAmount:     baseAmount,
		CounterAmount:  counterAmount,
		Price:          price,
		Account:        to,
		LedgerSequence: testLedger,
		TxHash:         testTxHash.HexString(),
		OpIndex:        0,
		ClosedAt:       testClosedAt,
	}
}

func TestTransformSoroswap(t *testing.T) {
	singleHop := contractEvent(routerByte, routerSwapTopics(), swapData(
		[]xdr.ScVal{scContract(tokenA), scContract(tokenB)},
		[]xdr.ScVal{scI128(10_000_000), scI128(25_000_000)},
		scAccount(0x02),
	))

	tests := []struct {
		name    string
		tx      ingest.LedgerTransaction
		want    []event.TradeEvent
		wantErr string
	}{
		{
			name: "single hop router swap",
			tx:   invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(singleHop)),
			want: []event.TradeEvent{
				hop(tokenA, tokenB, "10000000", "25000000", "2.5000000", accountAddress(0x02)),
			},
		},
		{
			name: "multi-hop path emits one event per hop",
			tx: invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(
				contractEvent(routerByte, routerSwapTopics(), swapData(
					[]xdr.ScVal{scContract(tokenA), scContract(tokenB), scContract(tokenC)},
					[]xdr.ScVal{scI128(100), scI128(250), scI128(50)},
					scContract(0x03),
				)),
			)),
			want: []event.TradeEvent{
				hop(tokenA, tokenB, "100", "250", "2.5000000", contractAddress(0x03)),
				hop(tokenB, tokenC, "250", "50", "0.2000000", contractAddress(0x03)),
			},
		},
		{
			name: "transaction meta v3 events",
			tx:   invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV3(singleHop)),
			want: []event.TradeEvent{
				hop(tokenA, tokenB, "10000000", "25000000", "2.5000000", accountAddress(0x02)),
			},
		},
		{
			name: "symbol router topic accepted",
			tx: invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(
				contractEvent(routerByte, []xdr.ScVal{scSymbol("SoroswapRouter"), scSymbol("swap")}, swapData(
					[]xdr.ScVal{scContract(tokenB), scContract(tokenA)},
					[]xdr.ScVal{scI128(3), scI128(1)},
					scAccount(0x02),
				)),
			)),
			want: []event.TradeEvent{
				hop(tokenB, tokenA, "3", "1", "0.3333333", accountAddress(0x02)),
			},
		},
		{
			name: "non-swap router events and pair token transfers are ignored",
			tx: invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(
				contractEvent(tokenA, []xdr.ScVal{scSymbol("transfer"), scAccount(0x02), scContract(otherByte)}, scI128(10_000_000)),
				contractEvent(routerByte, []xdr.ScVal{scString("SoroswapRouter"), scSymbol("add")}, scMap(entry("liquidity", scI128(5)))),
				singleHop,
			)),
			want: []event.TradeEvent{
				hop(tokenA, tokenB, "10000000", "25000000", "2.5000000", accountAddress(0x02)),
			},
		},
		{
			name: "swap-shaped event from unregistered contract is ignored",
			tx: invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(
				contractEvent(otherByte, routerSwapTopics(), swapData(
					[]xdr.ScVal{scContract(tokenA), scContract(tokenB)},
					[]xdr.ScVal{scI128(1), scI128(2)},
					scAccount(0x02),
				)),
			)),
			want: nil,
		},
		{
			name: "failed transaction yields no events",
			tx:   invokeTx(xdr.TransactionResultCodeTxFailed, xdr.OperationTypeInvokeHostFunction, metaV4(singleHop)),
			want: nil,
		},
		{
			name: "classic operation is skipped",
			tx:   invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeBumpSequence, metaV4(singleHop)),
			want: nil,
		},
		{
			name: "malformed: amounts length differs from path",
			tx: invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(
				contractEvent(routerByte, routerSwapTopics(), swapData(
					[]xdr.ScVal{scContract(tokenA), scContract(tokenB), scContract(tokenC)},
					[]xdr.ScVal{scI128(100), scI128(250)},
					scAccount(0x02),
				)),
			)),
			wantErr: "path has 3 tokens but amounts has 2 entries",
		},
		{
			name: "malformed: data is not a map",
			tx: invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(
				contractEvent(routerByte, routerSwapTopics(), scVec(scContract(tokenA), scContract(tokenB))),
			)),
			wantErr: "want map",
		},
		{
			name: "malformed: missing to field",
			tx: invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(
				contractEvent(routerByte, routerSwapTopics(), scMap(
					entry("amounts", scVec(scI128(1), scI128(2))),
					entry("path", scVec(scContract(tokenA), scContract(tokenB))),
				)),
			)),
			wantErr: "missing field to",
		},
		{
			name: "malformed: account address in token path",
			tx: invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(
				contractEvent(routerByte, routerSwapTopics(), swapData(
					[]xdr.ScVal{scContract(tokenA), scAccount(0x04)},
					[]xdr.ScVal{scI128(1), scI128(2)},
					scAccount(0x02),
				)),
			)),
			wantErr: "path[1]: account address where token contract expected",
		},
		{
			name: "malformed: non-positive amount",
			tx: invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(
				contractEvent(routerByte, routerSwapTopics(), swapData(
					[]xdr.ScVal{scContract(tokenA), scContract(tokenB)},
					[]xdr.ScVal{scI128(5), scI128(-1)},
					scAccount(0x02),
				)),
			)),
			wantErr: "amounts[1]: amount -1 is not positive",
		},
		{
			name: "malformed: amount is not i128",
			tx: invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, metaV4(
				contractEvent(routerByte, routerSwapTopics(), swapData(
					[]xdr.ScVal{scContract(tokenA), scContract(tokenB)},
					[]xdr.ScVal{scI128(5), scSymbol("seven")},
					scAccount(0x02),
				)),
			)),
			wantErr: "amounts[1]: value is ScValTypeScvSymbol, want i128",
		},
		{
			name:    "malformed: transaction meta v4 without body",
			tx:      invokeTx(xdr.TransactionResultCodeTxSuccess, xdr.OperationTypeInvokeHostFunction, xdr.TransactionMeta{V: 4}),
			wantErr: "transaction meta v4 has no body",
		},
	}

	registry := NewRegistry()
	registry.Register(NewSoroswapAdapter(contractAddress(routerByte)))
	tr := NewTransformer(registry)

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
			if len(got) != len(tc.want) {
				t.Fatalf("got %d events, want %d\ngot:  %+v\nwant: %+v", len(got), len(tc.want), got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("event %d mismatch\ngot:  %+v\nwant: %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	router := contractAddress(routerByte)
	tests := []struct {
		name      string
		register  []ProtocolAdapter
		lookup    string
		wantFound bool
	}{
		{name: "empty registry", lookup: router, wantFound: false},
		{name: "registered router matches", register: []ProtocolAdapter{NewSoroswapAdapter(router)}, lookup: router, wantFound: true},
		{name: "unregistered contract misses", register: []ProtocolAdapter{NewSoroswapAdapter(router)}, lookup: contractAddress(otherByte), wantFound: false},
		{name: "default adapter matches mainnet router", register: []ProtocolAdapter{NewSoroswapAdapter()}, lookup: SoroswapMainnetRouter, wantFound: true},
		{name: "nil adapter ignored", register: []ProtocolAdapter{nil}, lookup: router, wantFound: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			for _, a := range tc.register {
				r.Register(a)
			}
			a, ok := r.AdapterFor(tc.lookup)
			if ok != tc.wantFound {
				t.Fatalf("AdapterFor found = %v, want %v", ok, tc.wantFound)
			}
			if ok && a.Name() != ProtocolSoroswap {
				t.Fatalf("adapter name = %q, want %q", a.Name(), ProtocolSoroswap)
			}
		})
	}
}
