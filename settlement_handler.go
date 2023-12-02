package main

import (
	"bytes"
	"context"
	"github.com/lightningnetwork/lnd/lnrpc"
)

type SettlementHandler struct {
	lndClient lnrpc.LightningClient
}

func NewSettlementHandler(
	lndClient lnrpc.LightningClient) *SettlementHandler {

	return &SettlementHandler{
		lndClient: lndClient,
	}
}

/*func (sh *SettlementHandler) subscribeToInvoice(r_hash string, comment string) error {
	resp, err := sh.get("/v2/invoices/subscribe/" + strings.NewReplacer("+", "-", "/", "_").Replace(r_hash))
	if err != nil {
		log.Infof("Error subscribing to invoice: %s", err)
		return err
	}
	go func() {
		defer resp.Body.Close()
		dec := json.NewDecoder(resp.Body)
		for dec.More() {
			var invoice struct {
				Result struct {
					AmtPaidSat string `json:"amt_paid_sat"`
					Settled    bool
				}
			}
			dec.Decode(&invoice)
			log.Printf("New invoice info: %+v", invoice)
			if invoice.Result.Settled {
				log.Printf("Invoice %s settled", r_hash)
				amt, err := strconv.ParseUint(invoice.Result.AmtPaidSat, 10, 64)
				if err != nil {
					log.Printf("Invalid amount %s.", invoice.Result.AmtPaidSat)
				} else {
					broadcastNotification(amt, comment)
				}
			}
		}
	}()
	return nil
}*/

func (s *SettlementHandler) subscribeToInvoiceRpc(ctx context.Context,
	rHash []byte, comment string) error {

	stream, err := s.lndClient.SubscribeInvoices(
		ctx, &lnrpc.InvoiceSubscription{},
	)
	if err != nil {
		return err
	}

	go func() {
		for {
			invoice, err := stream.Recv()
			if err != nil {
				log.Warnf("invoice stream error")
			}

			if invoice.State != lnrpc.Invoice_SETTLED {
				continue
			}

			if bytes.Equal(invoice.RHash, rHash) {
				broadcastNotification(
					uint64(invoice.AmtPaidSat), comment,
				)
			}
		}
	}()

	return nil
}
