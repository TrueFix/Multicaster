FROM node:22-alpine AS web
WORKDIR /web
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /cmd/multicaster/web ./cmd/multicaster/web
ARG VERSION
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/truefix/multicaster/internal/updates.Version=${VERSION:-$(cat VERSION)}" -o /multicaster ./cmd/multicaster

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /multicaster /app/multicaster
EXPOSE 1935 8080
ENV MULTICASTER_DATA_DIR=/app/data
VOLUME ["/app/data"]
ENTRYPOINT ["/app/multicaster"]
