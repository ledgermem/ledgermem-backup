# syntax=docker/dockerfile:1.7
FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /out/getmnemo-backup ./cmd/getmnemo-backup

# pg_dump is required at runtime for snapshot.
FROM alpine:3.20
RUN apk add --no-cache postgresql16-client ca-certificates && update-ca-certificates
COPY --from=build /out/getmnemo-backup /usr/local/bin/getmnemo-backup
ENTRYPOINT ["/usr/local/bin/getmnemo-backup"]
