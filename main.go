package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/fiatjaf/makeinvoice"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Configuration struct {
	RPCHost           string
	InvoiceMacaroon   string
	LightningAddress  string
	MinSendable       int
	MaxSendable       int
	CommentAllowed    int
	Tag               string
	Metadata          string
	InvoiceCallback   string
	AddressServerPort int
}

type LNUrlPay struct {
	MinSendable     int    `json:"minSendable"`
	MaxSendable     int    `json:"maxSendable"`
	CommentAllowed  int    `json:"commentAllowed"`
	Tag             string `json:"tag"`
	Metadata        string `json:"metadata"`
	Callback        string `json:"callback"`
	DescriptionHash []byte
}

type Invoice struct {
	Pr     string   `json:"pr"`
	Routes []string `json:"routes"`
}

type Error struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func main() {
	c := flag.String("config", "./config.json", "Specify the configuration file")
	flag.Parse()
	file, err := os.Open(*c)
	if err != nil {
		log.Fatal("Cannot open config file: ", err)
	}
	defer file.Close()

	Config := Configuration{}
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&Config)
	if err != nil {
		log.Fatal("Cannot decode config JSON: ", err)
	}
	log.Printf("Printing config.json: %#v\n", Config)

	lnurlp := fmt.Sprintf("/.well-known/lnurlp/%s", strings.Split(Config.LightningAddress, "@")[0])

	http.HandleFunc(string(lnurlp), handleLNUrlp(Config))
	http.HandleFunc("/invoice/", handleInvoiceCreation(Config))

	http.ListenAndServe(fmt.Sprintf(":%d", Config.AddressServerPort), nil)
}

func handleLNUrlp(config Configuration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("LNUrlp request: %#v\n", *r)
		resp := LNUrlPay{
			MinSendable:    config.MinSendable,
			MaxSendable:    config.MaxSendable,
			CommentAllowed: config.CommentAllowed,
			Tag:            config.Tag,
			Metadata:       config.Metadata,
			Callback:       config.InvoiceCallback,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}
}

func handleInvoiceCreation(config Configuration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		keys, hasAmount := r.URL.Query()["amount"]

		if !hasAmount || len(keys[0]) < 1 {
			err := Error{
				Status: "ERROR",
				Reason: "URL Query parameter 'amount' is missing.",
			}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err)
			return
		}

		msat, isInt := strconv.Atoi(keys[0])
		if isInt != nil {
			err := Error{
				Status: "ERROR",
				Reason: "Amount needs to be a number denoting the number of milli satoshis.",
			}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err)
			return
		}

		if msat < config.MinSendable || msat > config.MaxSendable {
			reason := fmt.Sprintf("Wrong amount. Amount needs to be in between [%d,%d] msat", config.MinSendable, config.MaxSendable)
			err := Error{
				Status: "ERROR",
				Reason: reason,
			}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err)
			return
		}

		// create invoice
		backend := makeinvoice.LNDParams{
			Host:     config.RPCHost,
			Macaroon: config.InvoiceMacaroon,
		}

		label := fmt.Sprintf("%s: %d sats", strconv.FormatInt(time.Now().Unix(), 16), msat)
		params := makeinvoice.Params{
			Msatoshi:    int64(msat),
			Backend:     backend,
			Label:       label,
			Description: config.Metadata,
		}

		h := sha256.Sum256([]byte(params.Description))
		params.DescriptionHash = h[:]

		bolt11, err := makeinvoice.MakeInvoice(params)
		if err != nil {
			log.Printf("Cannot create invoice: %s\n", err)
		} else {
			log.Printf("Invoice creation succeeded: %s\n", bolt11)
		}

		invoice := Invoice{
			Pr:     bolt11,
			Routes: make([]string, 0, 0),
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(invoice)
	}
}
