FROM golang:1.27.1-alpine3.24 AS builder

WORKDIR /app
COPY . /app
ARG BUILD_REVISION
RUN CGO_ENABLED=0 go build -ldflags "-X github.com/samcro1967/glance/internal/glance.buildRevision=${BUILD_REVISION}" .

FROM alpine:3.25

RUN apk upgrade --no-cache

WORKDIR /app
COPY --from=builder /app/glance .

EXPOSE 8080/tcp
ENTRYPOINT ["/app/glance", "--config", "/app/config/glance.yml"]
