FROM golang:1.24.6-alpine as builder

# Force Go to use the cgo based DNS resolver. This is required to ensure DNS
# queries required to connect to linked containers succeed.
ENV GODEBUG netdns=cgo

# Explicitly turn on the use of modules (until this becomes the default).
ENV GO111MODULE on

# Install dependencies.
RUN apk add --no-cache --update alpine-sdk git make

# Build and install binary.
COPY . /go/src/github.com/hieblmi/go-host-lnaddr
RUN cd /go/src/github.com/hieblmi/go-host-lnaddr && go install ./...

# Start a new, final image to reduce size.
FROM alpine as final

# Expose port.
EXPOSE 9990

# Copy the binary from the builder image.
COPY --from=builder /go/bin/go-host-lnaddr /bin/

# Add bash.
RUN apk add --no-cache \
    bash \
    ca-certificates

RUN mkdir -p /app
WORKDIR /app

ENTRYPOINT ["go-host-lnaddr"]
