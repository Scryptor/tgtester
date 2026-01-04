package telegram

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"
)

// LogFunc тип функции для логирования
type LogFunc func(level, message string)

// Client представляет клиент для работы с Telegram Bot API
type Client struct {
	httpClient *http.Client
	logFunc    LogFunc
	proxyURL   string
}

// NewClient создает новый клиент Telegram
func NewClient(timeout time.Duration, proxyURL string, disableKeepAlive bool, logFunc LogFunc) (*Client, error) {
	// Создаём кастомный dialer с логированием
	baseDialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Оборачиваем dialer для логирования
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		logFunc("info", fmt.Sprintf("🔌 Dialer: начало подключения к %s (%s)", addr, network))
		dialStart := time.Now()

		conn, err := baseDialer.DialContext(ctx, network, addr)
		dialDuration := time.Since(dialStart)

		if err != nil {
			logFunc("error", fmt.Sprintf("🔌 Dialer: ошибка подключения к %s за %v: %v", addr, dialDuration, err))
			return nil, err
		}

		logFunc("info", fmt.Sprintf("🔌 Dialer: подключено к %s за %v (local: %s)", addr, dialDuration, conn.LocalAddr()))
		return conn, nil
	}

	transport := &http.Transport{
		DialContext:           dialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		DisableKeepAlives:     disableKeepAlive,
	}

	if disableKeepAlive {
		logFunc("info", "🔄 Keep-Alive отключён: каждый запрос будет использовать новое соединение")
	}

	if proxyURL != "" {
		parsedProxyURL, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга прокси URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(parsedProxyURL)

		// Добавляем callback для логирования CONNECT запроса к прокси
		transport.OnProxyConnectResponse = func(ctx context.Context, proxyURL *url.URL, connectReq *http.Request, connectRes *http.Response) error {
			logFunc("info", fmt.Sprintf("🔀 Proxy CONNECT: ответ от прокси %s -> статус %d %s",
				proxyURL.Host, connectRes.StatusCode, connectRes.Status))
			if connectRes.StatusCode != 200 {
				logFunc("error", fmt.Sprintf("🔀 Proxy CONNECT: туннель не установлен, код %d", connectRes.StatusCode))
			}
			return nil
		}

		logFunc("info", fmt.Sprintf("🔀 Прокси настроен: %s (схема: %s, хост: %s)", proxyURL, parsedProxyURL.Scheme, parsedProxyURL.Host))
		if parsedProxyURL.User != nil {
			logFunc("info", fmt.Sprintf("🔀 Прокси аутентификация: пользователь '%s'", parsedProxyURL.User.Username()))
		}
	} else {
		logFunc("info", "Прокси не используется (прямое подключение)")
	}

	logFunc("info", fmt.Sprintf("HTTP клиент создан. Timeout: %v, DialTimeout: 30s, TLSHandshake: 15s, ResponseHeader: 30s", timeout))

	return &Client{
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		logFunc:  logFunc,
		proxyURL: proxyURL,
	}, nil
}

