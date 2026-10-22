FROM golang:1.27.2
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o edumfa-exporter

FROM alpine:3.24.3@sha256:b3413572228e34bd1c9245f5565e6cd127005655e8a7899aef33d8e83255c9e6
COPY --from=0 /app/edumfa-exporter /usr/local/bin/edumfa-exporter