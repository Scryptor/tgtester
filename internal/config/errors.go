package config

import "errors"

var (
	ErrChatIDRequired   = errors.New("chat ID обязателен для указания")
	ErrBotTokenRequired = errors.New("токен бота обязателен для указания")
)

