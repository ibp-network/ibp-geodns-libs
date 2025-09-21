# IBP GeoDNS Libs

Shared libraries for the IBP GeoDNS System v2, providing configuration management, data persistence, NATS messaging, health monitoring consensus, and geographic utilities.

## Overview

IBP GeoDNS Libs is the foundational package that powers all IBP GeoDNS components with:
- Centralized configuration with hot-reload support
- MySQL persistence for events, usage, and metrics
- NATS-based distributed consensus protocol
- MaxMind GeoIP integration with auto-updates
- Matrix alerting for outage notifications

## Core Modules

### config
Unified configuration management for local and remote configs.

**Features**:
- Local JSON config loading
- Remote config fetching from GitHub with hot-reload
- Member override preservation during reloads
- Type-safe config structures

**Key Functions**:
- `Init(cfgFile string)` - Initialize with local config path
- `GetConfig()` - Thread-safe config retrieval
- `GetMember(name)` - Member lookup with override status

### data & data2
Two-tier data persistence layer for monitoring results and metrics.

**data/** - Primary data layer:
- Official/local result storage with consensus updates
- Usage statistics with in-memory aggregation
- Event recording for member outages
- 5-minute automatic flush to MySQL

**data2/** - Collator-optimized layer:
- Per-node usage upserts (idempotent)
- Network status tracking with vote data
- Proposal caching for consensus
- Matrix notification triggers

### nats
NATS messaging for distributed consensus and cluster coordination.

**Consensus Protocol**:
1. Monitor proposes status change
2. Other monitors vote based on local observations
3. Majority consensus required (min 2 votes)
4. Finalized results become official

**Key Subjects**:
- `consensus.propose` - Status change proposals
- `consensus.vote` - Voting on proposals
- `consensus.finalize` - Consensus results
- `dns.usage.*` - Usage data collection
- `monitor.stats.*` - Downtime statistics

### maxmind
GeoIP database management with automatic updates.

**Features**:
- Auto-download of GeoLite2 databases (City, Country, ASN)
- HTTP Last-Modified tracking for updates
- Distance calculation using Haversine formula
- Country, ASN, and network lookups

**Databases**:
- `CityLite.mmdb` - Lat/lon coordinates
- `CountryLite.mmdb` - Country codes
- `AsnLite.mmdb` - Network information

### matrix
Real-time alerting via Matrix protocol.

**Features**:
- Deduplicated OFFLINE alerts (one per outage)
- In-place edits for ONLINE recovery
- Member @mentions from config
- Automatic reconnection handling

### logging
Structured logging with configurable levels.

**Levels**: Debug, Info, Warn, Error, Fatal

## Installation

```bash
go get github.com/ibp-network/ibp-geodns-libs
```

## Configuration Structure

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

## Usage Examples

### Initialize Configuration
```go
import cfg "github.com/ibp-network/ibp-geodns-libs/config"

cfg.Init("/path/to/config.json")
config := cfg.GetConfig()
```

### Record DNS Usage
```go
import dat "github.com/ibp-network/ibp-geodns-libs/data"

dat.Init(dat.InitOptions{
    UseLocalOfficialCaches: false,
    UseUsageStats: true,
})
dat.RecordDnsHit(false, "203.0.113.1", "rpc.example.com", "member1")
```

### Propose Status Change
```go
import nats "github.com/ibp-network/ibp-geodns-libs/nats"

nats.ProposeCheckStatus(
    "endpoint",           // checkType
    "wss",               // checkName
    "member1",           // memberName
    "rpc.example.com",   // domainName
    "wss://...",         // endpoint
    false,               // status (offline)
    "Connection timeout", // errorText
    nil,                 // data
    false,               // isIPv6
)
```

### MaxMind Lookups
```go
import max "github.com/ibp-network/ibp-geodns-libs/maxmind"

lat, lon := max.GetClientCoordinates("203.0.113.1")
country := max.GetCountryCode("203.0.113.1")
asn, network := max.GetAsnAndNetwork("203.0.113.1")
distance := max.Distance(lat1, lon1, lat2, lon2)
```

## Database Schema

### member_events
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
    KEY idx_member_status (member_name, status),
    KEY idx_time (start_time, end_time)
);
```

### requests (usage)
```sql
CREATE TABLE requests (
    date DATE,
    node_id VARCHAR(100),
    domain_name VARCHAR(255),
    member_name VARCHAR(255),
    country_code CHAR(2),
    network_asn VARCHAR(20),
    network_name VARCHAR(255),
    country_name VARCHAR(255),
    is_ipv6 TINYINT(1),
    hits INT,
    PRIMARY KEY (date, node_id, domain_name, member_name, 
                 network_asn, network_name, country_code, 
                 country_name, is_ipv6)
);
```

## Environment Requirements

- Go 1.24.x or higher
- MySQL 5.7+ with UTC timezone
- NATS server cluster
- MaxMind account for GeoLite2
- Matrix homeserver (optional)

## Key Design Principles

1. **Thread Safety**: All shared state protected by mutexes
2. **Idempotency**: Usage upserts replace totals, not increment
3. **Consensus**: Minimum 2 votes + majority for status changes
4. **Hot Reload**: Remote configs refresh without restart
5. **Deduplication**: One alert per outage, edited on recovery

## Dependencies

- `github.com/go-sql-driver/mysql` - MySQL driver
- `github.com/nats-io/nats.go` - NATS messaging
- `github.com/oschwald/maxminddb-golang` - MaxMind reader
- `maunium.net/go/mautrix` - Matrix client
- `github.com/google/uuid` - UUID generation

## License

See LICENSE file in repository root.