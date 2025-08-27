package invoice

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/btcsuite/btclog"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/nbd-wtf/go-nostr"
)

var (
	log btclog.Logger
)

// SetLogger allows the main package to provide a shared logger.
func SetLogger(l btclog.Logger) { log = l }

// Config is the minimal configuration the invoice handler needs.
type Config struct {
	MinSendableMsat  int
	MaxSendableMsat  int
	MaxCommentLength int
	SuccessMessage   string
}

// Invoice is the JSON response for a created invoice.
type Invoice struct {
	Pr            string         `json:"pr"`
	Routes        []string       `json:"routes"`
	SuccessAction *SuccessAction `json:"successAction"`
}

// SuccessAction optionally contains a success message.
type SuccessAction struct {
	Tag     string `json:"tag"`
	Message string `json:"message,omitempty"`
}

func badRequestError(w http.ResponseWriter, reason string,
	args ...interface{}) {

	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "Error",
		"reason": fmt.Sprintf(reason, args...),
	})
}

// Manager coordinates invoice creation and related concerns.
type Manager struct {
	Cfg *ManagerConfig
}

type ManagerConfig struct {
	LndClient         lnrpc.LightningClient
	SettlementHandler *SettlementHandler
}

type Params struct {
	Msat            int64
	Description     string
	DescriptionHash []byte
}

type zapReceipt struct {
	event       nostr.Event
	description string
	relays      []string
}

func NewInvoiceManager(cfg *ManagerConfig) *Manager {
	return &Manager{
		Cfg: cfg,
	}
}

func (m *Manager) processZapRequest(zapRequest []string,
	mSat int, w http.ResponseWriter) *zapReceipt {

	e := nostr.Event{}
	err := e.UnmarshalJSON([]byte(zapRequest[0]))
	if err != nil {
		badRequestError(w, "Invalid nostr field: %s", err)
		return nil
	}
	if ok, err := e.CheckSignature(); !ok {
		badRequestError(w, "Invalid nostr signature: %s", err)
		return nil
	}
	if e.Kind != nostr.KindZapRequest {
		badRequestError(w, "Invalid event kind: %d", e.Kind)
		return nil
	}
	if len(e.Tags) == 0 {
		badRequestError(w, "No nostr tags")
		return nil
	}

	var (
		tp     []string
		tP     []string
		te     []string
		relays []string
		ta     []string
	)
	for _, t := range e.Tags {
		if len(t) > 0 {
			if t[0] == "p" {
				tp = append(tp, t[1])
			}
			if t[0] == "e" {
				te = append(te, t[1])
			}
			if t[0] == "amount" {
				amount, err := strconv.Atoi(t[1])
				if err != nil {
					badRequestError(w, "Invalid amount "+
						"tag: %s", t[1])

					return nil
				}
				if amount != mSat {
					badRequestError(w, "Incorrect "+
						"amount: %d expected %d",
						amount, mSat)

					return nil
				}
			}
			if t[0] == "relays" {
				relays = append(relays, t[1:]...)
			}
			if t[0] == "a" {
				ta = append(ta, t[1])
			}
			if t[0] == "P" {
				tP = append(tP, t[1])
			}
		}
	}
	if len(tp) != 1 {
		badRequestError(w, "Zap request should have 1 p tag")
		return nil
	}
	if len(te) > 1 {
		badRequestError(w, "Zap request should have 0 or 1 e tag")
		return nil
	}
	description, err := e.MarshalJSON()
	if err != nil {
		badRequestError(w, "Can't marshal zap request: %s", err)
		return nil
	}
	receiptTags := []nostr.Tag{{"description", string(description)}}
	receiptTags = append(receiptTags, nostr.Tag{"p", tp[0]})
	if len(te) > 0 {
		receiptTags = append(receiptTags, nostr.Tag{"e", te[0]})
	}
	if len(ta) > 0 {
		receiptTags = append(receiptTags, nostr.Tag{"a", ta[0]})
	}
	if len(tP) > 0 {
		receiptTags = append(receiptTags, nostr.Tag{"P", tP[0]})
	}

	return &zapReceipt{
		event: nostr.Event{
			Kind: nostr.KindZap,
			Tags: receiptTags,
		},
		relays:      relays,
		description: string(description),
	}
}

func (m *Manager) HandleInvoiceCreation(
	config Config, baseMetadata string) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

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

		if mSat < config.MinSendableMsat ||
			mSat > config.MaxSendableMsat {

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

		metadata := baseMetadata
		zapRequest, hasNostr := r.URL.Query()["nostr"]
		var zapReceipt *zapReceipt
		if hasNostr && len(zapRequest) > 0 {
			zapReceipt = m.processZapRequest(zapRequest, mSat, w)
			if zapReceipt == nil {
				return
			}

			metadata = zapReceipt.description
		}

		// parameters ok, creating invoice
		invoiceParams := Params{
			Msat:        int64(mSat),
			Description: metadata,
		}

		h := sha256.Sum256([]byte(invoiceParams.Description))
		invoiceParams.DescriptionHash = h[:]

		bolt11, r_hash, err := m.MakeInvoice(invoiceParams)
		if err != nil {
			log.Infof("Cannot create invoice: %s", err)
			badRequestError(w, "Invoice creation failed.")
			return
		}

		if zapReceipt != nil {
			zapReceipt.event.Tags = append(
				zapReceipt.event.Tags,
				nostr.Tag{"bolt11", bolt11},
			)
		}

		invoice := Invoice{
			Pr:     bolt11,
			Routes: make([]string, 0),
			SuccessAction: &SuccessAction{
				Tag:     "message",
				Message: config.SuccessMessage,
			},
		}
		m.Cfg.SettlementHandler.subscribeInvoiceSettlements(
			context.Background(), r_hash, comment, zapReceipt,
		)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(invoice)
	}
}

func (m *Manager) MakeInvoice(params Params) (string, []byte,
	error) {

	invoice := &lnrpc.Invoice{
		ValueMsat:       params.Msat,
		Memo:            params.Description,
		DescriptionHash: params.DescriptionHash,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := m.Cfg.LndClient.AddInvoice(
		ctx, invoice,
	)
	if err != nil {
		return "", nil, err
	}

	return resp.PaymentRequest, resp.RHash, nil
}
