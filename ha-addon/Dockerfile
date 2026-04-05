ARG BUILD_FROM
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /usr/bin/srne .

FROM ${BUILD_FROM}

COPY --from=builder /usr/bin/srne /usr/bin/srne
COPY ha-addon/run.sh /etc/services.d/srne/run

RUN chmod a+x /etc/services.d/srne/run
