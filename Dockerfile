FROM golang:1.20.1-alpine as builder

ARG TARGETARCH
ARG TARGETOS

LABEL maintainer="info@reinkrul.nl"

ENV GO111MODULE on
ENV GOPATH /

RUN mkdir /app-src && cd /app-src
COPY go.mod .
COPY go.sum .
RUN go mod download && go mod verify

COPY . .
RUN go build -o /app

# alpine
FROM alpine:3.17.2
COPY --from=builder /app /app

HEALTHCHECK --start-period=10s --timeout=5s --interval=2s \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ || exit 1

EXPOSE 8080
ENTRYPOINT ["/app"]

