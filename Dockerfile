# syntax=docker/dockerfile:1

# --- 1. build the React frontend -> web/dist ---
FROM node:20-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- 2. build the Go server (static binary) ---
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

# --- 3. minimal runtime image ---
FROM alpine:3.20
# ca-certificates is required for the outbound HTTPS calls to the OSM services.
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=web /web/dist /app/web/dist
# MAPS_PROVIDER defaults to the free OSM stack; the host injects $PORT.
ENV MAPS_PROVIDER=osm
EXPOSE 8080
ENTRYPOINT ["/app/server"]
