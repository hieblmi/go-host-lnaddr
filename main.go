package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	baselog "log"
	"net/http"
	"os"
	"strings"

	"github.com/MadAppGang/httplog"
	"github.com/btcsuite/btclog"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

type ServerConfig struct {
	RPCHost             string
	InvoiceMacaroonPath string
	TLSCertPath         string
	WorkingDir          string
	LightningAddresses  []string
	MinSendableMsat     int
	MaxSendableMsat     int
	MaxCommentLength    int
	Tag                 string
	Metadata            [][]string
	Thumbnail           string
	SuccessMessage      string
	InvoiceCallback     string
	AddressServerPort   int
	Nostr               *NostrConfig
	Notificators        []notificatorConfig
}

type LNUrlPay struct {
	MinSendable    int    `json:"minSendable"`
	MaxSendable    int    `json:"maxSendable"`
	CommentAllowed int    `json:"commentAllowed"`
	Tag            string `json:"tag"`
	Metadata       string `json:"metadata"`
	Callback       string `json:"callback"`
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
	log btclog.Logger
)

type NostrConfig struct {
	Names  map[string]string   `json:"names"`
	Relays map[string][]string `json:"relays"`
}

func main() {
	c := flag.String(
		"config", "./config.json", "Specify the configuration file",
	)
	flag.Parse()
	file, err := os.Open(*c)
	if err != nil {
		baselog.Fatalf("cannot open config file %v", err)
	}
	defer func() {
		_ = file.Close()
	}()

	config := ServerConfig{}
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		baselog.Fatalf("cannot decode config JSON %v", err)
	}

	workingDir := config.WorkingDir
	log, err = GetLogger(workingDir, "LNADDR")
	if err != nil {
		baselog.Fatalf("cannot get logger %v", err)
	}

	log.Infof("Starting lightning address server on port %v...",
		config.AddressServerPort)

	clientConn, err := getClientConn(
		config.RPCHost, config.TLSCertPath, config.InvoiceMacaroonPath,
	)
	if err != nil {
		log.Errorf("unable to get a lnd client connection")
		return
	}

	lndClient := lnrpc.NewLightningClient(clientConn)
	settlementHandler := NewSettlementHandler(lndClient)

	invoiceManager := NewInvoiceManager(
		&InvoiceManagerConfig{
			LndClient:         lndClient,
			SettlementHandler: settlementHandler,
		},
	)

	setupHandlerPerAddress(config)
	setupNostrHandlers(config.Nostr)
	setupNotificators(config)

	http.HandleFunc("/invoice/", useLogger(
		invoiceManager.handleInvoiceCreation(config),
	))
	err = http.ListenAndServe(
		fmt.Sprintf(":%d", config.AddressServerPort), nil,
	)
	if err != nil {
		log.Errorf("unable to start server: %v", err)
	}
}

func useLogger(h http.HandlerFunc) http.HandlerFunc {
	logger := httplog.LoggerWithConfig(httplog.LoggerConfig{
		Formatter: httplog.ChainLogFormatter(
			httplog.DefaultLogFormatter,
			httplog.RequestHeaderLogFormatter,
			httplog.RequestBodyLogFormatter,
			httplog.ResponseHeaderLogFormatter,
			httplog.ResponseBodyLogFormatter,
		),
		CaptureBody: true,
	})
	return logger(h).ServeHTTP
}

func setupHandlerPerAddress(config ServerConfig) {
	metadata, err := metadataToString(config)
	if err != nil {
		return
	}
	for _, addr := range config.LightningAddresses {
		addr := strings.Split(addr, "@")[0]
		endpoint := fmt.Sprintf("/.well-known/lnurlp/%s", addr)
		http.HandleFunc(
			endpoint, useLogger(handleLNUrlp(config, metadata)),
		)
	}
}

