FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY src/go.mod ./
RUN go mod download
COPY ./src .
RUN go build -o app .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/app .
CMD ["./app"]
