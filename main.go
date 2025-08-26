package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	baselog "log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/MadAppGang/httplog"
	"github.com/btcsuite/btclog"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/hieblmi/go-host-lnaddr/notifier"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/skip2/go-qrcode"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

type ServerConfig struct {
	RPCHost             string            `json:"RPCHost" toml:"RPCHost"`
	InvoiceMacaroonPath string            `json:"InvoiceMacaroonPath" toml:"InvoiceMacaroonPath"`
	TLSCertPath         string            `json:"TLSCertPath" toml:"TLSCertPath"`
	WorkingDir          string            `json:"WorkingDir" toml:"WorkingDir"`
	ExternalURL         string            `json:"ExternalURL" toml:"ExternalURL"`
	ListAllURLs         bool              `json:"ListAllURLs" toml:"ListAllURLs"`
	LightningAddresses  []string          `json:"LightningAddresses" toml:"LightningAddresses"`
	MinSendableMsat     int               `json:"MinSendableMsat" toml:"MinSendableMsat"`
	MaxSendableMsat     int               `json:"MaxSendableMsat" toml:"MaxSendableMsat"`
	MaxCommentLength    int               `json:"MaxCommentLength" toml:"MaxCommentLength"`
	Tag                 string            `json:"Tag" toml:"Tag"`
	Metadata            [][]string        `json:"Metadata" toml:"Metadata"`
	Thumbnail           string            `json:"Thumbnail" toml:"Thumbnail"`
	SuccessMessage      string            `json:"SuccessMessage" toml:"SuccessMessage"`
	InvoiceCallback     string            `json:"InvoiceCallback" toml:"InvoiceCallback"`
	AddressServerPort   int               `json:"AddressServerPort" toml:"AddressServerPort"`
	Nostr               *NostrConfig      `json:"Nostr" toml:"Nostr"`
	Notifiers           []notifier.Config `json:"Notificators" toml:"Notificators"`
	Zaps                *ZapsConfig       `json:"Zaps" toml:"Zaps"`
}

type LNUrlPay struct {
	MinSendable    int    `json:"minSendable"`
	MaxSendable    int    `json:"maxSendable"`
	CommentAllowed int    `json:"commentAllowed"`
	Tag            string `json:"tag"`
	Metadata       string `json:"metadata"`
	Callback       string `json:"callback"`
	AllowsNostr    bool   `json:"allowsNostr"`
	NostrPubkey    string `json:"nostrPubkey"`
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
	Names  map[string]string   `json:"names" toml:"names"`
	Relays map[string][]string `json:"relays" toml:"relays"`
}

type ZapsConfig struct {
	Npub string
	Nsec string
}

func main() {
	c := flag.String(
		"config", "./config.json", "Specify the configuration file",
	)
	gk := flag.Bool("genkey", false, "Generate nostr keypair for zaps")
	flag.Parse()

	if *gk {
		sk := nostr.GeneratePrivateKey()
		pk, _ := nostr.GetPublicKey(sk)
		nsec, _ := nip19.EncodePrivateKey(sk)
		npub, _ := nip19.EncodePublicKey(pk)
		fmt.Printf("npub: %s\nnsec: %s\n", npub, nsec)
		return
	}

	configBytes, err := os.ReadFile(*c)
	if err != nil {
		baselog.Fatalf("cannot read config file '%s': %v", *c, err)
	}

	config := ServerConfig{}
	ext := strings.ToLower(filepath.Ext(*c))
	switch ext {
	case ".toml", ".tml":
		if err := toml.Unmarshal(configBytes, &config); err != nil {
			baselog.Fatalf("cannot decode config TOML: %v", err)
		}

	case ".json":
		if err := json.Unmarshal(configBytes, &config); err != nil {
			baselog.Fatalf("cannot decode config JSON: %v", err)
		}

	default:
		baselog.Fatalf("unknown config file extension '%s'", ext)
	}

	workingDir := config.WorkingDir
	log, err = GetLogger(workingDir, "LNADDR")
	if err != nil {
		baselog.Fatalf("cannot get logger %v", err)
	}

	if isZapsConfigured(config) {
		_, sk, err := nip19.Decode(config.Zaps.Nsec)
		if err != nil {
			baselog.Fatalf("Error decoding private nostr zap "+
				"key: %s", err)
		}
		pk, err := nostr.GetPublicKey(sk.(string))
		if err != nil {
			baselog.Fatalf("Can't get public nostr zap key from "+
				"private key: %s", err)
		}
		npub, err := nip19.EncodePublicKey(pk)
		if err != nil {
			baselog.Fatalf("Error encoding public nostr zap "+
				"key: %s", err)
		}
		if npub != config.Zaps.Npub {
			baselog.Fatalf("Public nostr zap key in config is %s "+
				"doesn't match the expected key %s, make "+
				"sure you entered the correct key pair",
				config.Zaps.Npub, npub)
		}
		// save keys hex-encoded
		config.Zaps.Npub = pk
		config.Zaps.Nsec = sk.(string)
	}

	log.Infof("Starting lightning address server on port %v...",
		config.AddressServerPort)

	clientConn, err := getClientConn(
		config.RPCHost, config.TLSCertPath, config.InvoiceMacaroonPath,
	)
	if err != nil {
		log.Errorf("unable to get a lnd client connection: %v", err)
		return
	}

	lndClient := lnrpc.NewLightningClient(clientConn)
	settlementHandler := NewSettlementHandler(lndClient, config.Zaps.Nsec)

	invoiceManager := NewInvoiceManager(
		&InvoiceManagerConfig{
			LndClient:         lndClient,
			SettlementHandler: settlementHandler,
		},
	)

	setupHandlerPerAddress(config)
	setupNostrHandlers(config.Nostr)
	notifier.SetupNotifiers(config.Notifiers, log)
	setupIndexHandler(config)

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

		if isZapsConfigured(config) {
			resp.AllowsNostr = true
			resp.NostrPubkey = config.Zaps.Npub
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
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
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(nostr)
		}),
	)
}