func handleLNUrlp(config ServerConfig, metadata string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := LNUrlPay{
			MinSendable:    config.MinSendableMsat,
			MaxSendable:    config.MaxSendableMsat,
			CommentAllowed: config.MaxCommentLength,
			Tag:            config.Tag,
			Metadata:       metadata,
			Callback:       config.InvoiceCallback,
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func setupNostrHandlers(nostr *NostrConfig) {
	if nostr == nil {
		return
	}

	http.HandleFunc(
		"/.well-known/nostr.json",
		useLogger(func(w http.ResponseWriter, r *http.Request) {
			log.Infof("Nostr request: %#v\n", *r)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(nostr)
		}),
	)
}

func metadataToString(config ServerConfig) (string, error) {
	thumbnailMetadata, err := thumbnailToMetadata(config.Thumbnail)

	if thumbnailMetadata != nil {
		config.Metadata = append(config.Metadata, thumbnailMetadata)
	}

	marshalledMetadata, err := json.Marshal(config.Metadata)

	return string(marshalledMetadata), err
}

func thumbnailToMetadata(thumbnailPath string) ([]string, error) {
	bytes, err := os.ReadFile(thumbnailPath)
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
		return nil, fmt.Errorf("Unsupported encodeing %s of "+
			"thumbnail %s.\n", encoding, thumbnailPath)
	}
	encodedThumbnail := base64.StdEncoding.EncodeToString(bytes)

	return []string{encoding, encodedThumbnail}, nil
}

func badRequestError(w http.ResponseWriter, reason string,
	args ...interface{}) {

	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(Error{
		Status: "Error",
		Reason: fmt.Sprintf(reason, args...),
	})
}

// maxMsgRecvSize is the largest message our client will receive. We
// set this to 200MiB atm.
var (
	maxMsgRecvSize        = grpc.MaxCallRecvMsgSize(1 * 1024 * 1024 * 200)
	macaroonTimeout int64 = 60
)

func getClientConn(address, tlsCertPath, macaroonPath string) (*grpc.ClientConn,
	error) {

	// We always need to send a macaroon.
	macOption, err := readMacaroon(macaroonPath)
	if err != nil {
		return nil, err
	}

	// TODO (hieblmi) Support Tor dialing
	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(maxMsgRecvSize),
		macOption,
	}

	// TLS cannot be disabled, we'll always have a cert file to read.
	creds, err := credentials.NewClientTLSFromFile(tlsCertPath, "")
	if err != nil {
		return nil, err
	}

	opts = append(opts, grpc.WithTransportCredentials(creds))

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to RPC server: %v",
			err)
	}

	return conn, nil
}

// readMacaroon tries to read the macaroon file at the specified path and create
// gRPC dial options from it.
func readMacaroon(macPath string) (grpc.DialOption, error) {
	// Load the specified macaroon file.
	macBytes, err := os.ReadFile(macPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read macaroon path : %v", err)
	}

	mac := &macaroon.Macaroon{}
	if err = mac.UnmarshalBinary(macBytes); err != nil {
		return nil, fmt.Errorf("unable to decode macaroon: %v", err)
	}

	macConstraints := []macaroons.Constraint{
		// We add a time-based constraint to prevent replay of the
		// macaroon. It's good for 60 seconds by default to make up for
		// any discrepancy between client and server clocks, but leaking
		// the macaroon before it becomes invalid makes it possible for
		// an attacker to reuse the macaroon. In addition, the validity
		// time of the macaroon is extended by the time the server clock
		// is behind the client clock, or shortened by the time the
		// server clock is ahead of the client clock (or invalid
		// altogether if, in the latter case, this time is more than 60
		// seconds).
		macaroons.TimeoutConstraint(macaroonTimeout),
	}

	// Apply constraints to the macaroon.
	constrainedMac, err := macaroons.AddConstraints(mac, macConstraints...)
	if err != nil {
		return nil, err
	}

	// Now we append the macaroon credentials to the dial options.
	cred, err := macaroons.NewMacaroonCredential(constrainedMac)
	if err != nil {
		return nil, fmt.Errorf("error creating macaroon credential: %v",
			err)
	}
	return grpc.WithPerRPCCredentials(cred), nil
}
