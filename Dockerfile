# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25.5

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG SERVICE=api-gateway
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cityevents ./cmd/${SERVICE}

FROM alpine:3.22
RUN apk add --no-cache ca-certificates

COPY --from=build /out/cityevents /usr/local/bin/cityevents

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/cityevents"]
