FROM golang:1-alpine as builder

WORKDIR /go/src/github.com/jyap808/bridgeFunder

COPY . .

RUN go build -ldflags="-s -w"

FROM alpine:3

RUN addgroup -S julian -g 1000 && \
    adduser -S julian -G julian -u 1000

COPY --from=builder /go/src/github.com/jyap808/bridgeFunder/bridgeFunder /usr/local/bin

USER julian

CMD ["/usr/local/bin/bridgeFunder"]
