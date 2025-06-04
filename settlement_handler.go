package main

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/nbd-wtf/go-nostr"
)

type SettlementHandler struct {
	lndClient lnrpc.LightningClient
	nsec      string
}

func NewSettlementHandler(
	lndClient lnrpc.LightningClient, nsec string) *SettlementHandler {

	return &SettlementHandler{
		lndClient: lndClient,
		nsec:      nsec,
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

func publishZapReceipt(zapReceipt *zapReceipt) {
	zapctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	wg := sync.WaitGroup{}
	for _, relayAddr := range zapReceipt.relays {
		wg.Add(1)
		go func(relayAddr string) {
			defer wg.Done()
			relay, err := nostr.RelayConnect(zapctx, relayAddr)
			if err != nil {
				log.Warnf("Error connecting to relay %s", relayAddr)
				return
			}
			err = relay.Publish(zapctx, zapReceipt.event)
			if err != nil {
				log.Warnf("Error publishing zap receipt to relay %s: %s", relayAddr, err)
			} else {
				log.Infof("Published zap receipt to relay %s", relayAddr)
			}
			relay.Close()
		}(relayAddr)
	}
	wg.Wait()
}

func (s *SettlementHandler) subscribeToInvoiceRpc(ctx context.Context,
	rHash []byte, comment string, zapReceipt *zapReceipt) error {

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
				if zapReceipt != nil && s.nsec != "" {
					zapReceipt.event.CreatedAt = nostr.Timestamp(invoice.SettleDate)
					zapReceipt.event.Sign(s.nsec)
					go publishZapReceipt(zapReceipt)
				}
			}
		}
	}()

	return nil
}
