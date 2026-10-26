package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectPostgres membuka connection pool ke PostgreSQL dan memvalidasi koneksi
func ConnectPostgres(ctx context.Context, dsn string, logger *slog.Logger) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("gagal mem-parsing DSN database: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 2
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 15 * time.Minute

	logger.Info("Menghubungkan ke PostgreSQL...", "host", config.ConnConfig.Host, "database", config.ConnConfig.Database)

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat pool koneksi database: %w", err)
	}

	// Cek koneksi dengan ping dan timeout
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("gagal melakukan ping ke database: %w", err)
	}

	logger.Info("Koneksi ke PostgreSQL berhasil terhubung")
	return pool, nil
}

