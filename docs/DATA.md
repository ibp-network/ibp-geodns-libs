# data - Primary Data Management Library

## Overview
The data package manages monitoring results, usage statistics, and event recording for the IBP GeoDNS system. It maintains both official (consensus-based) and local monitoring results with automatic MySQL persistence.

## Key Features
- Dual-store architecture (Official/Local results)
- In-memory usage aggregation with 5-minute flushes
- Event recording for member outages
- JSON cache persistence for recovery
- Thread-safe concurrent access
- IPv4/IPv6 aware monitoring

## Architecture

### Data Stores
1. **Official Results** - Consensus-validated monitoring data
2. **Local Results** - Node-specific monitoring observations  
3. **Usage Statistics** - In-memory DNS hit aggregation
4. **Event Records** - MySQL-persisted outage tracking

### Initialization
```go
Init(opts InitOptions)
// Options:
// - UseLocalOfficialCaches: Enable cache file persistence
// - UseUsageStats: Enable usage tracking
```

## Core Data Types

### Result Structures
```go
type Result struct {
    Member    cfg.Member              // Provider details
    Status    bool                    // Online/offline
    Checktime time.Time              // Check timestamp
    ErrorText string                  // Error details
    Data      map[string]interface{} // Additional metadata
    IsIPv6    bool                   // IPv6 check flag
}

type SiteResult struct {
    Check   cfg.Check  // Check configuration
    IsIPv6  bool       // IPv6 flag
    Results []Result   // Per-member results
}

type DomainResult struct {
    Check   cfg.Check   // Check configuration
    Service cfg.Service // Service details
    Domain  string      // Domain name
    IsIPv6  bool        // IPv6 flag
    Results []Result    // Per-member results
}

type EndpointResult struct {
    Check    cfg.Check   // Check configuration
    Service  cfg.Service // Service details
    RpcUrl   string      // Endpoint URL
    Domain   string      // Domain name
    IsIPv6   bool        // IPv6 flag
    Results  []Result    // Per-member results
}
```

## Result Management

### Official Results (Consensus)
Functions for consensus-validated data:
- `GetOfficialResults()` - Retrieve all official results
- `SetOfficialSnapshot(snap Snapshot)` - Atomic snapshot update
- `UpdateOfficialSiteResult()` - Update site-level status
- `UpdateOfficialDomainResult()` - Update domain-level status
- `UpdateOfficialEndpointResult()` - Update endpoint-level status
- `GetOfficialSiteStatus()` - Check site status for member
- `GetOfficialDomainStatus()` - Check domain status
- `GetOfficialEndpointStatus()` - Check endpoint status

### Local Results (Node-specific)
Functions for local observations:
- `GetLocalResults()` - Retrieve all local results
- `UpdateLocalSiteResult()` - Update local site observation
- `UpdateLocalDomainResult()` - Update local domain observation
- `UpdateLocalEndpointResult()` - Update local endpoint observation
- `GetLocalSiteStatusIPv4v6()` - Check local site status
- `GetLocalDomainStatusIPv4v6()` - Check local domain status
- `GetLocalEndpointStatusIPv4v6()` - Check local endpoint status

## Usage Statistics

### Recording DNS Hits
```go
RecordDnsHit(isIPv6 bool, clientIP, domain, memberName string)
```
- Increments in-memory counter
- Groups by date/domain/member/country/ASN
- Non-blocking operation

### Usage Aggregation Keys
- Date (YYYY-MM-DD)
- Domain name
- Member name
- Country code
- ASN (Autonomous System Number)
- Network name
- Country name

### Automatic Flushing
- Every 5 minutes via background goroutine
- On-demand via `FlushUsageToDatabase(date string)`
- Atomic database upserts

## Event Recording

### Event Types
1. **Site Events** - Infrastructure-wide outages
2. **Domain Events** - Service-specific issues
3. **Endpoint Events** - Individual endpoint failures

