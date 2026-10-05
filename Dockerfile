# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .

# Run the test suite: docker build --target test .
FROM build AS test
RUN CGO_ENABLED=0 go test -v ./...

FROM build AS builder
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# Distroless ships CA certificates (needed for HTTPS calls) and runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/server /server
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
