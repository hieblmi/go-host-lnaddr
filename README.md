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
- Flexible notifications on payment receipt via email, Telegram, HTTP and Signal (extensible).
- Nostr NIP-05 style account verification: https://github.com/nostr-protocol/nips/blob/master/05.md
- Nostr NIP-57 zaps support (optional).

## Install and Setup
### Clone & Build

You can use the Makefile or the Go toolchain directly.

Using the Makefile:

```bash
# Build the project (produces a local binary in the repo directory)
make

# Install the lnaddr binary into $GOBIN (GOBIN must be set)
make install
# This places the executable at: $GOBIN/lnaddr

# Run tests
make test
```

Alternatively, using the Go toolchain directly:

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

[[Notifiers]]
Type = "mail"
MinAmount = 1000
  [Notifiers.Params]
  From = "tips@sendmesats.com"
  Target = "username@example.com"
  SmtpServer = "smtp.sendmesats.com:587"
  Login = "tips@sendmesats.com"
  Password = "somerandompassword"

[[Notifiers]]
Type = "telegram"
MinAmount = 1000
  [Notifiers.Params]
  ChatId = "1234567890"
  Token = "TelegramToken"

[[Notifiers]]
Type = "http"
MinAmount = 1000
  [Notifiers.Params]
  Target = "https://sendmesats.com/notify?amount={{.Amount}}"
  Method = "POST"
  Encoding = "application/x-www-form-urlencoded"
  BodyTemplate = "message={{.Message}}&title=New+payment+received"

[[Notifiers]]
Type = "signal"
MinAmount = 1000
  [Notifiers.Params]
  FromPhoneNumber = "+12345678901"
  ToPhoneNumber = "+46912345678"
```

#### Notes on Notifiers:
- mail: sends via SMTP using PlainAuth. Target is the recipient address; From/SmtpServer/Login/Password are required.
- telegram: sends a message via Bot API. Provide ChatId and Token; MinAmount filters small payments.
- http: templated URL/body with Encoding controlling Content-Type and escaping. GET ignores BodyTemplate; POST uses it as the request body.
- signal: sends a message via `signal-cli`. Provide FromPhoneNumber and ToPhoneNumber.

#### Signal Notifier Setup
1. (optional) If you need a new phone number for a separate Signal instance, checkout https://silent.link/.
2. Install signal-cli from https://github.com/AsamK/signal-cli
3. Follow the registration instructions https://github.com/AsamK/signal-cli?tab=readme-ov-file#usage
4. Assign the registered number to `FromPhoneNumber` in your `.toml` config.
5. Assign the receiver Signal number to `ToPhoneNumber` in your `.toml` config.

#### Reverse proxy tip (example Nginx): proxy requests for
/.well-known/lnurlp/* and /invoice/* to http://127.0.0.1:9990 while serving your domain over HTTPS.

### Run
```bash
$GOBIN/lnaddr --config /path/to/config.toml
```

You can also run the binary you built via `go install` directly from your Go bin directory. Sample configurations are provided (sample-config.toml, dev-config.toml). JSON configuration files are also supported if you prefer using .json instead of .toml. If using Docker, mount your configuration file and TLS/macaroon files as needed.

## Notes
This project is experimental. Feedback is welcome — please open an issue if you have questions or suggestions, or open a PR yourself if you'd like to contribute.

