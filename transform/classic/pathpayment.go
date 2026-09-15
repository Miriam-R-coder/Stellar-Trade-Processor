package classic

import (
	"fmt"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// pathPaymentClaims returns the claim atoms crossed while routing a
// successful PathPaymentStrictSend or PathPaymentStrictReceive operation.
// Each claim atom is one hop of the path; a direct payment between identical
// send and destination assets crosses no offers and yields no claims.
func pathPaymentClaims(tr xdr.OperationResultTr) ([]xdr.ClaimAtom, error) {
	switch tr.Type {
	case xdr.OperationTypePathPaymentStrictSend:
		res, ok := tr.GetPathPaymentStrictSendResult()
		if !ok {
			return nil, fmt.Errorf("path payment strict send result is missing")
		}
		success, ok := res.GetSuccess()
		if !ok {
			return nil, fmt.Errorf("path payment strict send result code %d in successful transaction", res.Code)
		}
		return success.Offers, nil

	case xdr.OperationTypePathPaymentStrictReceive:
		res, ok := tr.GetPathPaymentStrictReceiveResult()
		if !ok {
			return nil, fmt.Errorf("path payment strict receive result is missing")
		}
		success, ok := res.GetSuccess()
		if !ok {
			return nil, fmt.Errorf("path payment strict receive result code %d in successful transaction", res.Code)
		}
		return success.Offers, nil

	default:
		return nil, fmt.Errorf("operation result type %s is not a path payment", tr.Type)
	}
}
