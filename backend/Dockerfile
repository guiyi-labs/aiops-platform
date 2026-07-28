FROM golang:1.25-alpine AS dependencies

WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

FROM dependencies AS source

COPY . .

FROM source AS build

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X k8s-aiops.local/backend/internal/buildinfo.Version=${VERSION}" \
    -o /out/api ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S -G app app
USER app
WORKDIR /app

COPY --from=build /out/api /app/api

EXPOSE 8080
ENTRYPOINT ["/app/api"]
