package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fiatjaf/makeinvoice"
)

type Config struct {
	RPCHost             string
	InvoiceMacaroonPath string
	LightningAddresses  []string
	MinSendable         int
	MaxSendable         int
	CommentAllowed      int
	Tag                 string
	Metadata            string
	SuccessMessage      string
	InvoiceCallback     string
	AddressServerPort   int
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
	Pr            string         `json:"pr"`
	Routes        []string       `json:"routes"`
	SuccessAction *SuccessAction `json:"successAction"`
}

type Error struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type SuccessAction struct {
	Tag     string `json:"tag"`
	Message string `json:"message,omitempty"`
}

func main() {
	c := flag.String("config", "./config.json", "Specify the configuration file")
	flag.Parse()
	file, err := os.Open(*c)
	if err != nil {
		log.Fatal("Cannot open config file: ", err)
	}
	defer file.Close()

	config := Config{}
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal("Cannot decode config JSON: ", err)
	}
	log.Printf("Printing config.json: %#v\n", config)

	setupHandlerPerAddress(config)
	http.HandleFunc("/invoice/", handleInvoiceCreation(config))
	http.ListenAndServe(fmt.Sprintf(":%d", config.AddressServerPort), nil)
}

func setupHandlerPerAddress(config Config) {
	for _, addr := range config.LightningAddresses {
		http.HandleFunc(fmt.Sprintf("/.well-known/lnurlp/%s", strings.Split(addr, "@")[0]), handleLNUrlp(config))
	}
}

func handleLNUrlp(config Config) http.HandlerFunc {
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

func handleInvoiceCreation(config Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		keys, hasAmount := r.URL.Query()["amount"]

		if !hasAmount || len(keys[0]) < 1 {
			err := getErrorResponse("Mandatory URL Query parameter 'amount' is missing.")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err)
			return
		}

		msat, isInt := strconv.Atoi(keys[0])
		if isInt != nil {
			err := getErrorResponse("Amount needs to be a number denoting the number of milli satoshis.")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err)
			return
		}

		if msat < config.MinSendable || msat > config.MaxSendable {
			err := getErrorResponse(fmt.Sprintf("Wrong amount. Amount needs to be in between [%d,%d] msat", config.MinSendable, config.MaxSendable))
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err)
			return
		}

		// parameters ok, creating invoice
		macaroonBytes, err := ioutil.ReadFile(config.InvoiceMacaroonPath)
		if err != nil {
			log.Fatal(fmt.Sprintf("Cannot read macaroon file %s", config.InvoiceMacaroonPath), err)
		}

		backend := makeinvoice.LNDParams{
			Host:     config.RPCHost,
			Macaroon: fmt.Sprintf("%X", macaroonBytes),
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
			err := getErrorResponse("Invoice creation failed.")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err)
			return
		}

		invoice := Invoice{
			Pr:     bolt11,
			Routes: make([]string, 0, 0),
			SuccessAction: &SuccessAction{
				Tag:     "message",
				Message: config.SuccessMessage,
			},
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(invoice)
	}
}

func getErrorResponse(reason string) (err Error) {
	return Error{
		Status: "Error",
		Reason: reason,
	}
}
