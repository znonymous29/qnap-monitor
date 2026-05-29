# --- Stage 1: build frontend ---
FROM node:20-alpine AS frontend
WORKDIR /app
# NOTE: deliberately NOT copying package-lock.json — npm's optional native
# dependencies (e.g. @rollup/rollup-linux-*-musl) are pinned per-platform in the
# lockfile, and a lockfile generated on the host will miss the variant required
# inside this alpine/musl build image. Letting npm re-resolve here is reliable.
COPY frontend/package.json ./
RUN npm config set registry https://registry.npmmirror.com && npm install --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# --- Stage 2: build backend with embedded frontend ---
FROM golang:1.22-alpine AS backend
# Use a CN-friendly module proxy to avoid timeouts to proxy.golang.org.
ENV GOPROXY=https://goproxy.cn,https://goproxy.io,direct
ENV GOSUMDB=off
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Replace the embed placeholder with the real built frontend.
COPY --from=frontend /app/dist ./cmd/server/web
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/qnap-monitor ./cmd/server

# --- Stage 3: minimal runtime ---
# The Go binary is statically compiled (CGO_ENABLED=0), so we just need a
# minimal image that provides /tmp and /etc/resolv.conf. alpine:3.19 is ~5 MB.
FROM alpine:3.19
COPY --from=backend /out/qnap-monitor /qnap-monitor
RUN mkdir -p /data
EXPOSE 8080
VOLUME ["/data"]
ENV ADDR=:8080 DATA_DIR=/data
ENTRYPOINT ["/qnap-monitor"]
