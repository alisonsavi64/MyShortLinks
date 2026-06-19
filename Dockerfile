FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY src/go.mod ./
COPY ./src .
RUN go mod tidy
RUN go build -o app .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/app .
COPY --from=builder /app/migrations ./migrations
CMD ["./app"]
