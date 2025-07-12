FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.* .

RUN go mod download -x

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -o /bin/app .

FROM alpine:latest

COPY --from=builder /bin/app /app
