# logging - Structured Logging Library

## Overview
The logging package provides a lightweight, leveled logging system for the IBP GeoDNS infrastructure with UTC timestamps and configurable verbosity.

## Key Features
- Five log levels: Debug, Info, Warn, Error, Fatal
- UTC timestamp formatting
- Thread-safe operations
- Configurable runtime log level
- Printf-style formatting
- Automatic initialization

## Log Levels

### Level Hierarchy
```go
const (
    Debug LogLevel = iota  // 0 - Verbose debugging
    Info                   // 1 - Informational messages
    Warn                   // 2 - Warning conditions
    Error                  // 3 - Error conditions
    Fatal                  // 4 - Fatal errors (program termination)
)
```

Messages are only printed if their level >= configured minimum level.

## Core Functions

### Log(level LogLevel, format string, v ...interface{})
Primary logging function with level filtering.
```go
log.Log(log.Info, "Connected to %s on port %d", host, port)
// Output: 2024-01-15 14:30:00 INFO: Connected to localhost on port 3306
```

### SetLogLevel(level LogLevel)
Dynamically adjust minimum log level.
```go
log.SetLogLevel(log.Debug)  // Enable all messages
log.SetLogLevel(log.Error)  // Only errors and fatal
```

### ParseLogLevel(levelStr string) LogLevel
Convert string configuration to log level.
```go
level := log.ParseLogLevel("debug")  // Returns Debug
level := log.ParseLogLevel("INFO")   // Case-insensitive
```

### Fmt(format string, v ...interface{}) error
Convenience wrapper for formatted error creation.
```go
return log.Fmt("failed to connect to %s: %w", url, err)
```

## Output Format
```
YYYY-MM-DD HH:MM:SS LEVEL: Message
```
- Timestamps in UTC
- Level name in uppercase
- Message with printf formatting

## Usage Examples

### Basic Logging
```go
import log "github.com/ibp-network/ibp-geodns-libs/logging"

// Simple message
log.Log(log.Info, "Service started")

// With formatting
log.Log(log.Debug, "Processing %d records", count)

// Error with context
log.Log(log.Error, "Database query failed: %v", err)
```

### Conditional Logging
```go
if err != nil {
    log.Log(log.Error, "Operation failed: %v", err)
    if critical {
        log.Log(log.Fatal, "Cannot continue: %v", err)
        os.Exit(1)
    }
}
```

### Dynamic Level Configuration
```go
// From config file
config := loadConfig()
level := log.ParseLogLevel(config.LogLevel)
log.SetLogLevel(level)

// Runtime adjustment for debugging
if debugMode {
    log.SetLogLevel(log.Debug)
}
```

## Implementation Details

### Initialization
- Automatic on package import
- Creates logger with stdout output
- Default level: Info
- Includes file location flags

### Thread Safety
- Global logger instance
- Mutex protection on level changes
- Safe for concurrent logging

## Best Practices

1. **Use appropriate levels**:
   - Debug: Detailed flow, variable values
   - Info: Normal operations, milestones
   - Warn: Recoverable issues, degraded performance
   - Error: Failures requiring attention
   - Fatal: Unrecoverable errors

2. **Include context** in error messages:
   ```go
   log.Log(log.Error, "Failed to update member %s: %v", memberName, err)
   ```

3. **Avoid excessive debug logging** in production:
   ```go
   if log.ParseLogLevel(config.LogLevel) == log.Debug {
       log.Log(log.Debug, "Detailed state: %+v", state)
   }
   ```

4. **Use Fatal sparingly** - only for startup failures

## Configuration

### From JSON Config
```json
{
    "System": {
        "LogLevel": "info"
    }
}
```

### Environment-based
```go
if os.Getenv("DEBUG") == "true" {
    log.SetLogLevel(log.Debug)
}
```

## Performance Considerations
- Level check before formatting
- Minimal overhead for filtered messages
- No buffering (immediate output)
- Printf-style formatting cost

## Output Destinations
- Default: stdout
- Timestamps: UTC with date and time
- Format: Text (not JSON)

## Dependencies
- Standard library only (fmt, log, os, strings)
- Zero external dependencies