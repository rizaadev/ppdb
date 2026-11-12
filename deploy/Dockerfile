# Stage 1: Build static React Vite SPA (Node.js hanya digunakan di build-time)
FROM node:24-alpine AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Compile single Go binary dengan frontend dist ter-embed
FROM golang:1.26-alpine AS backend-builder
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend-builder /src/frontend/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/pbdb-server ./cmd/server

# Stage 3: Minimal runtime image tanpa Node.js
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend-builder /out/pbdb-server /app/pbdb-server
EXPOSE 8080
USER nobody:nobody
ENTRYPOINT ["/app/pbdb-server"]

