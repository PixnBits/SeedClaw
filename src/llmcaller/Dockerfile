FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY llmcaller.go .
RUN go build -o llmcaller llmcaller.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/llmcaller /app/llmcaller
CMD ["/app/llmcaller"]