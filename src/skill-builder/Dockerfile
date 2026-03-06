FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY skill-builder.go .
RUN go build -o skill-builder skill-builder.go

FROM golang:1.23-alpine
COPY --from=builder /app/skill-builder /app/skill-builder
CMD ["/app/skill-builder"]