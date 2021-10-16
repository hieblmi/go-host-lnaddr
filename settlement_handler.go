package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type SettlementHandler struct {
	hc       http.Client
	host     string
	macaroon string
}

func (sh *SettlementHandler) setupSettlementHandler(backend LNDParams) (err error) {
	cfg := &tls.Config{}
	if backend.Cert != "" {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM([]byte(backend.Cert))
		cfg.RootCAs = caCertPool
	} else {
		cfg.InsecureSkipVerify = true
	}
	t := &http.Transport{TLSClientConfig: cfg}
	sh.hc.Transport = t
	sh.host = backend.Host
	sh.macaroon = backend.Macaroon
	return
}

func (sh *SettlementHandler) get(url string) (*http.Response, error) {
	return sh.do("GET", url)
}

func (sh *SettlementHandler) do(method string, url string) (*http.Response, error) {
	req, err := http.NewRequest(method, sh.host+url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Grpc-Metadata-macaroon", sh.macaroon)
	resp, err := sh.hc.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (sh *SettlementHandler) subscribeToInvoice(r_hash string) error {
	resp, err := sh.get("/v2/invoices/subscribe/" + strings.NewReplacer("+", "-", "/", "_").Replace(r_hash))
	if err != nil {
		log.Printf("Error subscribing to invoice: %s", err)
		return err
	}
	go func() {
		dec := json.NewDecoder(resp.Body)
		for dec.More() {
			var invoice struct {
				Result struct {
					AmtPaidSat  string `json:"amt_paid_sat"`
					Settled     bool
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
					broadcastNotification(uint(amt))
				}
			}
		}
	}()
	return nil
}
