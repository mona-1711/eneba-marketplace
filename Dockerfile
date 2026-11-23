# One image: builds the React app, builds the Go API, serves both from one origin.
FROM node:22-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/server .

FROM alpine:3.20
RUN adduser -D app && mkdir -p /data && chown app /data
USER app
WORKDIR /app
COPY --from=api /out/server ./server
COPY --from=web /web/dist ./public
ENV PORT=8080 STATIC_DIR=/app/public DB_PATH=/data/marketplace.db COOKIE_SECURE=1
EXPOSE 8080
CMD ["./server"]
