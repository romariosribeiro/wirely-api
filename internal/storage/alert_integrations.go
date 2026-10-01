package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"
)

const alertIntegrationsKey = "alert_integrations"

type TelegramAlertConfig struct {
	Enabled     bool     `json:"enabled"`
	HasToken    bool     `json:"hasToken"`
	ChatID      string   `json:"chatId"`
	InstanceIDs []string `json:"instanceIds"`
}

type SMTPAlertConfig struct {
	Enabled     bool     `json:"enabled"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Security    string   `json:"security"`
	Username    string   `json:"username"`
	HasPassword bool     `json:"hasPassword"`
	From        string   `json:"from"`
	Recipient   string   `json:"recipient"`
	InstanceIDs []string `json:"instanceIds"`
}

type AlertIntegrations struct {
	Telegram TelegramAlertConfig `json:"telegram"`
	SMTP     SMTPAlertConfig     `json:"smtp"`
}

type AlertIntegrationsUpdate struct {
	Telegram struct {
		Enabled     bool     `json:"enabled"`
		Token       string   `json:"token"`
		ClearToken  bool     `json:"clearToken"`
		ChatID      string   `json:"chatId"`
		InstanceIDs []string `json:"instanceIds"`
	} `json:"telegram"`
	SMTP struct {
		Enabled       bool     `json:"enabled"`
		Host          string   `json:"host"`
		Port          int      `json:"port"`
		Security      string   `json:"security"`
		Username      string   `json:"username"`
		Password      string   `json:"password"`
		ClearPassword bool     `json:"clearPassword"`
		From          string   `json:"from"`
		Recipient     string   `json:"recipient"`
		InstanceIDs   []string `json:"instanceIds"`
	} `json:"smtp"`
}

type AlertTargets struct {
	Telegram TelegramAlertConfig
	Token    string
	SMTP     SMTPAlertConfig
	Password string
}

type storedAlertIntegrations struct {
	Telegram TelegramAlertConfig `json:"telegram"`
	Token    string              `json:"telegramToken"`
	SMTP     SMTPAlertConfig     `json:"smtp"`
	Password string              `json:"smtpPassword"`
}

func defaultAlertIntegrations() storedAlertIntegrations {
	return storedAlertIntegrations{
		Telegram: TelegramAlertConfig{InstanceIDs: []string{}},
		SMTP:     SMTPAlertConfig{Port: 587, Security: "starttls", InstanceIDs: []string{}},
	}
}

func (s *Store) readStoredAlertIntegrations(ctx context.Context) (storedAlertIntegrations, error) {
	stored := defaultAlertIntegrations()
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", alertIntegrationsKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return stored, nil
	}
	if err != nil {
		return stored, fmt.Errorf("read alert integrations: %w", err)
	}
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return stored, fmt.Errorf("decode alert integrations: %w", err)
	}
	if stored.SMTP.Port == 0 {
		stored.SMTP.Port = 587
	}
	if stored.SMTP.Security == "" {
		stored.SMTP.Security = "starttls"
	}
	return stored, nil
}

func publicAlertIntegrations(stored storedAlertIntegrations) AlertIntegrations {
	if stored.Telegram.InstanceIDs == nil {
		stored.Telegram.InstanceIDs = []string{}
	}
	if stored.SMTP.InstanceIDs == nil {
		stored.SMTP.InstanceIDs = []string{}
	}
	stored.Telegram.HasToken = stored.Token != ""
	stored.SMTP.HasPassword = stored.Password != ""
	return AlertIntegrations{Telegram: stored.Telegram, SMTP: stored.SMTP}
}

func (s *Store) GetAlertIntegrations(ctx context.Context) (AlertIntegrations, error) {
	stored, err := s.readStoredAlertIntegrations(ctx)
	if err != nil {
		return AlertIntegrations{}, err
	}
	return publicAlertIntegrations(stored), nil
}

func (s *Store) SaveAlertIntegrations(ctx context.Context, update AlertIntegrationsUpdate) (AlertIntegrations, error) {
	stored, err := s.readStoredAlertIntegrations(ctx)
	if err != nil {
		return AlertIntegrations{}, err
	}
	allowed, err := s.alertInstanceIDs(ctx)
	if err != nil {
		return AlertIntegrations{}, err
	}
	telegramIDs, err := normalizeAlertInstanceIDs(update.Telegram.InstanceIDs, allowed)
	if err != nil {
		return AlertIntegrations{}, err
	}
	smtpIDs, err := normalizeAlertInstanceIDs(update.SMTP.InstanceIDs, allowed)
	if err != nil {
		return AlertIntegrations{}, err
	}
	stored.Telegram.Enabled = update.Telegram.Enabled
	stored.Telegram.ChatID = strings.TrimSpace(update.Telegram.ChatID)
	stored.Telegram.InstanceIDs = telegramIDs
	if update.Telegram.ClearToken {
		stored.Token = ""
	}
	if token := strings.TrimSpace(update.Telegram.Token); token != "" {
		if len(token) > 256 || strings.ContainsAny(token, " \t\r\n") {
			return AlertIntegrations{}, errors.New("invalid Telegram bot token")
		}
		stored.Token, err = s.encryptInstanceSecret("alerts", "telegram-token", token)
		if err != nil {
			return AlertIntegrations{}, fmt.Errorf("encrypt Telegram token: %w", err)
		}
	}
	stored.SMTP.Enabled = update.SMTP.Enabled
	stored.SMTP.Host = strings.TrimSpace(update.SMTP.Host)
	stored.SMTP.Port = update.SMTP.Port
	if stored.SMTP.Port == 0 {
		stored.SMTP.Port = 587
	}
	stored.SMTP.Security = strings.ToLower(strings.TrimSpace(update.SMTP.Security))
	if stored.SMTP.Security == "" {
		stored.SMTP.Security = "starttls"
	}
	stored.SMTP.Username = strings.TrimSpace(update.SMTP.Username)
	stored.SMTP.From = strings.TrimSpace(update.SMTP.From)
	stored.SMTP.Recipient = strings.TrimSpace(update.SMTP.Recipient)
	stored.SMTP.InstanceIDs = smtpIDs
	if update.SMTP.ClearPassword {
		stored.Password = ""
	}
	if update.SMTP.Password != "" {
		if len(update.SMTP.Password) > 1024 {
			return AlertIntegrations{}, errors.New("SMTP password is too long")
		}
		stored.Password, err = s.encryptInstanceSecret("alerts", "smtp-password", update.SMTP.Password)
		if err != nil {
			return AlertIntegrations{}, fmt.Errorf("encrypt SMTP password: %w", err)
		}
	}
	if err := validateAlertIntegrations(stored); err != nil {
		return AlertIntegrations{}, err
	}
	payload, err := json.Marshal(stored)
	if err != nil {
		return AlertIntegrations{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, alertIntegrationsKey, string(payload), time.Now().UTC().Unix())
	if err != nil {
		return AlertIntegrations{}, fmt.Errorf("save alert integrations: %w", err)
	}
	return publicAlertIntegrations(stored), nil
}

func validateAlertIntegrations(stored storedAlertIntegrations) error {
	if stored.Telegram.Enabled {
		if stored.Token == "" || stored.Telegram.ChatID == "" || len(stored.Telegram.InstanceIDs) == 0 {
			return errors.New("Telegram requires a bot token, chat ID and at least one instance")
		}
	}
	if stored.SMTP.Port < 1 || stored.SMTP.Port > 65535 {
		return errors.New("SMTP port must be between 1 and 65535")
	}
	if stored.SMTP.Security != "starttls" && stored.SMTP.Security != "tls" && stored.SMTP.Security != "none" {
		return errors.New("SMTP security must be starttls, tls or none")
	}
	if stored.SMTP.Security == "none" && (stored.SMTP.Username != "" || stored.Password != "") {
		return errors.New("SMTP authentication requires TLS or STARTTLS")
	}
	if stored.SMTP.Enabled {
		if stored.SMTP.Host == "" || stored.SMTP.From == "" || stored.SMTP.Recipient == "" || len(stored.SMTP.InstanceIDs) == 0 {
			return errors.New("SMTP requires server, sender, recipient and at least one instance")
		}
		if (stored.SMTP.Username == "") != (stored.Password == "") {
			return errors.New("SMTP username and password must be provided together")
		}
		if _, err := mail.ParseAddress(stored.SMTP.From); err != nil {
			return errors.New("invalid SMTP sender address")
		}
		if _, err := mail.ParseAddress(stored.SMTP.Recipient); err != nil {
			return errors.New("invalid SMTP recipient address")
		}
	}
	return nil
}

func (s *Store) AlertTargets(ctx context.Context) (AlertTargets, error) {
	stored, err := s.readStoredAlertIntegrations(ctx)
	if err != nil {
		return AlertTargets{}, err
	}
	targets := AlertTargets{Telegram: stored.Telegram, SMTP: stored.SMTP}
	if stored.Token != "" {
		targets.Token, err = s.decryptInstanceSecret("alerts", "telegram-token", stored.Token)
		if err != nil {
			return AlertTargets{}, fmt.Errorf("decrypt Telegram token: %w", err)
		}
	}
	if stored.Password != "" {
		targets.Password, err = s.decryptInstanceSecret("alerts", "smtp-password", stored.Password)
		if err != nil {
			return AlertTargets{}, fmt.Errorf("decrypt SMTP password: %w", err)
		}
	}
	return targets, nil
}

func (s *Store) alertInstanceIDs(ctx context.Context) (map[string]bool, error) {
	instances, err := s.ListInstances(ctx)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(instances))
	for _, instance := range instances {
		allowed[instance.ID] = true
	}
	return allowed, nil
}

func normalizeAlertInstanceIDs(values []string, allowed map[string]bool) ([]string, error) {
	unique := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !allowed[value] {
			continue
		}
		unique[value] = true
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}
