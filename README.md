# Channels WebSocket Server

A multi-tenant WebSocket server that supports subdomain-based routing with SQLite persistence.

## Project Structure

```
channels/
├── main.go                          # Application entry point
├── internal/
│   ├── config/                      # Configuration management
│   │   └── config.go
│   ├── database/                    # Database operations and migrations
│   │   └── database.go
│   ├── handlers/                    # HTTP/WebSocket request handlers
│   │   └── handlers.go
│   ├── models/                      # Data models and structures
│   │   └── models.go
│   └── services/                    # Business logic and services
│       └── websocket.go
├── go.mod
└── go.sum
```

## Features

- **Subdomain Support**: Automatically detects subdomains from the Host header
- **SQLite Persistence**: Stores messages and session data in SQLite database
- **Session Management**: Tracks WebSocket sessions with start/end times and message counts
- **Health Checks**: `/health` endpoint for monitoring
- **Graceful Shutdown**: Handles SIGINT/SIGTERM signals
- **Configurable**: Environment-based configuration

## Configuration

Set these environment variables to customize behavior:

- `PORT`: Server port (default: `:8080`)
- `DATABASE_PATH`: SQLite database file path (default: `./channels.db`)
- `MAX_PAYLOAD_SIZE`: Maximum request size in bytes (default: `16MB`)
- `SESSION_TIMEOUT`: WebSocket session timeout (default: `10s`)
- `READ_TIMEOUT`: Message read timeout (default: `1s`)
- `MAX_CONNECTIONS`: Maximum concurrent connections (default: `1000`)

## API Endpoints

### WebSocket Connection

- **URL**: `/` (root path)
- **Protocol**: WebSocket
- **Subdomain Detection**: Automatically extracts subdomain from Host header
- **Session Duration**: 10 seconds (configurable)
- **Response**: JSON with collected messages, timestamp, and duration

### Health Check

- **URL**: `/health`
- **Method**: GET
- **Response**: JSON with service status and timestamp

## Usage

1. **Build and run:**

   ```bash
   go build .
   ./channels.ws
   ```

2. **Connect via WebSocket:**

   ```javascript
   // Connect to subdomain
   const ws = new WebSocket("ws://abc.example.com/");

   // Send messages
   ws.send("Hello from subdomain abc!");

   // Receive response after 10 seconds
   ws.onmessage = (event) => {
     const response = JSON.parse(event.data);
     console.log(response.messages); // All messages sent during session
   };
   ```

3. **Check health:**
   ```bash
   curl http://localhost:8080/health
   ```

## Database Schema

### Sessions Table

- `id`: Primary key
- `subdomain_id`: Extracted subdomain identifier
- `client_ip`: Client IP address
- `start_time`: Session start timestamp
- `end_time`: Session end timestamp
- `duration`: Session duration string
- `message_count`: Total messages in session

### Messages Table

- `id`: Primary key
- `session_id`: Foreign key to sessions table
- `subdomain_id`: Subdomain identifier
- `content`: Message content
- `client_ip`: Client IP address
- `created_at`: Message timestamp

## Development

The codebase is organized following Go best practices:

- `/internal`: Private application code
- Clear separation of concerns (handlers, services, database)
- Environment-based configuration
- Structured logging
- Error handling with context

## Deployment

The application can be deployed as a single binary with minimal dependencies. SQLite provides embedded persistence without requiring external database setup.
