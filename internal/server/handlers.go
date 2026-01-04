package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"SendMsgTestForTG/internal/config"
	"SendMsgTestForTG/internal/sender"
	"SendMsgTestForTG/internal/telegram"
)

// Server представляет HTTP сервер
type Server struct {
	mu          sync.RWMutex
	config      *config.Config
	sender      *sender.Sender
	senderCtx   context.Context
	senderCancel context.CancelFunc
	logChan     chan sender.LogEntry
	subscribers map[chan sender.LogEntry]bool
	subMu       sync.RWMutex
}

// NewServer создает новый HTTP сервер
func NewServer() *Server {
	logChan := make(chan sender.LogEntry, 100)
	return &Server{
		config:      config.Default(),
		logChan:     logChan,
		subscribers: make(map[chan sender.LogEntry]bool),
	}
}

// GetConfig возвращает текущую конфигурацию
func (s *Server) GetConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.config); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// UpdateConfig обновляет конфигурацию
func (s *Server) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var newConfig config.Config
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, fmt.Sprintf("Ошибка декодирования JSON: %v", err), http.StatusBadRequest)
		return
	}

	if err := newConfig.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.config = &newConfig
	s.mu.Unlock()

	s.log("info", "Конфигурация обновлена")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Start запускает отправку сообщений
func (s *Server) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.senderCancel != nil {
		http.Error(w, "Отправка уже запущена", http.StatusBadRequest)
		return
	}

	if err := s.config.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Создаём функцию логирования для клиента
	logFunc := func(level, message string) {
		s.log(level, message)
	}

	client, err := telegram.NewClient(s.config.Timeout, s.config.ProxyURL, s.config.DisableKeepAlive, logFunc)
	if err != nil {
		http.Error(w, fmt.Sprintf("Ошибка создания клиента: %v", err), http.StatusInternalServerError)
		return
	}

	s.senderCtx, s.senderCancel = context.WithCancel(context.Background())
	s.sender = sender.NewSender(s.config, client, s.logChan)

	go s.sender.Start(s.senderCtx)

	s.log("info", "Отправка запущена")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// Stop останавливает отправку сообщений
func (s *Server) Stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.senderCancel == nil {
		http.Error(w, "Отправка не запущена", http.StatusBadRequest)
		return
	}

	s.senderCancel()
	s.senderCancel = nil
	s.sender = nil

	s.log("info", "Отправка остановлена")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

// GetStatus возвращает статус отправки
func (s *Server) GetStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	isRunning := s.senderCancel != nil
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"running": isRunning,
	})
}

// LogsSSE отправляет логи через Server-Sent Events
func (s *Server) LogsSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	subChan := make(chan sender.LogEntry, 10)
	s.subMu.Lock()
	s.subscribers[subChan] = true
	s.subMu.Unlock()

	defer func() {
		s.subMu.Lock()
		delete(s.subscribers, subChan)
		close(subChan)
		s.subMu.Unlock()
	}()

	ctx := r.Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, "data: {\"type\":\"ping\"}\n\n")
			w.(http.Flusher).Flush()
		case logEntry := <-subChan:
			data, err := json.Marshal(logEntry)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
		}
	}
}

// log отправляет запись в канал логов (broadcaster разошлёт подписчикам)
func (s *Server) log(level, message string) {
	entry := sender.LogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: message,
	}

	select {
	case s.logChan <- entry:
	default:
		// Канал переполнен, пропускаем
	}
}

// StartLogBroadcaster запускает широковещатель логов
func (s *Server) StartLogBroadcaster() {
	go func() {
		for entry := range s.logChan {
			s.subMu.RLock()
			for subChan := range s.subscribers {
				select {
				case subChan <- entry:
				default:
				}
			}
			s.subMu.RUnlock()
		}
	}()
}

