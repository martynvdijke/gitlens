FROM node:24-alpine AS ts-builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY tsconfig.json ./
COPY ts/ ts/
RUN mkdir -p static/js && npx tsc

FROM golang:1.26.4-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=ts-builder /app/static/js ./static/js

ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-X main.Version=${VERSION}" -o gitlens .

FROM alpine:latest
RUN apk add --no-cache sqlite-libs ca-certificates

WORKDIR /app

ENV DOCKER=true
ENV DB_PATH=/db/gitlens.db

COPY --from=builder /app/gitlens .
COPY --from=builder /app/static ./static

RUN mkdir -p /db /app/media && chmod 777 /db /app/media

EXPOSE 6270

CMD ["./gitlens"]
