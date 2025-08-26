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

### Configuration config.json
Below is a minimal, annotated example. Adjust paths/domains to your setup.

```
{
  "RPCHost": "https://127.0.0.1:8080",             // LND REST endpoint
  "InvoiceMacaroonPath": "/home/user/.lnd/invoice.macaroon",
  "TLSCertPath": "/home/user/.lnd/tls.cert",       // used to trust LND's REST TLS

  "LightningAddresses": ["thats@satswellspent.com"], // you can host multiple addresses

  "MinSendable": 1000,                               // msat minimum exposed via LNURLp
  "MaxSendable": 100000000,                          // msat maximum exposed via LNURLp
  "CommentAllowed": 140,                             // 0 disables comments; otherwise max characters
  "Tag": "payRequest",                              // LNURLp tag (usually payRequest)
  "Metadata": [["text/plain","Welcome to satswellspent.com"],["text/identifier","thats@satswellspent.com"]],
  "Thumbnail": "/path/to/thumbnail.png",           // optional: image for some wallets
  "SuccessMessage": "Thank you!",                   // shown by wallets after payment

  // This service will expose: https://yourdomain/.well-known/lnurlp/<username>
  // Your reverse proxy should route that to AddressServerPort below.
  "InvoiceCallback": "https://yourdomain.com/invoice/", // callback base used in LNURLp
  "AddressServerPort": 9990,                               // local port your reverse proxy targets

  // Nostr (optional): see NIP-05 example for structure
  "Nostr": {
    "names": {"thats": "npub1..."},
    "relays": {"npub1...": ["wss://relay.example"]}
  },

  // Zaps (optional): DO NOT use your main keys. Generate with: go-host-lnaddr -genkey
  "Zaps": {"Npub": "npub1...", "Nsec": "nsec1..."},

  // Notifications: one or more entries. MinAmount is in sats and acts as a threshold filter.
  "Notificators": [
    {
      "Type": "mail",
      "Target": "username@example.com",
      "MinAmount": 1000,
      "Params": {
        "From": "thats@satswellspent.com",
        "SmtpServer": "smtp.satswellspent.com:587",
        "Login": "thats@satswellspent.com",
        "Password": "somerandompassword"
      }
    },
    {
      "Type": "telegram",
      "MinAmount": 1000,
      "Params": {"ChatId": "123456789", "Token": "123:bot-token"}
    },
    {
      "Type": "http",
      "Params": {
        "Target": "https://example.com/notify/{{.Amount}}/{{.Message}}",
        "Method": "POST",                           // GET or POST
        "Encoding": "application/x-www-form-urlencoded", // or "application/json"
        "BodyTemplate": "amount={{.Amount}}&message={{.Message}}"
      }
    }
  ]
}
```

Notes on Notificators:
- mail: sends via SMTP using PlainAuth. Target is the recipient address; From/SmtpServer/Login/Password are required.
- telegram: sends a message via Bot API. Provide ChatId and Token; MinAmount filters small payments.
- http: templated URL/body with Encoding controlling Content-Type and escaping. GET ignores BodyTemplate; POST uses it as the request body.

Reverse proxy tip (example Nginx): proxy requests for
/.well-known/lnurlp/* and /invoice/* to http://127.0.0.1:9990 while serving your domain over HTTPS.

### Run
```$GOBIN/go-host-lnaddr --config /path/to/config.json```

You can also run the binary you built via `go install` directly from your Go bin directory. A sample development configuration is provided in dev-config.json. A Dockerfile is included if you prefer containerized deployment; mount your config.json and TLS/macaroon files as needed.

## Notes
This project is experimental. Feedback is welcome — please open an issue if you have questions or suggestions.

