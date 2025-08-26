package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/hieblmi/go-host-lnaddr/notifier"
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
				log.Warnf("Error connecting to relay %s",
					relayAddr)

				return
			}
			err = relay.Publish(zapctx, zapReceipt.event)
			if err != nil {
				log.Warnf("Error publishing zap receipt to "+
					"relay %s: %s", relayAddr, err)
			} else {
				log.Infof("Published zap receipt to "+
					"relay %s", relayAddr)
			}
			err = relay.Close()
			if err != nil {
				log.Warnf("Error closing relay %s: %s",
					relayAddr, err)
			}
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
				notifier.BroadcastNotification(
					uint64(invoice.AmtPaidSat), comment,
				)
				if zapReceipt != nil && s.nsec != "" {
					zapReceipt.event.CreatedAt = nostr.Timestamp(invoice.SettleDate)
					zapReceipt.event.Tags = append(
						zapReceipt.event.Tags,
						nostr.Tag{"preimage",
							hex.EncodeToString(invoice.RPreimage)})
					err = zapReceipt.event.Sign(s.nsec)
					if err != nil {
						log.Warnf("Error signing zap "+
							"receipt: %s", err)
					} else {
						log.Infof("Publishing zap "+
							"receipt: %+v",
							zapReceipt.event)
						go publishZapReceipt(zapReceipt)
					}
				}
			}
		}
	}()

	return nil
}
