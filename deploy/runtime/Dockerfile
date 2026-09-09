# syntax=docker/dockerfile:1.7

FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/baha ./cmd/baha

FROM alpine:3.22
RUN apk add --no-cache ca-certificates curl
COPY --from=build /out/baha /usr/local/bin/baha
ENTRYPOINT ["/usr/local/bin/baha"]
