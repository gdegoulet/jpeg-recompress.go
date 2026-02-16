# Stage 1: Build the static binary
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache build-base

WORKDIR /app

# Copy dependency files and download
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

ARG VERSION=dev

# Build fully static binaries
RUN GOAMD64=v3 CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.Version=${VERSION}" -o jpeg-recompress.go main.go
RUN GOAMD64=v3 CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o jpegli-encode.go jpegli_encode.go

# Final stage: minimal scratch image
FROM scratch
COPY --from=builder /app/jpeg-recompress.go /jpeg-recompress.go
COPY --from=builder /app/jpegli-encode.go /jpegli-encode.go

ENTRYPOINT ["/jpeg-recompress.go"]
