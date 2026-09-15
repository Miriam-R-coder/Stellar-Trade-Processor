package classic

import (
	"fmt"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// pathPaymentClaims returns the claim atoms crossed while routing a
// successful PathPaymentStrictSend or PathPaymentStrictReceive operation.
// Each claim atom is one hop of the path; a direct payment between identical
// send and destination assets crosses no offers and yields no claims.
//
// Union arms are checked for nil directly: the generated Get* accessors
// dereference the arm pointer and would panic on malformed input.
func pathPaymentClaims(tr xdr.OperationResultTr) ([]xdr.ClaimAtom, error) {
	switch tr.Type {
	case xdr.OperationTypePathPaymentStrictSend:
		res := tr.PathPaymentStrictSendResult
		if res == nil {
			return nil, fmt.Errorf("path payment strict send result is missing")
		}
		if res.Code != xdr.PathPaymentStrictSendResultCodePathPaymentStrictSendSuccess || res.Success == nil {
			return nil, fmt.Errorf("path payment strict send result code %d in successful transaction", res.Code)
		}
		return res.Success.Offers, nil

	case xdr.OperationTypePathPaymentStrictReceive:
		res := tr.PathPaymentStrictReceiveResult
		if res == nil {
			return nil, fmt.Errorf("path payment strict receive result is missing")
		}
		if res.Code != xdr.PathPaymentStrictReceiveResultCodePathPaymentStrictReceiveSuccess || res.Success == nil {
			return nil, fmt.Errorf("path payment strict receive result code %d in successful transaction", res.Code)
		}
		return res.Success.Offers, nil

	default:
		return nil, fmt.Errorf("operation result type %s is not a path payment", tr.Type)
	}
}
