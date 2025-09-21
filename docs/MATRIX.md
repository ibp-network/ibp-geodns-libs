# matrix - Matrix Alerting Library

## Overview
The matrix package provides real-time alerting via the Matrix protocol with deduplication, in-place editing, and member mentions for the IBP GeoDNS monitoring system.

## Key Features
- Deduplicated OFFLINE alerts (one per outage)
- In-place message editing for status updates
- Member @mentions from configuration
- Automatic reconnection handling
- HTML-formatted messages
- Thread-safe operations

## Architecture

### Core Components
- **Matrix Client** - Authenticated mautrix client
- **Offline Map** - Tracks active outages for deduplication
- **Message Formatter** - Creates plain/HTML message pairs
- **Edit Handler** - Updates existing messages in-place

### State Management
```go
var (
    client     *mautrix.Client  // Matrix client instance
    userID     id.UserID        // Bot user ID
    roomID     id.RoomID        // Target room
    offlineMap sync.Map         // outage-key → EventID
)
```

## Initialization

### Init() Function
- Single initialization via sync.Once
- Validates configuration completeness
- Authenticates with Matrix homeserver
- Sets up client credentials
- Configures target room

### Required Configuration
```go
type MatrixConfig struct {
    HomeServerURL string  // Matrix server URL
    Username      string  // Bot username
    Password      string  // Bot password
    RoomID        string  // Target room ID
}
```

## Alert Functions

### NotifyMemberOffline
```go
NotifyMemberOffline(
    member, checkType, checkName, domain, endpoint string,
    ipv6 bool, errText string,
)
```
- Creates single alert per unique outage
- Deduplicates multiple reports
- Includes member mentions
- Stores EventID for later editing

### NotifyMemberOnline
```go
NotifyMemberOnline(
    member, checkType, checkName, domain, endpoint string,
    ipv6 bool,
)
```
- Edits existing OFFLINE message to ONLINE
- Falls back to new message if edit fails
- Removes entry from offline map

## Deduplication System

### Outage Key Generation
```go
key := fmt.Sprintf("%s|%s|%s|%s|%s|%v",
    member, checkType, checkName, domain, endpoint, ipv6)
```

### Deduplication Logic
1. Check if outage already announced
2. Use LoadOrStore with sentinel value
3. Prevent concurrent duplicate alerts
4. Store EventID for future editing

## Message Formatting

### Alert Structure
```
@member1 @member2
⚠️ *OFFLINE* / ✅ *ONLINE*
• Member: **provider1**
• Check: site / ping
• Domain: rpc.example.com
• Endpoint: wss://rpc.example.com/ws
• IPv6: false
• Error: Connection timeout [offline only]
```

### HTML Formatting
- Bold for status and member name
- Line breaks with `<br/>`
- Proper mention rendering
- Clean visual hierarchy

## Member Mentions

### Configuration Format
```json
{
    "matrix": {
        "room": "!roomid:server.com",
        "members": {
            "provider1": ["@user1:server.com", "@user2:server.com"],
            "provider2": ["@user3:server.com"]
        }
    }
}
```

### Mention Resolution
- Case-insensitive member lookup
- Multiple users per member
- Only included in OFFLINE alerts
- No mentions for recovery

## Message Operations

### Send Operation
```go
sendFormattedText(ctx context.Context, body, formattedBody string)
```
- Posts HTML-formatted message
- Returns EventID for tracking
- 10-second timeout

### Edit Operation
```go
editFormattedText(ctx context.Context, target id.EventID, body, formattedBody string)
```
- In-place message replacement
- Preserves message history
- Uses Matrix replace relation

## Error Handling

### Connection Failures
- Graceful degradation
- No blocking of caller
- Logged but not propagated

### Edit Failures
- Automatic fallback to new message
- Ensures alert always delivered
- Logged for troubleshooting

## Thread Safety
- sync.Map for offline tracking
- sync.Once for initialization
- Concurrent-safe operations
- No blocking on alert calls

## Usage Examples

### Basic Alert Flow
```go
// Import
import "github.com/ibp-network/ibp-geodns-libs/matrix"

// Initialize once
matrix.Init()

// Send offline alert
matrix.NotifyMemberOffline(
    "provider1",
    "endpoint",
    "wss",
    "rpc.example.com",
    "wss://rpc.example.com/ws",
    false,
    "Connection refused",
)

// Later, send recovery
matrix.NotifyMemberOnline(
    "provider1",
    "endpoint",
    "wss",
    "rpc.example.com",
    "wss://rpc.example.com/ws",
    false,
)
```

### Configuration Example
```json
{
    "Matrix": {
        "HomeServerURL": "https://matrix.example.com",
        "Username": "geodns-bot",
        "Password": "secure-password",
        "RoomID": "!abc123:example.com"
    }
}
```

## Best Practices

1. **Initialize early** in application startup
2. **Check isReady()** before operations
3. **Don't block** on notification calls
4. **Monitor logs** for edit failures
5. **Test mentions** configuration

## Performance
- Non-blocking notification calls
- 10-second timeout on operations
- Minimal memory for offline tracking
- Efficient deduplication

## Dependencies
- `maunium.net/go/mautrix` - Matrix client library
- `github.com/ibp-network/ibp-geodns-libs/config`
- `github.com/ibp-network/ibp-geodns-libs/logging`