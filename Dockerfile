FROM golang:1.24.3-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

RUN go build -o har-mcp cmd/har-mcp/main.go

FROM alpine:3.19.0

COPY --from=builder /app/har-mcp /usr/local/bin/har-mcp

ENTRYPOINT ["har-mcp"]