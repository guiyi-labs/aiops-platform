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
    -o /out/api ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/credential-reencrypt ./cmd/credential-reencrypt && \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X k8s-aiops.local/backend/internal/buildinfo.Version=${VERSION}" \
    -o /out/audit-archive ./cmd/audit-archive && \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/identity-readiness ./cmd/identity-readiness && \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/recovery-readiness ./cmd/recovery-readiness

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S -G app app
USER app
WORKDIR /app

COPY --from=build /out/api /app/api
COPY --from=build /out/credential-reencrypt /app/credential-reencrypt
COPY --from=build /out/audit-archive /app/audit-archive
COPY --from=build /out/identity-readiness /app/identity-readiness
COPY --from=build /out/recovery-readiness /app/recovery-readiness

EXPOSE 8080
ENTRYPOINT ["/app/api"]
