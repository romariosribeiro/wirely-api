package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/backup"
	"github.com/romariosribeiro/wirely-api/internal/buildinfo"
	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/outbox"
	"github.com/romariosribeiro/wirely-api/internal/server"
	"github.com/romariosribeiro/wirely-api/internal/storage"
	"github.com/romariosribeiro/wirely-api/internal/updater"
	"github.com/romariosribeiro/wirely-api/internal/webhook"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	address := envOrDefault("WIRELY_ADDRESS", ":8080")
	dataDirectory := envOrDefault("WIRELY_DATA_DIR", "./data")

	restored, err := backup.ApplyPending(dataDirectory)
	if err != nil {
		fail("failed to apply pending restore", err)
	}
	if restored {
		slog.Info("backup restore applied", "data_directory", dataDirectory)
	}

	store, err := storage.Open(dataDirectory)
	if err != nil {
		fail("failed to open storage", err)
	}
	defer store.Close()

	initialPassword, err := store.EnsureAdmin(context.Background())
	if err != nil {
		fail("failed to ensure owner account", err)
	}
	if initialPassword != "" {
		slog.Warn("initial owner credential generated",
			"username", "admin",
			"initial_password", initialPassword,
			"notice", "save this password now; it will not be displayed again")
	}
	if err := store.PruneSessions(context.Background()); err != nil {
		slog.Error("failed to prune sessions", "error", err)
	}
	if err := store.PruneLoginAttempts(context.Background()); err != nil {
		slog.Error("failed to prune login attempts", "error", err)
	}
	if err := store.PruneActivity(context.Background(), 30*24*time.Hour); err != nil {
		slog.Error("failed to prune activity history", "error", err)
	}

	whatsappManager, err := engine.NewManager(dataDirectory, store)
	if err != nil {
		fail("failed to initialize WhatsApp engine", err)
	}
	defer whatsappManager.Close()
	if removed, pruneErr := whatsappManager.PruneReceivedMedia(time.Now().UTC().Add(-30 * 24 * time.Hour)); pruneErr != nil {
		slog.Error("failed to prune received media", "error", pruneErr)
	} else if removed > 0 {
		slog.Info("old received media removed", "count", removed)
	}
	webhookDispatcher := webhook.NewDispatcher(store)
	if err := webhookDispatcher.Start(); err != nil {
		fail("failed to start webhook delivery queue", err)
	}
	defer webhookDispatcher.Close()
	whatsappManager.SetEventHandler(func(event engine.Event) {
		inserted, saveErr := store.SaveActivityEvent(context.Background(), event.ID, event.InstanceID, event.Event, event.Timestamp, event.Data)
		if saveErr != nil {
			slog.Error("failed to persist event", "event_id", event.ID, "event", event.Event, "instance_id", event.InstanceID, "error", saveErr)
		}
		if saveErr != nil || inserted {
			webhookDispatcher.Dispatch(event)
		}
	})
	if err := whatsappManager.Restore(context.Background()); err != nil {
		slog.Error("failed to restore WhatsApp sessions", "error", err)
	}

	queueRate, err := time.ParseDuration(envOrDefault("WIRELY_QUEUE_RATE", "1s"))
	if err != nil || queueRate < 0 {
		fail("WIRELY_QUEUE_RATE must be a non-negative duration such as 1s or 500ms", err)
	}
	messageQueue, err := outbox.New(dataDirectory, store, whatsappManager, queueRate)
	if err != nil {
		fail("failed to initialize message queue", err)
	}
	if err := messageQueue.Start(); err != nil {
		fail("failed to start message queue", err)
	}
	defer messageQueue.Close()

	backupInterval, err := time.ParseDuration(envOrDefault("WIRELY_BACKUP_INTERVAL", "24h"))
	if err != nil || backupInterval < 0 {
		fail("WIRELY_BACKUP_INTERVAL must be a non-negative duration such as 24h", err)
	}
	backupRetention := envPositiveInt("WIRELY_BACKUP_RETENTION", 7)
	backupManager, err := backup.New(dataDirectory, backupRetention, backupInterval)
	if err != nil {
		fail("failed to initialize backups", err)
	}
	backupManager.Start()
	defer backupManager.Close()
	updateManager := updater.New(buildinfo.Version, dataDirectory)

	app := server.New(server.Dependencies{
		Store:         store,
		Engine:        whatsappManager,
		Webhooks:      webhookDispatcher,
		Queue:         messageQueue,
		Backups:       backupManager,
		Updates:       updateManager,
		RateLimit:     envPositiveInt("WIRELY_API_RATE_LIMIT", 120),
		SecureCookies: strings.EqualFold(os.Getenv("WIRELY_SECURE_COOKIE"), "true"),
		Restart: func() {
			slog.Warn("restarting process to apply staged restore")
			os.Exit(75)
		},
	})
	slog.Info("Wirely API listening", "address", address, "backup_interval", backupInterval.String(), "backup_retention", backupRetention)
	if err := app.ListenAndServe(address); err != nil {
		fail("Wirely API stopped", err)
	}
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envPositiveInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		fail(name+" must be a positive integer", err)
	}
	return parsed
}

func fail(message string, err error) {
	if err != nil {
		slog.Error(message, "error", err)
	} else {
		slog.Error(message)
	}
	os.Exit(1)
}
