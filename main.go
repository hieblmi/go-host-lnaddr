package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	RPCHost             string
	InvoiceMacaroonPath string
	TLSCertPath         string
	Private             bool
	LightningAddresses  []string
	MinSendable         int
	MaxSendable         int
	CommentAllowed      int
	Tag                 string
	Metadata            [][]string
	Thumbnail           string
	SuccessMessage      string
	InvoiceCallback     string
	AddressServerPort   int
	Notificators        []notificatorConfig
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

var (
	sh       SettlementHandler
	backend  LNDParams
	metadata string
)

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

	md, err := metadataToString(config)
	if err != nil {
		log.Printf("WARNING: Unable to convert metadata to string: %s\n", err)
	} else {
		metadata = md
	}

	setupHandlerPerAddress(config)
	macaroonBytes, err := ioutil.ReadFile(config.InvoiceMacaroonPath)
	if err != nil {
		log.Fatalf("Cannot read macaroon file %s: %s", config.InvoiceMacaroonPath, err)
	}

	backend = LNDParams{
		Host:     config.RPCHost,
		Macaroon: fmt.Sprintf("%X", macaroonBytes),
	}

	if config.TLSCertPath != "" {
		tlsCert, err := ioutil.ReadFile(config.TLSCertPath)
		if err != nil {
			log.Fatalf("Cannot read TLS certificate file %s: %s", config.TLSCertPath, err)
		}
		backend.Cert = string(tlsCert)
	} else {
		log.Printf("WARNING: TLSCertPath isn't set, connection to lnd REST API is insecure!")
	}

	err = sh.setupSettlementHandler(backend)
	if err == nil {
		setupNotificators(config)
	} else {
		log.Printf("Settlement handler was not initialized, notifications disabled: %s", err)
	}
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
			Metadata:       metadata,
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
			badRequestError(w, "Mandatory URL Query parameter 'amount' is missing.")
			return
		}

		msat, isInt := strconv.Atoi(keys[0])
		if isInt != nil {
			badRequestError(w, "Amount needs to be a number denoting the number of milli satoshis.")
			return
		}

		if msat < config.MinSendable || msat > config.MaxSendable {
			badRequestError(w, "Wrong amount. Amount needs to be in between [%d,%d] msat", config.MinSendable, config.MaxSendable)
			return
		}

		comment := r.URL.Query().Get("comment")
		if len(comment) > config.CommentAllowed {
			badRequestError(w, "Comment is too long, should be no longer than %d bytes", config.CommentAllowed)
			return
		}

		// parameters ok, creating invoice
		params := Params{
			Msatoshi:    int64(msat),
			Backend:     backend,
			Description: metadata,
		}

		h := sha256.Sum256([]byte(params.Description))
		params.DescriptionHash = h[:]

		if config.Private {
			params.Private = true
		}

		bolt11, r_hash, err := MakeInvoice(params)
		if err != nil {
			log.Printf("Cannot create invoice: %s\n", err)
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
		sh.subscribeToInvoice(r_hash, comment)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(invoice)
	}
}

func metadataToString(config Config) (string, error) {

	thumbnailMetadata, err := thumbnailToMetadata(config.Thumbnail)

	if thumbnailMetadata != nil {
		config.Metadata = append(config.Metadata, thumbnailMetadata)
	}

	marshalledMetadata, err := json.Marshal(config.Metadata)

	return string(marshalledMetadata), err

}

func thumbnailToMetadata(thumbnailPath string) ([]string, error) {

	bytes, err := ioutil.ReadFile(thumbnailPath)
	if err != nil {
		return nil, err
	}

	encoding := http.DetectContentType(bytes)
	switch encoding {
	case "image/jpeg":
		encoding = "image/jpeg;base64"
	case "image/png":
		encoding = "image/png;base64"
	default:
		return nil, errors.New(fmt.Sprintf("Could not determine encoding of thumbnail %s.\n", thumbnailPath))
	}

	encodedThumbnail := base64.StdEncoding.EncodeToString(bytes)

	metadata := []string{encoding, encodedThumbnail}

	return metadata, nil
}

func badRequestError(w http.ResponseWriter, reason string, args ...interface{}) {
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(Error{
		Status: "Error",
		Reason: fmt.Sprintf(reason, args...),
	})
}
