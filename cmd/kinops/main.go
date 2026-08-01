package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dcrespo1/kinops/internal/app"
	"github.com/dcrespo1/kinops/internal/auth"
	"github.com/dcrespo1/kinops/internal/config"
	"github.com/dcrespo1/kinops/internal/database"
	"github.com/dcrespo1/kinops/internal/service"
	"github.com/dcrespo1/kinops/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) == 2 && os.Args[1] == "hash-password" {
		if err := runPasswordHash(); err != nil {
			logger.Error("hash password", "error", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			logger.Error("healthcheck failed", "error", err)
			os.Exit(1)
		}
		return
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		logger.Error("apply migrations", "error", err)
		os.Exit(1)
	}

	managementService := service.New(store.NewSQLite(db), cfg.Location)
	if err := managementService.EnsureHorizon(context.Background()); err != nil {
		logger.Error("ensure scheduling horizon", "error", err)
	}
	if err := managementService.EnsureEventHorizon(context.Background()); err != nil {
		logger.Error("ensure event horizon", "error", err)
	}

	var adminAuth *auth.Manager
	if cfg.AdminEnabled() {
		adminAuth, err = auth.NewManager(cfg.AdminUsername, cfg.AdminPasswordHash, cfg.AdminCookieSecure)
		if err != nil {
			logger.Error("configure admin authentication", "error", err)
			os.Exit(1)
		}
	}

	router := app.NewRouter(app.Dependencies{
		DB:                  db,
		Logger:              logger,
		Config:              cfg,
		ManagementService:   managementService,
		DailyService:        managementService,
		CalendarService:     managementService,
		CalendarFeedService: managementService,
		AdminService:        managementService,
		AdminAuth:           adminAuth,
	})

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	go maintainHorizon(ctx, managementService, cfg.Location, logger)

	go func() {
		logger.Info("starting KinOps server", "address", cfg.ListenAddress)

		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shut down server", "error", err)
		os.Exit(1)
	}
}

func runPasswordHash() error {
	return writePasswordHash(os.Stdin, os.Stdout)
}

func writePasswordHash(input io.Reader, output io.Writer) error {
	data, err := bufio.NewReader(io.LimitReader(input, 1025)).ReadString('\n')
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return fmt.Errorf("read password from standard input: %w", err)
		}
	}
	if len(data) > 1024 {
		return errors.New("password must be 1024 bytes or fewer")
	}
	password := strings.TrimSuffix(strings.TrimSuffix(data, "\n"), "\r")
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, hash)
	return err
}

func runHealthcheck() error {
	address := os.Getenv("KINOPS_LISTEN_ADDRESS")
	if address == "" {
		address = ":8081"
	}
	if strings.HasPrefix(address, ":") {
		address = "127.0.0.1" + address
	}
	client := &http.Client{Timeout: 3 * time.Second}
	return checkHealth(client, address)
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func checkHealth(client httpDoer, address string) error {
	request, err := http.NewRequest(http.MethodGet, "http://"+address+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request health endpoint: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %s", response.Status)
	}
	return nil
}

func maintainHorizon(ctx context.Context, scheduler *service.Service, location *time.Location, logger *slog.Logger) {
	for {
		now := time.Now().In(location)
		next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			if err := scheduler.EnsureHorizon(ctx); err != nil {
				logger.Error("ensure daily scheduling horizon", "error", err)
			}
			if err := scheduler.EnsureEventHorizon(ctx); err != nil {
				logger.Error("ensure daily event horizon", "error", err)
			}
		}
	}
}
