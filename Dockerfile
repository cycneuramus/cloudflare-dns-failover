FROM golang:1.27-alpine3.24 AS builder

ENV GOARCH $TARGETARCH
ENV CGO_ENABLED=0

WORKDIR /build
COPY *.go go.* ./

RUN go build -buildvcs=false -ldflags="-w -s" -o ./cloudflare-dns-failover

FROM scratch

COPY --from=builder /build/cloudflare-dns-failover /bin/cloudflare-dns-failover
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/bin/cloudflare-dns-failover"]
