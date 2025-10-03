# Mailer

A simple SMTP mail server written in Go that captures emails and displays them in a web interface. Perfect for development and testing email functionality without sending real emails.

## Features

- **SMTP Server**: Receives emails on port 2500
- **IMAP Server**: Access emails via IMAP on port 1143
- **Web Interface**: View captured emails in a clean, modern UI
- **Real-time Updates**: Auto-refreshes every 2 seconds to show new emails
- **Email Management**: View, delete individual emails or clear all emails
- **Multiple Views**: View plain text, HTML, and raw headers
- **MCP Support**: Expose emails to AI assistants via Model Context Protocol
- **In-Memory Storage**: Fast, lightweight storage (emails are lost on restart)

## Project Structure

```
mailer/
├── main.go              # Application entry point with subcommand support
├── go.mod               # Go module definition
├── models/
│   └── email.go        # Email data structures
├── smtp/
│   └── server.go       # SMTP server implementation
├── imap/
│   ├── backend.go      # IMAP backend implementation
│   ├── mailbox.go      # IMAP mailbox implementation
│   └── server.go       # IMAP server
├── storage/
│   └── store.go        # In-memory email storage
├── api/
│   ├── handlers.go     # HTTP API handlers
│   └── web/
│       └── index.html  # AlpineJS web interface
└── mcp/
    └── server.go       # MCP server (HTTP client to daemon)
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

The application will start three servers:
- **SMTP Server**: `localhost:2500` - Send emails here
- **IMAP Server**: `localhost:1143` - Access emails via IMAP
- **Web Interface**: `http://localhost:8080` - View emails here

### Command-Line Flags

You can customize the server addresses using command-line flags:

```bash
./mailer -smtp-addr :2525 -imap-addr :1144 -http-addr 127.0.0.1:8081
```

Available flags:
- `-smtp-addr` - SMTP server bind address (default: `:2500`)
- `-imap-addr` - IMAP server bind address (default: `:1143`)
- `-http-addr` - HTTP server bind address (default: `:8080`)
  - Examples: `:8080` (all interfaces), `127.0.0.1:8080` (localhost only), `192.168.1.5:8080`
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

#### Via Web Interface

1. Open `http://localhost:8080` in your browser
2. Emails will appear in the left panel as they arrive
3. Click on an email to view its details
4. Switch between Plain Text, HTML, and Headers tabs
5. Use the delete buttons to remove emails

#### Via IMAP

You can connect to the mailer using any IMAP client (Thunderbird, Apple Mail, Outlook, etc.):

**IMAP Settings:**
- **Server**: `localhost`
- **Port**: `1143`
- **Username**: Any (e.g., `test@example.com`)
- **Password**: Any (authentication always succeeds for development)
- **Encryption**: None (unencrypted for development)

**Supported IMAP Operations:**
- ✅ List emails (INBOX mailbox)
- ✅ Read email content
- ✅ Delete emails (mark as deleted + expunge)
- ❌ Creating new messages (not supported)
- ❌ Multiple mailboxes (only INBOX available)

**Example using Python:**

```python
import imaplib

# Connect and login
imap = imaplib.IMAP4('localhost', 1143)
imap.login('testuser', 'testpass')

# Select INBOX
imap.select('INBOX')

# Search for all emails
status, messages = imap.search(None, 'ALL')

# Fetch first email
status, msg_data = imap.fetch(b'1', '(RFC822)')

# Delete an email
imap.store(b'1', '+FLAGS', '\\Deleted')
imap.expunge()

# Logout
imap.logout()
```

## API Endpoints

The application provides a REST API:

- `GET /api/emails` - List all captured emails
- `GET /api/emails/:id` - Get a specific email
- `GET /api/config` - Get server configuration (SMTP port, HTTP address)
- `DELETE /api/emails/:id` - Delete a specific email
- `DELETE /api/emails` - Delete all emails

## Model Context Protocol (MCP) Support

Mailer includes an MCP server that allows AI assistants like Claude to directly access captured emails. The MCP server acts as a client to the running mailer daemon, communicating via the HTTP API.

### Architecture

```
Claude Desktop/Code <--(stdio)--> MCP Server <--(HTTP)--> Mailer Daemon (SMTP+Web)
```

### Running the MCP Server

First, start the mailer daemon:

```bash
./mailer server
# or simply
./mailer
```

Then, in a separate terminal, run the MCP server:

```bash
./mailer mcp --api-url http://localhost:8080
```

The MCP server connects to the running daemon and exposes email data through the Model Context Protocol.

### MCP Resources

- `email://list` - List all captured emails in JSON format

### MCP Tools

The MCP server provides the following tools:

- **list_emails** - List all emails with optional filters
  - Optional parameters: `from`, `to`, `subject`
  - Returns: Array of email summaries with count

- **get_email** - Get full details of a specific email
  - Required parameter: `id` (email ID)
  - Returns: Complete email object with body, headers, etc.

- **search_emails** - Search emails by content
  - Required parameter: `query` (search term)
  - Searches in: subject and body fields
  - Returns: Matching emails with count

- **get_stats** - Get email statistics and server info
  - Returns: Total email count, SMTP port, HTTP address

### Claude Desktop Configuration

To use the MCP server with Claude Desktop, add this to your Claude Desktop config file:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`

**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "mailer": {
      "command": "/absolute/path/to/mailer",
      "args": ["mcp", "--api-url", "http://localhost:8080"]
    }
  }
}
```

After adding the configuration, restart Claude Desktop. You can then ask Claude to:
- "List all emails in the mailer"
- "Show me emails from john@example.com"
- "Search for emails containing 'password reset'"
- "Get email with ID 5"

### MCP Command-Line Flags

- `--api-url` - Mailer daemon API URL (default: `http://localhost:8080`)
  - Use this if your daemon is running on a different port or address

## Configuration

Server addresses can be customized using command-line flags (see above). The defaults are:
- SMTP: port 2500 (all interfaces)
- IMAP: port 1143 (all interfaces)
- HTTP: `:8080` (all interfaces on port 8080)

**Note:** The IMAP server uses port 1143 instead of the standard port 143 to avoid requiring root/administrator privileges.

## Graceful Shutdown

The application supports graceful shutdown. Press `Ctrl+C` to stop the servers. The application will display the number of emails captured during the session.

## Dependencies

- [github.com/emersion/go-smtp](https://github.com/emersion/go-smtp) - SMTP server library
- [github.com/emersion/go-imap](https://github.com/emersion/go-imap) - IMAP server library
- [github.com/modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) - MCP SDK for Go
- [AlpineJS](https://alpinejs.dev/) - Frontend framework (loaded via CDN)

## License

MIT
