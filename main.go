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

	invoice "github.com/hieblmi/go-host-lnaddr/invoice"
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
	Notifiers           []notifier.Config `json:"Notifiers" toml:"Notifiers"`
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

	config, err := loadConfig(*c)
	if err != nil {
		baselog.Fatalf("failed to load config: %v", err)
	}

	workingDir := config.WorkingDir
	log, err = GetLogger(workingDir, "LNADDR")
	if err != nil {
		baselog.Fatalf("cannot get logger %v", err)
	}
	invoice.SetLogger(log)

	if err := prepareZaps(config.Zaps); err != nil {
		baselog.Fatalf("zaps configuration error: %v", err)
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
	settlementHandler := invoice.NewSettlementHandler(lndClient, config.Zaps.Nsec)

	invoiceManager := invoice.NewInvoiceManager(
		&invoice.ManagerConfig{
			LndClient:         lndClient,
			SettlementHandler: settlementHandler,
		},
	)

	setupHandlerPerAddress(config)
	setupNostrHandlers(config.Nostr)
	notifier.SetupNotifiers(config.Notifiers, log)
	setupIndexHandler(config)

	// Precompute base metadata string once.
	metadata, err := metadataToString(config)
	if err != nil {
		log.Warnf("Unable to convert metadata to string: %v", err)
	}
	payCfg := invoice.Config{
		MinSendableMsat:  config.MinSendableMsat,
		MaxSendableMsat:  config.MaxSendableMsat,
		MaxCommentLength: config.MaxCommentLength,
		SuccessMessage:   config.SuccessMessage,
	}
	http.HandleFunc("/invoice/", useLogger(
		invoiceManager.HandleInvoiceCreation(payCfg, metadata),
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
		log.Warnf("unable to build metadata: %v", err)
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
	if err != nil {
		return "", err
	}

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
		return nil, fmt.Errorf("unsupported encoding %s of "+
			"thumbnail %s", encoding, thumbnailPath)
	}
	encodedThumbnail := base64.StdEncoding.EncodeToString(fileBytes)

	return []string{encoding, encodedThumbnail}, nil
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

	// TLS cannot be disabled; we'll always have a cert file to read.
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

// loadConfig reads and unmarshals a JSON or TOML config into ServerConfig.
func loadConfig(path string) (ServerConfig, error) {
	configBytes, err := os.ReadFile(path)
	if err != nil {
		return ServerConfig{}, fmt.Errorf("cannot read config file "+
			"'%s': %w", path, err)
	}

	config := ServerConfig{}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml", ".tml":
		if err := toml.Unmarshal(configBytes, &config); err != nil {
			return ServerConfig{}, fmt.Errorf("cannot decode "+
				"config TOML: %w", err)
		}

	case ".json":
		if err := json.Unmarshal(configBytes, &config); err != nil {
			return ServerConfig{}, fmt.Errorf("cannot decode "+
				"config JSON: %w", err)
		}
	default:
		return ServerConfig{}, fmt.Errorf("unknown config file "+
			"extension '%s'", ext)
	}

	return config, nil
}

// prepareZaps validates and normalizes the zaps config in-place. It decodes
// bech32 npub/nsec into hex for internal use.
func prepareZaps(z *ZapsConfig) error {
	if z == nil || z.Npub == "" || z.Nsec == "" {
		return nil
	}

	_, sk, err := nip19.Decode(z.Nsec)
	if err != nil {
		return fmt.Errorf("error decoding private nostr zap key: %w",
			err)
	}

	pk, err := nostr.GetPublicKey(sk.(string))
	if err != nil {
		return fmt.Errorf("can't get public nostr zap key from "+
			"private key: %w", err)
	}
	npub, err := nip19.EncodePublicKey(pk)
	if err != nil {
		return fmt.Errorf("error encoding public nostr zap "+
			"key: %w", err)
	}
	if npub != z.Npub {
		return fmt.Errorf("public nostr zap key in config is %s "+
			"doesn't match the expected key %s, make sure you "+
			"entered the correct key pair", z.Npub, npub)
	}

	// save keys hex-encoded
	z.Npub = pk
	z.Nsec = sk.(string)

	return nil
}
