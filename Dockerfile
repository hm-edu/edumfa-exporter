FROM golang:1.25.1
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o edumfa-exporter

FROM alpine:3.23.0@sha256:51183f2cfa6320055da30872f211093f9ff1d3cf06f39a0bdb212314c5dc7375
COPY --from=0 /app/edumfa-exporter /usr/local/bin/edumfa-exporter