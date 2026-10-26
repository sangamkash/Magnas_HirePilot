package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/magnas/hirepilot/internal/config"
	"github.com/magnas/hirepilot/internal/database"
	"github.com/magnas/hirepilot/internal/handler"
	"github.com/magnas/hirepilot/internal/migrate"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{Use: "hirepilot", Short: "Magnas Hiring Pilot backend", Args: cobra.NoArgs, RunE: runServer}
	root.AddCommand(newMigrateCommand())
	return root
}

func runServer(_ *cobra.Command, _ []string) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		return err
	}

	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Error("configure database", "error", err)
		return err
	}
	defer db.Close()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.New(db),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown", "error", err)
		}
	}()

	logger.Info("HTTP server starting", "addr", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP server failed", "error", err)
		return err
	}
	return nil
}

func newMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "migrate", Short: "Manage PostgreSQL schema migrations", Args: cobra.NoArgs}
	runner := func(ctx context.Context) (migrate.Runner, func(), error) {
		cfg, err := config.Load()
		if err != nil {
			return migrate.Runner{}, nil, err
		}
		pool, err := database.Open(cfg.DatabaseURL)
		if err != nil {
			return migrate.Runner{}, nil, err
		}
		return migrate.Runner{Dir: cfg.MigrationsDir, Store: migrate.PostgresStore{Pool: pool}}, pool.Close, nil
	}
	cmd.AddCommand(&cobra.Command{Use: "up", Short: "Apply all pending migrations", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		r, close, err := runner(c.Context())
		if err != nil {
			return err
		}
		defer close()
		if err := r.Up(c.Context()); err != nil {
			return err
		}
		fmt.Fprintln(c.OutOrStdout(), "Migrations applied.")
		return nil
	}})
	down := &cobra.Command{Use: "down [count]", Short: "Roll back the latest migration(s)", Args: cobra.MaximumNArgs(1), RunE: func(c *cobra.Command, args []string) error {
		count := 1
		if len(args) == 1 {
			n, err := strconv.Atoi(args[0])
			if err != nil || n < 1 {
				return errors.New("count must be a positive integer")
			}
			count = n
		}
		r, close, err := runner(c.Context())
		if err != nil {
			return err
		}
		defer close()
		if err := r.Down(c.Context(), count); err != nil {
			return err
		}
		fmt.Fprintf(c.OutOrStdout(), "Rolled back up to %d migration(s).\n", count)
		return nil
	}}
	cmd.AddCommand(down)
	cmd.AddCommand(&cobra.Command{Use: "status", Short: "Show applied and pending migrations", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		r, close, err := runner(c.Context())
		if err != nil {
			return err
		}
		defer close()
		migrations, applied, err := r.Status(c.Context())
		if err != nil {
			return err
		}
		for _, m := range migrations {
			state := "pending"
			if _, ok := applied[m.Version]; ok {
				state = "applied"
			}
			fmt.Fprintf(c.OutOrStdout(), "%06d_%s %s\n", m.Version, m.Name, state)
		}
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "create <name>", Short: "Create a paired SQL migration", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error { return createMigration(c, args[0]) }})
	return cmd
}

var createName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func createMigration(c *cobra.Command, name string) error {
	if !createName.MatchString(name) {
		return errors.New("name must use lowercase letters, digits, underscores, or hyphens")
	}
	dir := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR"))
	if dir == "" {
		dir = "migrations"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	max := 0
	for _, entry := range entries {
		base := filepath.Base(entry.Name())
		if len(base) >= 6 {
			n, e := strconv.Atoi(base[:6])
			if e == nil && n > max {
				max = n
			}
		}
	}
	version := max + 1
	up := filepath.Join(dir, fmt.Sprintf("%06d_%s.up.sql", version, name))
	down := filepath.Join(dir, fmt.Sprintf("%06d_%s.down.sql", version, name))
	if _, err := os.Stat(up); err == nil {
		return fmt.Errorf("migration already exists: %s", up)
	}
	if err := os.WriteFile(up, []byte("-- Write the up migration here.\n"), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(down, []byte("-- Write the matching down migration here.\n"), 0644); err != nil {
		_ = os.Remove(up)
		return err
	}
	fmt.Fprintf(c.OutOrStdout(), "Created %s and %s\n", up, down)
	return nil
}
