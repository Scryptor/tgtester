package config

import (
	"time"
)

// Config содержит все настройки приложения
type Config struct {
	ProxyURL        string
	Timeout         time.Duration
	Interval        time.Duration
	ChatID          string
	BotToken        string
	MessageThreadID string
}

// Validate проверяет обязательные поля конфигурации
func (c *Config) Validate() error {
	if c.ChatID == "" {
		return ErrChatIDRequired
	}
	if c.BotToken == "" {
		return ErrBotTokenRequired
	}
	return nil
}

// Default возвращает конфигурацию с значениями по умолчанию
func Default() *Config {
	return &Config{
		Timeout:  60 * time.Second,
		Interval: 3 * time.Second,
	}
}

