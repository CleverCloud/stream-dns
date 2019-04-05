FROM golang:stretch

ADD . /go/src/stream-dns

RUN go get -v stream-dns

RUN CGO_ENABLED=0 GOOS=linux go install -a -installsuffix cgo stream-dns

FROM scratch

COPY --from=0 /go/bin/stream-dns /

EXPOSE 53 53/udp

ENTRYPOINT ["/stream-dns"]
