FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/baha ./cmd/baha

FROM alpine:3.24
RUN apk add --no-cache ca-certificates curl
COPY --from=build /out/baha /usr/local/bin/baha
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/baha"]
CMD ["serve"]
