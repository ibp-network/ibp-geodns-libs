# data2 - Collator Data Management Library

## Overview
The data2 package provides collator-optimized data persistence with idempotent operations, per-node usage tracking, and Matrix alerting integration. Designed specifically for the IBP Collator nodes.

## Key Features
- Idempotent usage upserts (replace, not increment)
- Per-node usage tracking
- Network status management with vote data
- Matrix alert integration
- In-memory proposal caching
- UTC-enforced timestamps

## Core Functionality

### Database Connection
```go
Init()
```
- Establishes MySQL connection with retry logic
- Configures connection pooling (40 max open, 5 idle)
- Forces UTC timezone
- 30-second retry window

### Connection Settings
- Max idle connections: 5
- Max open connections: 40
- Connection lifetime: 4 hours
- Idle timeout: 2 minutes

## Usage Management

### UpsertUsage Function
```go
UpsertUsage(r UsageRecord) error
```
**Critical Design**: Uses `ON DUPLICATE KEY UPDATE hits = VALUES(hits)`
- **Replaces** total hits, does NOT increment
- Idempotent - safe to replay same period
- Primary key: date, node_id, domain_name, member_name, network_asn, network_name, country_code, country_name, is_ipv6

### UsageRecord Structure
```go
type UsageRecord struct {
    Date        time.Time // UTC date
    NodeID      string    // Origin node identifier
    Domain      string    // Service domain
    MemberName  string    // Provider name
    Asn         string    // AS number
    NetworkName string    // Network organization
    CountryCode string    // ISO country code
    CountryName string    // Country name
    IsIPv6      bool      // IPv6 flag
    Hits        int       // Total hits (replaced, not added)
}
```

### Batch Storage
```go
StoreUsageRecords(recs []UsageRecord) error
```
- Processes multiple records
- Continues on individual failures
- Returns aggregated error report

## Network Status Management

### Status Recording
```go
InsertNetStatus(rec NetStatusRecord) error
```
- Records new outages
- Updates existing records
- Triggers Matrix OFFLINE alerts
- Stores vote data as JSON

### Status Resolution
```go
CloseOpenEvent(rec NetStatusRecord) error
```
- Marks outage as resolved
- Sets end_time to UTC_TIMESTAMP()
- Triggers Matrix ONLINE alerts

### NetStatusRecord Structure
```go
type NetStatusRecord struct {
    CheckType int                    // 1=site, 2=domain, 3=endpoint
    CheckName string                  // Check identifier
    CheckURL  string                  // Target URL
    Domain    string                  // Service domain
    Member    string                  // Provider name
    Status    bool                    // Online/offline
    IsIPv6    bool                    // IPv6 check
    StartTime time.Time              // Outage start (UTC)
    EndTime   sql.NullTime           // Resolution time
    Error     string                  // Error details
    VoteData  map[string]bool        // Consensus votes
    Extra     map[string]interface{} // Additional metadata
}
```

## Proposal Management

### In-Memory Cache
```go
CacheProposal(p Proposal)     // Store proposal
PopProposal(id string)         // Retrieve and remove
ExpireStaleProposals()         // Clean old proposals
```

### Expiry Settings
- Default expiry: 10 minutes
- Cleaned by collator janitor service
- Thread-safe with sync.RWMutex

## Matrix Integration

### Alert Triggers
1. **OFFLINE Alert** - Sent on `InsertNetStatus` when status=false
2. **ONLINE Alert** - Sent on `CloseOpenEvent` when outage closes

### Alert Data Passed
- Member name
- Check type (site/domain/endpoint)
- Check name
- Domain
- Endpoint URL
- IPv6 flag
- Error text (for offline)

## Database Schema

### member_events Table (Enhanced)
```sql
CREATE TABLE member_events (
    check_type INT,          -- 1=site, 2=domain, 3=endpoint
    check_name VARCHAR(100),
    endpoint TEXT,
    domain_name VARCHAR(255),
    member_name VARCHAR(255),
    status TINYINT(1),
    is_ipv6 TINYINT(1),
    start_time TIMESTAMP,
    end_time TIMESTAMP NULL,
    error TEXT,
    vote_data JSON,          -- Consensus votes
    additional_data JSON,    -- Extra metadata
    UNIQUE KEY (check_type, check_name, endpoint, domain_name, 
                member_name, is_ipv6, status, start_time)
);
```

### requests Table (Per-Node)
```sql
CREATE TABLE requests (
    date DATE,
    node_id VARCHAR(100),    -- Critical: tracks origin node
    domain_name VARCHAR(255),
    member_name VARCHAR(255),
    network_asn VARCHAR(20),
    network_name VARCHAR(255),
    country_code CHAR(2),
    country_name VARCHAR(255),
    is_ipv6 TINYINT(1),
    hits INT,
    PRIMARY KEY (date, node_id, domain_name, member_name,
                 network_asn, network_name, country_code,
                 country_name, is_ipv6)
);
```

## Usage Patterns

### Hourly Collection (Collator)
```go
// Runs every hour at :00
records := RequestAllDnsUsage(...)
for _, r := range records {
    UpsertUsage(convertToData2Record(r))
}
```

### Event Recording
```go
// On consensus OFFLINE
rec := NetStatusRecord{
    CheckType: 3,  // endpoint
    Member: "provider1",
    Status: false,
    StartTime: time.Now().UTC(),
}
InsertNetStatus(rec)  // Triggers alert

// On consensus ONLINE
CloseOpenEvent(rec)   // Triggers recovery alert
```

## Helper Functions

### Type Conversions
```go
ctToString(ct int) string         // Check type to string
boolToTiny(b bool) int            // Bool to MySQL tinyint
nullOrString(s string) sql.NullString  // Handle nullable strings
nullOrEmpty(s string) sql.NullString   // Alternative null handler
```

## Thread Safety
- Proposal cache protected by sync.RWMutex
- Database operations use connection pooling
- Safe for concurrent operations

## Best Practices

1. **Always use UpsertUsage** for idempotent updates
2. **Include node_id** in all usage records
3. **Force UTC** for all timestamps
4. **Check Matrix integration** status in logs
5. **Monitor proposal expiry** to prevent memory leaks

## Error Handling
- Database errors logged but don't halt processing
- Batch operations continue on individual failures
- Matrix failures don't block database operations
- Connection retry on initialization

## Dependencies
- `github.com/go-sql-driver/mysql`
- `github.com/ibp-network/ibp-geodns-libs/config`
- `github.com/ibp-network/ibp-geodns-libs/logging`
- `github.com/ibp-network/ibp-geodns-libs/matrix`