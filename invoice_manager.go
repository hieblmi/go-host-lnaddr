package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/lightningnetwork/lnd/lnrpc"
	"net/http"
	"strconv"
)

var TorProxyURL = "socks5://127.0.0.1:9050"

type InvoiceManager struct {
	Cfg *InvoiceManagerConfig
}

type InvoiceManagerConfig struct {
	LndClient         lnrpc.LightningClient
	SettlementHandler *SettlementHandler
}

type InvoiceParams struct {
	Msat            int64
	Description     string
	DescriptionHash []byte
}

func NewInvoiceManager(cfg *InvoiceManagerConfig) *InvoiceManager {
	return &InvoiceManager{
		Cfg: cfg,
	}
}

func (m *InvoiceManager) handleInvoiceCreation(config ServerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("Handling invoice creation: %v\n", *r)
		w.Header().Set("Content-Type", "application/json")

		keys, hasAmount := r.URL.Query()["amount"]
		if !hasAmount || len(keys[0]) < 1 {
			badRequestError(
				w, "Mandatory URL Query parameter 'amount' "+
					"is missing.")

			return
		}

		mSat, isInt := strconv.Atoi(keys[0])
		if isInt != nil {
			badRequestError(w, "Amount needs to be a number "+
				"denoting the number of msat.")
			return
		}

		if mSat < config.MinSendableMsat || mSat > config.MaxSendableMsat {
			badRequestError(w, "Wrong amount. Amount needs to "+
				"be in between [%d,%d] msat",
				config.MinSendableMsat, config.MaxSendableMsat)

			return
		}

		comment := r.URL.Query().Get("comment")
		if len(comment) > config.MaxCommentLength {
			badRequestError(w, "Comment is too long, should be "+
				"no longer than %d bytes",
				config.MaxCommentLength)

			return
		}

		metadata, err := metadataToString(config)
		if err != nil {
			log.Warnf("Unable to convert metadata to string: %v\n",
				err)
		}

		// parameters ok, creating invoice
		invoiceParams := InvoiceParams{
			Msat:        int64(mSat),
			Description: metadata,
		}

		h := sha256.Sum256([]byte(invoiceParams.Description))
		invoiceParams.DescriptionHash = h[:]

		bolt11, r_hash, err := m.MakeInvoice(invoiceParams)
		if err != nil {
			log.Infof("Cannot create invoice: %s\n", err)
			badRequestError(w, "Invoice creation failed.")
			return
		}

		invoice := Invoice{
			Pr:     bolt11,
			Routes: make([]string, 0),
			SuccessAction: &SuccessAction{
				Tag:     "message",
				Message: config.SuccessMessage,
			},
		}
		m.Cfg.SettlementHandler.subscribeToInvoiceRpc(
			context.Background(), r_hash, comment,
		)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(invoice)
	}
}

func (m *InvoiceManager) MakeInvoice(params InvoiceParams) (string, []byte,
	error) {

	invoice := &lnrpc.Invoice{
		ValueMsat:       params.Msat,
		Memo:            params.Description,
		DescriptionHash: params.DescriptionHash,
	}

	resp, err := m.Cfg.LndClient.AddInvoice(
		context.Background(), invoice,
	)
	if err != nil {
		return "", nil, err
	}

	return resp.PaymentRequest, resp.RHash, nil
}
