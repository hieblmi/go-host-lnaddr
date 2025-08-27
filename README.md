# ⚡🖥️👾 Host your own Lightning Address on LND
Lightning wallets like [Zeus](https://github.com/ZeusLN/zeus), [Blixt](https://blixtwallet.github.io) and [many more](https://github.com/andrerfneves/lightning-address/blob/master/README.md#wallets-supported) let you send sats to a [Lightning Address](https://lightningaddress.com) such as thats@satswellspent.com — no QR code scan required.

## Pre-requisites
- A domain name and a static IP (or DNS) to host your Lightning Address (e.g., user@domain.com). A reverse proxy will typically terminate TLS for your domain. 
- A public Lightning Network node (LND) with enough inbound liquidity to receive payments.
- Go toolchain installed: see [installation docs](https://golang.org/doc/install).
- A web server and reverse proxy like Nginx or Caddy for routing to the service (example setup [here](https://www.digitalocean.com/community/tutorials/how-to-deploy-a-go-web-application-using-nginx-on-ubuntu-18-04)).
- Valid TLS certificates (e.g., via Certbot) because LNURLp endpoints must be served over HTTPS (guide [here](https://www.digitalocean.com/community/tutorials/how-to-secure-nginx-with-let-s-encrypt-on-ubuntu-18-04)).

## Features
- Implements the Lightning Address (LNURLp) flow to receive Lightning payments to an email-like address.
- Host multiple Lightning Addresses on the same server instance.
- Flexible notifications on payment receipt via email, Telegram, and HTTP (extensible).
- Nostr NIP-05 style account verification: https://github.com/nostr-protocol/nips/blob/master/05.md
- Nostr NIP-57 zaps support (optional).

## Install and Setup
### Clone & Build
```
go install github.com/hieblmi/go-host-lnaddr@latest
```

### Configuration (TOML)
Below is a minimal TOML example. Adjust paths/domains to your setup. A JSON configuration is also supported if you prefer.

```toml
RPCHost = "localhost:10009"
InvoiceMacaroonPath = "/lnd/macaroonpath/invoices.macaroon"
TLSCertPath = "/home/alice/.lnd/tls.cert"
WorkingDir = "/home/alice/.go-host-lnaddr"
ExternalURL = "https://sendmesats.com"
ListAllURLs = true
LightningAddresses = ["tips@sendmesats.com"]
MinSendableMsat = 1000
MaxSendableMsat = 100000000
MaxCommentLength = 150
Tag = "payRequest"
Metadata = [
  ["text/plain", "Welcome to sendmesats.com"],
  ["text/identifier", "tips@sendmesats.com"],
]
Thumbnail = "/path/to/thumbnail.[jpeg|png]"
SuccessMessage = "Thank you!"
InvoiceCallback = "https://sendmesats.com/invoice/"
AddressServerPort = 9990

[Nostr]
  [Nostr.names]
  myNostrUsername = "npub1h....."

  [Nostr.relays]
  "b9b....." = ["wss://my.relay.com"]

[Zaps]
Npub = "npub1..."
Nsec = "nsec1..."

[[Notificators]]
Type = "mail"
MinAmount = 1000
  [Notificators.Params]
  From = "tips@sendmesats.com"
  Target = "username@example.com"
  SmtpServer = "smtp.sendmesats.com:587"
  Login = "tips@sendmesats.com"
  Password = "somerandompassword"

[[Notificators]]
Type = "telegram"
MinAmount = 1000
  [Notificators.Params]
  ChatId = "1234567890"
  Token = "TelegramToken"

[[Notificators]]
Type = "http"
MinAmount = 1000
  [Notificators.Params]
  Target = "https://sendmesats.com/notify?amount={{.Amount}}"
  Method = "POST"
  Encoding = "application/x-www-form-urlencoded"
  BodyTemplate = "message={{.Message}}&title=New+payment+received"
```

Notes on Notificators:
- mail: sends via SMTP using PlainAuth. Target is the recipient address; From/SmtpServer/Login/Password are required.
- telegram: sends a message via Bot API. Provide ChatId and Token; MinAmount filters small payments.
- http: templated URL/body with Encoding controlling Content-Type and escaping. GET ignores BodyTemplate; POST uses it as the request body.

Reverse proxy tip (example Nginx): proxy requests for
/.well-known/lnurlp/* and /invoice/* to http://127.0.0.1:9990 while serving your domain over HTTPS.

### Run
```bash
$GOBIN/go-host-lnaddr --config /path/to/config.toml
```

You can also run the binary you built via `go install` directly from your Go bin directory. Sample configurations are provided (sample-config.toml, dev-config.toml). JSON configuration files are also supported if you prefer using .json instead of .toml. If using Docker, mount your configuration file and TLS/macaroon files as needed.

## Notes
This project is experimental. Feedback is welcome — please open an issue if you have questions or suggestions.

