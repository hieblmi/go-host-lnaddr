# ⚡🖥️👾 Host your own Lightning Address
Lighting Wallets like BlueWallet, Blixt and [many more](https://github.com/andrerfneves/lightning-address/blob/master/README.md#wallets-supported) allow us to send sats to a [Lighting Addresses](https://lightningaddress.com) like tips@allmysats.com. We can hence pay without scanning the QR code of an invoice.

## Prerequisites
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
### Configuration


