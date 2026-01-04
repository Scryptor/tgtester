# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Build the binary
go build -o SendMsgTestForTG ./cmd/server

# Run in development mode
go run ./cmd/server

# Run with custom port
./SendMsgTestForTG -addr=:3000
```

Default port is 8080. Web interface available at http://localhost:8080

## Architecture Overview

Go web application for testing/debugging Telegram Bot API message sending, with detailed HTTP tracing and real-time log monitoring.

### Layer Structure

- **cmd/server/main.go** - Entry point, sets up HTTP routes and starts the server
- **internal/config/** - Config struct with validation (ChatID, BotToken required)
- **internal/telegram/client.go** - HTTP client with `httptrace` for detailed connection logging (DNS, TCP, TLS, response timing)
- **internal/sender/sender.go** - Message sending loop with configurable intervals, passes log function to client
- **internal/server/handlers.go** - HTTP handlers, SSE log broadcasting, manages sender lifecycle
- **web/static/index.html** - Alpine.js frontend with log filtering, search, export

### Key Patterns

**Logging flow**: telegram.Client receives a LogFunc callback -> writes to Server.logChan -> StartLogBroadcaster distributes to SSE subscribers

**HTTP tracing**: Uses `net/http/httptrace` to log each connection stage (DNSStart/Done, ConnectStart/Done, TLSHandshakeStart/Done, GotFirstResponseByte)

**Sender lifecycle**: Server.Start() creates context + Sender, runs in goroutine. Server.Stop() cancels context.

**Time handling**: Config stores durations in nanoseconds (Go time.Duration). Web UI converts to/from seconds.

**Interval timing**: Next request starts `interval` after the *start* of previous request. If request takes longer, next one starts immediately with a warning log.

### Config Fields

- `ChatID` (required) - Telegram chat/channel ID
- `BotToken` (required) - Bot token
- `MessageThreadID` (optional) - Thread/topic ID for supergroups
- `ProxyURL` (optional) - HTTP or SOCKS5 proxy
- `Timeout` - HTTP client timeout (default 60s)
- `Interval` - Time between requests (default 3s)

### API Endpoints

- `GET /api/config` - Get current configuration
- `POST /api/config/update` - Update configuration (JSON body)
- `POST /api/start` - Start message sending
- `POST /api/stop` - Stop message sending
- `GET /api/status` - Check if sender is running
- `GET /api/logs` - SSE stream for real-time logs