// SendMessage отправляет сообщение в Telegram
func (c *Client) SendMessage(ctx context.Context, chatID, botToken, messageThreadID, message string) error {
	data := url.Values{}
	data.Add("chat_id", chatID)
	data.Add("text", message)
	if messageThreadID != "" {
		data.Add("message_thread_id", messageThreadID)
	}
	data.Add("parse_mode", "MarkdownV2")
	data.Add("disable_web_page_preview", "True")

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	c.logFunc("info", fmt.Sprintf("Подготовка запроса к %s", "api.telegram.org"))

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		apiURL,
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		c.logFunc("error", fmt.Sprintf("Ошибка создания запроса: %v", err))
		return fmt.Errorf("создание запроса: %w", err)
	}

	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Добавляем трейсинг для детального логирования
	var (
		getConnStart              time.Time
		dnsStart, dnsDone         time.Time
		connectStart, connectDone time.Time
		tlsStart, tlsDone         time.Time
		reqStart                  time.Time
		gotFirstByte              time.Time
		connReused                bool
		remoteAddr                string
	)

	isProxy := c.proxyURL != ""

	trace := &httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			getConnStart = time.Now()
			if isProxy {
				c.logFunc("info", fmt.Sprintf("📡 GetConn: запрос соединения для %s (через прокси)", hostPort))
			} else {
				c.logFunc("info", fmt.Sprintf("📡 GetConn: запрос соединения для %s", hostPort))
			}
		},
		DNSStart: func(info httptrace.DNSStartInfo) {
			dnsStart = time.Now()
			if isProxy {
				c.logFunc("info", fmt.Sprintf("🔍 DNS lookup начат для прокси: %s", info.Host))
			} else {
				c.logFunc("info", fmt.Sprintf("🔍 DNS lookup начат для: %s", info.Host))
			}
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			dnsDone = time.Now()
			if info.Err != nil {
				c.logFunc("error", fmt.Sprintf("🔍 DNS lookup ошибка: %v (за %v)", info.Err, dnsDone.Sub(dnsStart)))
			} else {
				addrs := make([]string, len(info.Addrs))
				for i, addr := range info.Addrs {
					addrs[i] = addr.String()
				}
				c.logFunc("info", fmt.Sprintf("🔍 DNS lookup завершён за %v. IP: %v", dnsDone.Sub(dnsStart), addrs))
			}
		},
		ConnectStart: func(network, addr string) {
			connectStart = time.Now()
			if isProxy {
				c.logFunc("info", fmt.Sprintf("🔗 TCP соединение с ПРОКСИ начато: %s %s", network, addr))
			} else {
				c.logFunc("info", fmt.Sprintf("🔗 TCP соединение начато: %s %s", network, addr))
			}
		},
		ConnectDone: func(network, addr string, err error) {
			connectDone = time.Now()
			if err != nil {
				if isProxy {
					c.logFunc("error", fmt.Sprintf("🔗 TCP соединение с ПРОКСИ ошибка: %v (за %v)", err, connectDone.Sub(connectStart)))
				} else {
					c.logFunc("error", fmt.Sprintf("🔗 TCP соединение ошибка: %v (за %v)", err, connectDone.Sub(connectStart)))
				}
			} else {
				if isProxy {
					c.logFunc("info", fmt.Sprintf("🔗 TCP соединение с ПРОКСИ установлено за %v", connectDone.Sub(connectStart)))
				} else {
					c.logFunc("info", fmt.Sprintf("🔗 TCP соединение установлено за %v", connectDone.Sub(connectStart)))
				}
			}
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
			if isProxy {
				c.logFunc("info", "🔐 TLS handshake начат (через прокси-туннель к api.telegram.org)")
			} else {
				c.logFunc("info", "🔐 TLS handshake начат")
			}
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			tlsDone = time.Now()
			if err != nil {
				c.logFunc("error", fmt.Sprintf("🔐 TLS handshake ошибка: %v (за %v)", err, tlsDone.Sub(tlsStart)))
			} else {
				c.logFunc("info", fmt.Sprintf("🔐 TLS handshake завершён за %v. Версия: %s, Cipher: %s, ServerName: %s",
					tlsDone.Sub(tlsStart),
					tlsVersionString(state.Version),
					tls.CipherSuiteName(state.CipherSuite),
					state.ServerName))
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			connReused = info.Reused
			remoteAddr = info.Conn.RemoteAddr().String()
			connTime := time.Since(getConnStart)
			if info.Reused {
				c.logFunc("info", fmt.Sprintf("✅ GotConn: переиспользовано соединение к %s (idle: %v, всего: %v)", remoteAddr, info.IdleTime, connTime))
			} else {
				c.logFunc("info", fmt.Sprintf("✅ GotConn: новое соединение к %s (всего: %v)", remoteAddr, connTime))
			}
		},
		WroteHeaders: func() {
			c.logFunc("info", "📤 HTTP заголовки отправлены")
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			reqStart = time.Now()
			if info.Err != nil {
				c.logFunc("error", fmt.Sprintf("📤 Ошибка записи запроса: %v", info.Err))
			} else {
				c.logFunc("info", "📤 Запрос полностью отправлен, ожидание ответа...")
			}
		},
		GotFirstResponseByte: func() {
			gotFirstByte = time.Now()
			c.logFunc("info", fmt.Sprintf("📥 Первый байт ответа получен за %v (TTFB)", gotFirstByte.Sub(reqStart)))
		},
	}

	req = req.WithContext(httptrace.WithClientTrace(ctx, trace))

	c.logFunc("info", "Выполнение HTTP запроса...")
	startTime := time.Now()

	resp, err := c.httpClient.Do(req)
	totalTime := time.Since(startTime)

	if err != nil {
		c.logFunc("error", fmt.Sprintf("HTTP запрос ошибка за %v: %v", totalTime, err))
		// Детализируем тип ошибки
		if ctx.Err() == context.DeadlineExceeded {
			c.logFunc("error", "Причина: превышен таймаут контекста")
		} else if ctx.Err() == context.Canceled {
			c.logFunc("error", "Причина: контекст отменён")
		}
		if urlErr, ok := err.(*url.Error); ok {
			c.logFunc("error", fmt.Sprintf("URL Error детали - Op: %s, Timeout: %v", urlErr.Op, urlErr.Timeout()))
			if urlErr.Unwrap() != nil {
				c.logFunc("error", fmt.Sprintf("Внутренняя ошибка: %v", urlErr.Unwrap()))
			}
		}
		return fmt.Errorf("выполнение запроса: %w", err)
	}
	defer resp.Body.Close()

	c.logFunc("info", fmt.Sprintf("📥 Ответ получен. Статус: %d, Время: %v, ConnReused: %v", resp.StatusCode, totalTime, connReused))

	// Логируем заголовки ответа
	c.logFunc("info", fmt.Sprintf("📥 Response Headers: Content-Length=%s, Content-Type=%s",
		resp.Header.Get("Content-Length"),
		resp.Header.Get("Content-Type")))

	readStart := time.Now()
	body, err := io.ReadAll(resp.Body)
	readTime := time.Since(readStart)

	if err != nil {
		c.logFunc("error", fmt.Sprintf("Ошибка чтения тела ответа за %v: %v", readTime, err))
		return fmt.Errorf("чтение ответа: %w", err)
	}

	c.logFunc("info", fmt.Sprintf("Тело ответа прочитано за %v, размер: %d байт", readTime, len(body)))

	if resp.StatusCode != http.StatusOK {
		c.logFunc("error", fmt.Sprintf("Telegram API ошибка: status=%d, body=%s", resp.StatusCode, string(body)))
		return errors.New(fmt.Sprintf("status is not ok: %d, body: %s", resp.StatusCode, string(body)))
	}

	c.logFunc("info", fmt.Sprintf("Запрос успешен. Общее время: %v", totalTime))
	return nil
}

// tlsVersionString возвращает строковое представление версии TLS
func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (0x%04x)", version)
	}
}

