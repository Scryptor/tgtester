package sender

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"SendMsgTestForTG/internal/config"
	"SendMsgTestForTG/internal/telegram"
)

// Sender управляет отправкой сообщений
type Sender struct {
	config  *config.Config
	client  *telegram.Client
	logChan chan<- LogEntry
}

// LogEntry представляет запись лога
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// NewSender создает новый отправитель
func NewSender(cfg *config.Config, client *telegram.Client, logChan chan<- LogEntry) *Sender {
	return &Sender{
		config:  cfg,
		client:  client,
		logChan: logChan,
	}
}

// Start запускает процесс отправки сообщений
func (s *Sender) Start(ctx context.Context) {
	s.log("info", "========== ЗАПУСК ОТПРАВКИ ==========")
	s.log("info", fmt.Sprintf("Конфигурация: Таймаут=%v, Интервал=%v", s.config.Timeout, s.config.Interval))
	s.log("info", fmt.Sprintf("Chat ID: %s", s.config.ChatID))
	s.log("info", fmt.Sprintf("Прокси: %s", func() string {
		if s.config.ProxyURL == "" {
			return "не используется"
		}
		return s.config.ProxyURL
	}()))

	requestNum := 0
	for {
		requestNum++
		requestStart := time.Now()

		s.log("info", fmt.Sprintf("---------- Запрос #%d ----------", requestNum))
		s.log("info", fmt.Sprintf("Время начала: %s", requestStart.Format("15:04:05.000")))

		workerCtx, workerCancel := context.WithTimeout(ctx, s.config.Timeout)
		s.log("info", fmt.Sprintf("Контекст создан с таймаутом %v", s.config.Timeout))

		text := s.generateMessage()
		s.log("info", fmt.Sprintf("Сообщение сгенерировано (%d байт)", len(text)))

		err := s.client.SendMessage(workerCtx, s.config.ChatID, s.config.BotToken, s.config.MessageThreadID, text)
		workerCancel()

		requestDuration := time.Since(requestStart)
		if err != nil {
			s.log("error", fmt.Sprintf("РЕЗУЛЬТАТ #%d: ОШИБКА за %v", requestNum, requestDuration))
			s.log("error", fmt.Sprintf("Детали ошибки: %v", err))

			// Проверяем тип ошибки
			if ctx.Err() != nil {
				s.log("error", fmt.Sprintf("Контекст родителя: %v", ctx.Err()))
			}
		} else {
			s.log("info", fmt.Sprintf("РЕЗУЛЬТАТ #%d: УСПЕХ за %v", requestNum, requestDuration))
		}

		// Вычисляем, сколько времени нужно подождать до следующего запроса
		elapsed := time.Since(requestStart)
		if elapsed < s.config.Interval {
			sleepDuration := s.config.Interval - elapsed
			s.log("info", fmt.Sprintf("Ожидание %v до следующего запроса...", sleepDuration))
			select {
			case <-ctx.Done():
				s.log("info", "Получен сигнал остановки")
				return
			case <-time.After(sleepDuration):
			}
		} else {
			s.log("warn", fmt.Sprintf("Запрос занял больше интервала (%v > %v), следующий запрос сразу", elapsed, s.config.Interval))
			// Проверяем контекст даже если не ждём
			select {
			case <-ctx.Done():
				s.log("info", "Получен сигнал остановки")
				return
			default:
			}
		}
	}
}

// generateMessage генерирует тестовое сообщение
func (s *Sender) generateMessage() string {
	return fmt.Sprintf(
		"*iPhone %d, %d ГБ*\n💵 *%d %d  ₽*  ⭐️ *0\\.0* *\\(0\\)*\nhttps://www\\.avito\\.ru/79051%d",
		rand.Intn(20),
		rand.Intn(512),
		rand.Intn(30),
		100+rand.Intn(899),
		10000+rand.Intn(38000),
	)
}

// log отправляет запись в канал логов
func (s *Sender) log(level, message string) {
	select {
	case s.logChan <- LogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: message,
	}:
	default:
		// Если канал переполнен, пропускаем запись
	}
}

