package invoice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/nbd-wtf/go-nostr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// mockLightningClient is a minimal test double for lnrpc.LightningClient.
// It records AddInvoice calls and returns deterministic values.
type mockLightningClient struct {
	lnrpc.LightningClient

	lastInvoice *lnrpc.Invoice
}

func (f *mockLightningClient) AddInvoice(_ context.Context, in *lnrpc.Invoice,
    _ ...grpc.CallOption) (*lnrpc.AddInvoiceResponse, error) {

	f.lastInvoice = in
	return &lnrpc.AddInvoiceResponse{
		PaymentRequest: "lnbc1testpr",
		RHash:          []byte{1, 2, 3, 4},
	}, nil
}

// Implement SubscribeInvoices with a no-op stream for this test. We don't need
// to exercise settlement publishing to validate request processing per spec.
func (f *mockLightningClient) SubscribeInvoices(_ context.Context,
    _ *lnrpc.InvoiceSubscription, _ ...grpc.CallOption) (
    lnrpc.Lightning_SubscribeInvoicesClient, error) {

	return &noopInvoiceStream{}, nil
}

type noopInvoiceStream struct{}

func (n *noopInvoiceStream) Recv() (*lnrpc.Invoice, error) { // blocks forever
	time.Sleep(time.Hour)
	return nil, context.Canceled
}

func (n *noopInvoiceStream) Header() (metadata.MD, error) { return metadata.MD{}, nil }
func (n *noopInvoiceStream) Trailer() metadata.MD         { return metadata.MD{} }
func (n *noopInvoiceStream) CloseSend() error             { return nil }
func (n *noopInvoiceStream) Context() context.Context     { return context.Background() }
func (n *noopInvoiceStream) SendMsg(m interface{}) error  { return nil }
func (n *noopInvoiceStream) RecvMsg(m interface{}) error  { return nil }

// helper to build a valid zap request event JSON string for amount and
// recipient pk.
func makeZapRequest(t *testing.T, amountMsat int, recipientPub string) string {
	t.Helper()

	// Generate a sender key
	sk := nostr.GeneratePrivateKey()
	pk, err := nostr.GetPublicKey(sk)
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}

	e := nostr.Event{
		PubKey:    pk,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindZapRequest,
		Tags: nostr.Tags{
			{"relays", "wss://relay.example1", "wss://relay.example2"},
			{"amount", strconv.Itoa(amountMsat)},
			{"p", recipientPub},
		},
		Content: "zap test",
	}
	if err := e.Sign(sk); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	b, err := e.MarshalJSON()
	if err != nil {
		t.Fatalf("Marshal zap: %v", err)
	}
	return string(b)
}

func TestInvoiceCreationWithZapRequest_FollowsSpecBasics(t *testing.T) {
	// Setup: fake LND client and manager
	fl := &mockLightningClient{}
	sh := NewSettlementHandler(fl, "") // empty nsec to skip signing/publish path
	mgr := NewInvoiceManager(&ManagerConfig{LndClient: fl, SettlementHandler: sh})

	// HTTP server with only the invoice handler
	mux := http.NewServeMux()
	cfg := Config{
		MinSendableMsat:  1000,
		MaxSendableMsat:  2_000_000,
		MaxCommentLength: 150,
		SuccessMessage:   "ok",
	}
	mux.HandleFunc("/invoice/", mgr.HandleInvoiceCreation(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Prepare a valid zap request for amount.
	amount := 21000
	// recipient pubkey is what clients expect from /.well-known/lnurlp as
	// nostrPubkey, but here we just need any hex key.
	recSk := nostr.GeneratePrivateKey()
	recPk, _ := nostr.GetPublicKey(recSk)
	zapJSON := makeZapRequest(t, amount, recPk)

	// Call /invoice with matching amount and nostr event
	u, _ := url.Parse(ts.URL + "/invoice/")
	q := u.Query()
	q.Set("amount", strconv.Itoa(amount))
	q.Set("nostr", zapJSON) // handler expects raw JSON (already string); http lib will encode
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		t.Fatalf("GET invoice: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	var inv Invoice
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		t.Fatalf("decode invoice: %v", err)
	}
	if inv.Pr == "" {
		t.Fatalf("expected pr to be set")
	}

	// Assert LND AddInvoice was called with description exactly the zap
	// JSON.
	if fl.lastInvoice == nil {
		t.Fatalf("AddInvoice was not called")
	}
	if fl.lastInvoice.Memo != zapJSON {
		t.Fatalf("expected description to be zap JSON; got %q",
			fl.lastInvoice.Memo)
	}

	// And that DescriptionHash is SHA256(description)
	h := sha256.Sum256([]byte(zapJSON))
	if !bytes.Equal(fl.lastInvoice.DescriptionHash, h[:]) {
		t.Fatalf("description hash mismatch: got %x want %x",
			fl.lastInvoice.DescriptionHash, h[:])
	}
}

func TestInvoiceCreationWithZapRequest_AmountMismatchIs400(t *testing.T) {
	fl := &mockLightningClient{}
	sh := NewSettlementHandler(fl, "")
	mgr := NewInvoiceManager(&ManagerConfig{
		LndClient:         fl,
		SettlementHandler: sh},
	)

	mux := http.NewServeMux()
	cfg := Config{MinSendableMsat: 1, MaxSendableMsat: 100000000}
	mux.HandleFunc("/invoice/", mgr.HandleInvoiceCreation(cfg))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// build zap for 1000 msat but query uses 2000 msat
	recPk, _ := nostr.GetPublicKey(nostr.GeneratePrivateKey())
	zapJSON := makeZapRequest(t, 1000, recPk)

	u, _ := url.Parse(ts.URL + "/invoice/")
	q := u.Query()
	q.Set("amount", "2000")
	q.Set("nostr", zapJSON)
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(b))
	}
}
