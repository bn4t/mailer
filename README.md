# Mailer

A simple SMTP mail server written in Go that captures emails and displays them in a web interface. Perfect for development and testing email functionality without sending real emails.

## Features

- **SMTP Server**: Receives emails on port 2500
- **Web Interface**: View captured emails in a clean, modern UI
- **Real-time Updates**: Auto-refreshes every 2 seconds to show new emails
- **Email Management**: View, delete individual emails or clear all emails
- **Multiple Views**: View plain text, HTML, and raw headers
- **In-Memory Storage**: Fast, lightweight storage (emails are lost on restart)

## Project Structure

```
mailer/
├── main.go              # Application entry point
├── go.mod               # Go module definition
├── models/
│   └── email.go        # Email data structures
├── smtp/
│   └── server.go       # SMTP server implementation
├── storage/
│   └── store.go        # In-memory email storage
├── api/
│   └── handlers.go     # HTTP API handlers
└── web/
    └── index.html      # AlpineJS web interface
```

## Quick Start

### Build

```bash
go build -o mailer
```

### Run

```bash
./mailer
```

The application will start two servers:
- **SMTP Server**: `localhost:2500` - Send emails here
- **Web Interface**: `http://localhost:8080` - View emails here

### Command-Line Flags

You can customize the ports using command-line flags:

```bash
./mailer -smtp-port 2525 -http-port 8081
```

Available flags:
- `-smtp-port` - SMTP server port (default: 2500)
- `-http-port` - HTTP server port (default: 8080)
- `-h` - Show help

## Usage

### Sending Test Emails

You can send test emails using various tools:

#### Using `swaks` (recommended)

```bash
swaks --to test@example.com \
      --from sender@example.com \
      --server localhost:2500 \
      --header "Subject: Test Email" \
      --body "This is a test email"
```

#### Using Python

```python
import smtplib
from email.mime.text import MIMEText

msg = MIMEText("This is a test email")
msg['Subject'] = 'Test Email'
msg['From'] = 'sender@example.com'
msg['To'] = 'test@example.com'

with smtplib.SMTP('localhost', 2500) as server:
    server.send_message(msg)
```

#### Using `telnet`

```bash
telnet localhost 2500
EHLO localhost
MAIL FROM:<sender@example.com>
RCPT TO:<test@example.com>
DATA
Subject: Test Email

This is a test email
.
QUIT
```

### Viewing Emails

1. Open `http://localhost:8080` in your browser
2. Emails will appear in the left panel as they arrive
3. Click on an email to view its details
4. Switch between Plain Text, HTML, and Headers tabs
5. Use the delete buttons to remove emails

## API Endpoints

The application provides a REST API:

- `GET /api/emails` - List all captured emails
- `GET /api/emails/:id` - Get a specific email
- `DELETE /api/emails/:id` - Delete a specific email
- `DELETE /api/emails` - Delete all emails

## Configuration

Ports can be customized using command-line flags (see above). The defaults are:
- SMTP: port 2500
- HTTP: port 8080

## Graceful Shutdown

The application supports graceful shutdown. Press `Ctrl+C` to stop the servers. The application will display the number of emails captured during the session.

## Dependencies

- [github.com/emersion/go-smtp](https://github.com/emersion/go-smtp) - SMTP server library
- [AlpineJS](https://alpinejs.dev/) - Frontend framework (loaded via CDN)

## License

MIT