func setupIndexHandler(config ServerConfig) {
	if !config.ListAllURLs || len(config.LightningAddresses) == 0 ||
		config.ExternalURL == "" {
		return
	}

	type user struct {
		User    string
		Encoded string
		QRCode  string
	}

	var users []user
	for _, addr := range config.LightningAddresses {
		userName := strings.Split(addr, "@")[0]
		url := fmt.Sprintf("%s/.well-known/lnurlp/%s",
			config.ExternalURL, userName)

		converted, err := bech32.ConvertBits([]byte(url), 8, 5, true)
		if err != nil {
			log.Errorf("Unable to convert url: %v", err)
		}

		lnurl, err := bech32.Encode("lnurl", converted)
		if err != nil {
			log.Errorf("Unable to encode url: %v", err)
			continue
		}

		png, err := qrcode.Encode(lnurl, qrcode.Highest, 256)
		if err != nil {
			log.Errorf("Unable to encode QR code: %v", err)
			continue
		}

		users = append(users, user{
			User:    userName,
			Encoded: lnurl,
			QRCode:  base64.StdEncoding.EncodeToString(png),
		})

	}
	htmlTemplate := `<!DOCTYPE html>
<html>
<head>
	<title>LNURLs</title>
</head>
<body>
	<h1>LNURLs</h1>
	<ul>
		{{range .}}
		<li>
			<h2>User: {{.User}}</h2>
			<img src="data::image/png;base64, {{.QRCode}}" style="margin-left:-18px"/><br/>
			<pre>{{.Encoded}}</pre>
		</li>
		{{end}}
	</ul>
</body>
</html>
`

	bodyTemlate, err := template.New("html").Parse(htmlTemplate)
	if err != nil {
		log.Errorf("Error building URL template: %w", err)
		return
	}

	var buf bytes.Buffer
	err = bodyTemlate.Execute(&buf, users)
	if err != nil {
		log.Errorf("Error executing URL template: %w", err)
		return
	}

	http.HandleFunc(
		"/", useLogger(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(buf.Bytes())
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
	fileBytes, err := os.ReadFile(thumbnailPath)
	if err != nil {
		return nil, err
	}

	encoding := http.DetectContentType(fileBytes)
	switch encoding {
	case "image/jpeg":
		encoding = "image/jpeg;base64"
	case "image/png":
		encoding = "image/png;base64"
	default:
		return nil, fmt.Errorf("Unsupported encodeing %s of "+
			"thumbnail %s.\n", encoding, thumbnailPath)
	}
	encodedThumbnail := base64.StdEncoding.EncodeToString(fileBytes)

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
	maxMsgRecvSize = grpc.MaxCallRecvMsgSize(1 * 1024 * 1024 * 200)
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

	// Now we append the macaroon credentials to the dial options.
	cred, err := macaroons.NewMacaroonCredential(mac)
	if err != nil {
		return nil, fmt.Errorf("error creating macaroon credential: %v",
			err)
	}
	return grpc.WithPerRPCCredentials(cred), nil
}

func isZapsConfigured(config ServerConfig) bool {
	return config.Zaps != nil && config.Zaps.Npub != "" &&
		config.Zaps.Nsec != ""
}
