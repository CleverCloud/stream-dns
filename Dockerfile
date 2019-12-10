FROM golang:stretch

ENV GO111MODULE on

WORKDIR /go/src/stream-dns

COPY go.sum ./go.sum
COPY go.mod ./go.mod

RUN go mod download

ADD . .

RUN CGO_ENABLED=0 GOOS=linux go install -a -installsuffix cgo stream-dns

FROM scratch

COPY --from=0 /go/bin/stream-dns /

EXPOSE 53 53/udp

ENTRYPOINT ["/stream-dns"]