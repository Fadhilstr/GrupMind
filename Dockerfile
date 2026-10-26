# ==============================================================================
# Multi-stage Dockerfile untuk AI Agent Telegram Bot & Pengingat Monev
# ==============================================================================

# Stage 1: Build binary menggunakan Go compiler resmi
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install dependensi sertifikat dan git
RUN apk add --no-cache ca-certificates git tzdata

# Cache dependensi go.mod & go.sum
COPY go.mod go.sum ./
RUN go mod download

# Salin seluruh kode sumber
COPY . .

# Kompilasi binary Go secara statis (CGO_ENABLED=0)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /build/botteleabsensi \
    ./cmd/bot

# Stage 2: Runtime image minimal yang aman dan ringan
FROM alpine:3.20

WORKDIR /app

# Install ca-certificates (untuk koneksi HTTPS ke Telegram & Gemini API)
# dan tzdata (untuk dukungan zona waktu Asia/Jakarta)
RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

# Salin binary dari stage builder
COPY --from=builder /build/botteleabsensi /app/botteleabsensi

# Jalankan container menggunakan user non-root
USER appuser

# Expose port HTTP health check
EXPOSE 8080

# Jalankan aplikasi
ENTRYPOINT ["/app/botteleabsensi"]

