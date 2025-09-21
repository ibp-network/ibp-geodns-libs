# config - Configuration Management Library

## Overview
The config package provides centralized configuration management for the IBP GeoDNS system, supporting both local JSON configuration and remote configuration fetching with hot-reload capabilities.

## Key Features
- Local JSON configuration loading from disk
- Remote configuration fetching from GitHub URLs
- Automatic periodic reload of remote configurations
- Thread-safe access with read/write mutexes
- Member override preservation during reloads
- Type-safe configuration structures

## Architecture

### Initialization Flow
1. Load local system configuration from JSON file
2. Extract remote config URLs from system config
3. Fetch and parse remote configurations (StaticDNS, Members, Services, etc.)
4. Start background goroutine for periodic config updates
5. Preserve member overrides across reloads

### Configuration Structure
```go
type Config struct {
    Local           LocalConfig            // Node-specific settings
    StaticDNS       []DNSRecord           // Static DNS entries
    Members         map[string]Member     // Infrastructure providers
    Services        map[string]Service    // Service definitions
    Pricing         map[string]IaasPricing // Cost models
    ServiceRequests ServiceRequests       // Request statistics
    Alerts          AlertsConfig          // Notification settings
}
```

## Core Functions

### Init(cfgFile string)
Initializes the configuration system with a local config file path.
- Loads system configuration
- Fetches remote configurations
- Starts auto-updater goroutine
- Thread-safe initialization with sync.Once

### GetConfig() Config
Returns a deep copy of the current configuration.
- Thread-safe with RLock
- Returns complete config snapshot
- Safe for concurrent access

### Member Management
- `GetMember(name string) (Member, bool)` - Retrieve member by name
- `SetMember(name string, member Member)` - Update member data
- `DeleteMember(name string)` - Remove member
- `ListMembers() map[string]Member` - Get all members

## Configuration Types

### LocalConfig
Node-specific settings including:
- System parameters (workdir, log level, reload intervals)
- MaxMind database configuration
- NATS messaging settings
- MySQL database connection
- API endpoint configurations
- Health check worker settings

### Member
Infrastructure provider definition:
```go
type Member struct {
    Details            MemberDetails       // Name, website, logo
    Membership         Membership          // Level, join date
    Service            ServiceInfo         // IPs, monitoring URL
    Override           bool                // Manual override flag
    OverrideTime       time.Time          // Override timestamp
    ServiceAssignments map[string][]string // Service mappings
    Location           Location            // Geographic coordinates
}
```

### Service
Blockchain service configuration:
```go
type Service struct {
    Configuration ServiceConfiguration       // Service metadata
    Resources     Resources                  // Resource requirements
    Providers     map[string]ServiceProvider // RPC endpoints by provider
}
```

## Hot Reload Mechanism

### Reload Process
1. Timer triggers every `ConfigReloadTime` seconds
2. Download remote configs from URLs
3. Parse and validate new configurations
4. **Preserve member Override flags** during update
5. Atomically swap configuration

### Override Preservation
Critical feature ensuring manual overrides survive reloads:
```go
// Preserve existing overrides
for name, existingMember := range cfg.data.Members {
    if existingMember.Override {
        if newMember, exists := newMembers[name]; exists {
            newMember.Override = true
            newMember.OverrideTime = existingMember.OverrideTime
            newMembers[name] = newMember
        }
    }
}
```

## Remote Configuration Sources

### Default URLs (if not specified)
- Alerts: `https://raw.githubusercontent.com/ibp-network/config/refs/heads/main/alerts.json`

### Supported Remote Configs
1. **StaticDNS** - Static DNS record definitions
2. **Members** - Infrastructure provider registry
3. **Services** - Blockchain service catalog
4. **IaasPricing** - Cost calculation models
5. **ServicesRequests** - Usage statistics
6. **Alerts** - Matrix notification settings

## Error Handling

### Initial Load Failures
- Fatal errors terminate the program
- Ensures valid configuration at startup
- Prevents running with incomplete config

### Reload Failures
- Non-fatal during runtime reloads
- Preserves existing configuration
- Logs errors for monitoring

## Thread Safety
- All config access protected by RWMutex
- GetConfig() returns deep copy
- Member operations use exclusive locks
- Safe for concurrent read/write

## Usage Examples

### Basic Initialization
```go
import cfg "github.com/ibp-network/ibp-geodns-libs/config"

func main() {
    cfg.Init("/etc/ibp-geodns/config.json")
    
    config := cfg.GetConfig()
    fmt.Printf("Loaded %d members\n", len(config.Members))
}
```

### Member Override
```go
// Disable a member temporarily
member, exists := cfg.GetMember("provider1")
if exists {
    member.Override = true
    member.OverrideTime = time.Now()
    cfg.SetMember("provider1", member)
}
```

### Access Service Configuration
```go
config := cfg.GetConfig()
for name, service := range config.Services {
    if service.Configuration.Active == 1 {
        fmt.Printf("Active service: %s (Level %d)\n", 
            name, service.Configuration.LevelRequired)
    }
}
```

## Configuration File Format

### Local Config Example
```json
{
    "System": {
        "WorkDir": "/var/lib/ibp-geodns",
        "LogLevel": "info",
        "ConfigReloadTime": 300,
        "ConfigUrls": {
            "StaticDNSConfig": "https://example.com/static-dns.json",
            "MembersConfig": "https://example.com/members.json"
        }
    },
    "Nats": {
        "NodeID": "monitor-us-east-1",
        "Url": "nats://localhost:4222"
    }
}
```

## Best Practices

1. **Always use GetConfig()** for read operations to ensure thread safety
2. **Check member existence** before operations with GetMember()
3. **Preserve Override flags** when updating members programmatically
4. **Monitor reload logs** to detect configuration fetch failures
5. **Use appropriate timeouts** for remote config fetches (15 seconds default)

## Dependencies
- Standard library only (encoding/json, sync, net/http, os, time)
- No external dependencies for core functionality

## Version
Current version: v0.4.0 (via GetVersion())