### Event Lifecycle
```go
RecordEvent(checkType, checkName, memberName, domainName, 
           endpoint string, status bool, errorText string,
           data map[string]interface{}, isIPv6 bool)
```

#### Offline Event Creation
- Creates new event record on first failure
- Ignores short flaps (<30 seconds)
- Stores error details and metadata

#### Online Event Closure
- Finds open offline event
- Updates end_time
- Calculates downtime duration

### Event Retrieval
```go
GetMemberEvents(memberName, domain string, start, end time.Time) ([]EventRecord, error)
```

## Cache Management

### Cache Files
- `official.cache.json` - Official results backup
- `local.cache.json` - Local results backup

### Cache Operations
- `LoadAllCaches()` - Restore from disk on startup
- `SaveAllCaches()` - Persist to disk (90-second interval)
- Thread-safe with mutex protection

## Member Management

### Override Functions
```go
MemberEnable(name string)  // Clear override flag
MemberDisable(name string) // Set override flag
```
- Updates configuration
- Records state change events
- Affects routing decisions

### Online Status Checks
```go
IsMemberOnlineForDomain(domain, memberName string) bool       // IPv4
IsMemberOnlineForDomainIPv6(domain, memberName string) bool  // IPv6
```
- Checks official results hierarchically
- Site → Domain → Endpoint precedence
- Override flag consideration

## MySQL Schema

### requests Table
```sql
CREATE TABLE requests (
    date DATE,
    domain_name VARCHAR(255),
    member_name VARCHAR(255),
    country_code CHAR(2),
    network_asn VARCHAR(20),
    network_name VARCHAR(255),
    country_name VARCHAR(255),
    is_ipv6 CHAR(1),
    hits INT,
    KEY idx_date_domain (date, domain_name)
);
```

### member_events Table
```sql
CREATE TABLE member_events (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    member_name VARCHAR(255),
    check_type VARCHAR(50),
    check_name VARCHAR(100),
    domain_name VARCHAR(255),
    endpoint TEXT,
    status BOOLEAN,
    start_time TIMESTAMP,
    end_time TIMESTAMP NULL,
    error TEXT,
    additional_data JSON,
    is_ipv6 BOOLEAN,
    KEY idx_member_time (member_name, start_time)
);
```

## Usage Examples

### Initialize with Usage Stats
```go
import dat "github.com/ibp-network/ibp-geodns-libs/data"

dat.Init(dat.InitOptions{
    UseLocalOfficialCaches: true,
    UseUsageStats: true,
})
```

### Record DNS Hit
```go
dat.RecordDnsHit(
    false,              // IPv4
    "203.0.113.1",     // Client IP
    "rpc.example.com", // Domain
    "provider1",       // Member name
)
```

### Check Member Status
```go
if dat.IsMemberOnlineForDomain("rpc.example.com", "provider1") {
    // Route to this member
}
```

### Update Official Results
```go
dat.UpdateOfficialEndpointResult(
    check,
    member,
    service,
    "rpc.example.com",
    "wss://rpc.example.com/ws",
    false,  // offline
    "Connection timeout",
    nil,    // extra data
    false,  // IPv4
)
```

## Thread Safety
- All shared state protected by sync.RWMutex
- Atomic snapshot updates for official results
- Safe concurrent read/write operations
- Lock-free usage recording (mutex per operation)

## Background Tasks
1. **Cache Persistence** - Every 90 seconds
2. **Usage Flush** - Every 5 minutes
3. Both run as separate goroutines

## Best Practices
1. Always check member override status before routing
2. Use official results for DNS responses
3. Flush usage before date changes to avoid data loss
4. Monitor event table for growing outage records
5. Enable caching for recovery after restarts

## Dependencies
- `github.com/ibp-network/ibp-geodns-libs/config`
- `github.com/ibp-network/ibp-geodns-libs/logging`
- `github.com/ibp-network/ibp-geodns-libs/maxmind`
- `github.com/go-sql-driver/mysql`