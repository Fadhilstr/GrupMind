package database

import (
	"context"
	"embed"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// RunMigrations menjalankan seluruh skrip migrasi SQL pada database
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	migrationFiles := []string{
		"migrations/000001_init_schema.up.sql",
	}

	for _, file := range migrationFiles {
		logger.Info("Menjalankan migrasi database", "file", file)
		content, err := migrationFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("gagal membaca file migrasi %s: %w", file, err)
		}

		if _, err := pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("gagal mengeksekusi migrasi %s: %w", file, err)
		}
		logger.Info("Migrasi database berhasil dieksekusi", "file", file)
	}

	return nil
}

