package config

import (
	"time"
)

// Config содержит все настройки приложения
type Config struct {
	ProxyURL         string        `json:"proxyURL"`
	Timeout          time.Duration `json:"timeout"`
	Interval         time.Duration `json:"interval"`
	ChatID           string        `json:"chatID"`
	BotToken         string        `json:"botToken"`
	MessageThreadID  string        `json:"messageThreadID"`
	DisableKeepAlive bool          `json:"disableKeepAlive"`
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

