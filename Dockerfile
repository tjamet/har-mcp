ARG BASE_REGISTRY=docker.io/library

FROM --platform=$BUILDPLATFORM ${BASE_REGISTRY}/golang:1.25.14-alpine AS builder

ARG TARGETPLATFORM

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOARCH=$(echo ${TARGETPLATFORM:-linux/amd64} | cut -d/ -f2) go build -o har-mcp cmd/har-mcp/main.go

FROM ${BASE_REGISTRY}/alpine:3.19.0

COPY --from=builder /app/har-mcp /usr/local/bin/har-mcp

ENTRYPOINT ["har-mcp"]