# Build-from-source image: `git clone … && docker build .` produces a working
# app with no prerequisites. This is the path for local/standalone use.
# Published release images are built separately by GoReleaser via
# Dockerfile.goreleaser (which repackages the release binaries).
FROM golang:1.26.4-alpine3.23@sha256:eb5a920799142c2fe9ec705cfa0ebcc4380e2e2f041b84e9ae5c6d82a2e56c82 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /saasquatch

FROM alpine:3.24@sha256:f5064d3e5f88c467c714509f491853ab2d951932c5cad699c0cb969dcec6f3b4

RUN addgroup -S saasquatchuser && adduser -S saasquatchuser -G saasquatchuser

WORKDIR /app
COPY --from=build /saasquatch ./saasquatch
COPY rules/ ./rules/

USER saasquatchuser
ENTRYPOINT ["./saasquatch"]
CMD ["-h"]
