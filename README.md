# ⚡🖥️👾 Host your own Lightning Address(currently on LND only)
Lighting Wallets like BlueWallet, Blixt and [many more](https://github.com/andrerfneves/lightning-address/blob/master/README.md#wallets-supported) allow us to send sats to a [Lighting Addresses](https://lightningaddress.com) like tips@allmysats.com. We can hence pay without scanning the QR code of an invoice.

## Pre-requisites
- An existing Domain name and static ip address to express the lighting address(e.g. user@domain.com)
- A public Lightning Network node with sufficient inbound liquidity to receive payments to your lightning address.
- Golang [installation](https://golang.org/doc/install)
- A Webserver and reverse proxy like Nginx or Caddy. (example setup instructions [here](https://www.digitalocean.com/community/tutorials/how-to-deploy-a-go-web-application-using-nginx-on-ubuntu-18-04)
- Certbot to serve http over TLS. This is required since calls to the LNURLP are done via https. (example setup instructions [here](https://www.digitalocean.com/community/tutorials/how-to-secure-nginx-with-let-s-encrypt-on-ubuntu-18-04)

## Install and Setup
### Clone & Build
```
git clone https://github.com/hieblmi/go-host-lnaddr.git
cd go-host-lnaddr && go build main.go
```
### Configuration config.json
- "RPCHost": This is your lnd's REST/RPC host:port e.g. "https://localhost:8080"
- "InvoiceMacaroon": Generate the hex encoded invoice macaroon : 
```xxd -ps -u -c 1000  $HOME/.lnd/data/chain/bitcoin/mainnet/invoice.macaroon```
- "LightningAddress": Your preferred lightning address, mine is: tips@allmysats.com :-). This resolves to https://allmysats.com/.well-known/lnurlp/tips
- "MinSendable": 1000,
- "MaxSendable": 100000000,
- "CommentAllowed": If set to 0 the sender to your address can't add a comment otherwise the number stands for the permitted number of characters.
- "Tag": "payRequest",
- "Metadata": "[[\"text/plain\",\"Welcome to satsonlightning.com\"],[\"text/identifier\",\"tips@allmysats.com\"]]",
- "InvoiceCallback": "https://[YOUR_DOMAIN].com/invoice/" - this is the endpoint that will create the invoice
- "AddressServerPort": 9990 - the port your reverse proxy points to

### Run
```./main --config config.json```

## Notes
This stuff is experimental. I appreciate your comments and if you have questions please feel free to reach out to hieblmi@protonmail.com.
I just recently setup my own address this way so feel free to send me some sats if you think this has been helpful to you.
 ⚡ **tips@allmysats.com**

