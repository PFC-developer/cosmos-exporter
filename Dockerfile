FROM golang:1.24-alpine AS builder

COPY . /app

WORKDIR /app

RUN go build ./cmd/cosmos-exporter
RUN go build ./cmd/kuji-cosmos-exporter
RUN go build ./cmd/sei-cosmos-exporter
RUN go build ./cmd/inj-cosmos-exporter
RUN go build ./cmd/pryzm-cosmos-exporter
RUN go build ./cmd/initia-cosmos-exporter


FROM alpine

COPY --from=builder /app/cosmos-exporter /usr/local/bin/cosmos-exporter
COPY --from=builder /app/kuji-cosmos-exporter /usr/local/bin/kuji-cosmos-exporter
COPY --from=builder /app/sei-cosmos-exporter /usr/local/bin/sei-cosmos-exporter
COPY --from=builder /app/inj-cosmos-exporter /usr/local/bin/inj-cosmos-exporter
COPY --from=builder /app/pryzm-cosmos-exporter /usr/local/bin/pryzm-cosmos-exporter
COPY --from=builder /app/initia-cosmos-exporter /usr/local/bin/initia-cosmos-exporter

ENTRYPOINT [ "/usr/local/bin/cosmos-exporter" ]
