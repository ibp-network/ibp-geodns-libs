# nats - Distributed Messaging Library

## Overview
The nats package implements distributed consensus, cluster coordination, and cross-node communication for the IBP GeoDNS system using NATS messaging.

## Key Features
- Distributed consensus protocol with voting
- Cluster membership management
- Usage and downtime data aggregation
- Request-reply fan-out patterns
- Automatic reconnection handling
- Role-based message routing

## Architecture

### Core Components
1. **Connection Manager** - NATS client lifecycle
2. **Consensus Engine** - Proposal/vote/finalize protocol
3. **Cluster Coordinator** - Node discovery and health
4. **Data Collectors** - Usage and downtime aggregation
5. **Role Handlers** - Node type-specific processing

## Consensus Protocol

### Three-Phase Process
1. **Propose** - Node detects status change
2. **Vote** - Other nodes verify independently
3. **Finalize** - Majority consensus applied

### Consensus Requirements
- Minimum 2 votes required
- Majority of active monitors must agree
- 30-second proposal timeout
- Automatic garbage collection

### Proposal Structure
```go
type Proposal struct {
    ID             string                 // UUID
    SenderNodeID   string                 // Origin node
    CheckType      string                 // site/domain/endpoint
    CheckName      string                 // Check identifier
    MemberName     string                 // Provider name
    DomainName     string                 // Service domain
    Endpoint       string                 // Full URL
    ProposedStatus bool                   // Online/offline
    ErrorText      string                 // Error details
    Data           map[string]interface{} // Metadata
    IsIPv6         bool                   // IPv6 flag
    Timestamp      time.Time              // UTC timestamp
}
```

## Node Roles

### IBPMonitor
- Participates in consensus voting
- Proposes status changes
- Maintains local and official results
- Responds to downtime requests

### IBPDns
- Responds to usage data requests
- Tracks DNS query statistics
- No consensus participation

### IBPCollator
- Collects usage data hourly
- Stores consensus results
- Manages proposal cache
- No voting capability

## Connection Management

### Connection Options
```go
opts := []nats.Option{
    nats.MaxReconnects(-1),        // Infinite reconnection
    nats.ReconnectWait(2 * time.Second),
    nats.Timeout(10 * time.Second),
    nats.PingInterval(200 * time.Second),
    nats.MaxPingsOutstanding(5),
}
```

### Error Handling
- Automatic reconnection on disconnect
- Graceful handling of I/O resets
- Connection state callbacks

## Message Subjects

### Consensus Subjects
- `consensus.propose` - Status change proposals
- `consensus.vote` - Voting messages
- `consensus.finalize` - Consensus results
- `consensus.cluster` - Node join/leave

### Data Collection Subjects
- `dns.usage.getUsage` - Request usage data
- `dns.usage.usageData` - Usage responses
- `monitor.stats.getDowntime` - Request downtime
- `monitor.stats.downtimeData` - Downtime responses

## Cluster Management

### Node Discovery
```go
type NodeInfo struct {
    NodeID        string    // Unique identifier
    PublicAddress string    // External IP
    ListenAddress string    // Bind address
    ListenPort    string    // Service port
    NodeRole      string    // IBPMonitor/IBPDns/IBPCollator
    LastHeard     time.Time // Last activity
}
```

### Heartbeat System
- 90-second heartbeat interval
- 10-minute active window
- Automatic stale node cleanup
- Join broadcast on startup

## Data Collection

### Usage Request/Response
```go
RequestAllDnsUsage(req UsageRequest, timeout time.Duration) ([]UsageRecord, error)
```
- Fan-out to all DNS nodes
- Aggregates responses
- Deduplicates by key
- Timeout handling

### Downtime Request/Response
```go
RequestAllMonitorsDowntime(req DowntimeRequest, timeout time.Duration) ([]DowntimeEvent, error)
```
- Queries all monitor nodes
- Collects offline events
- Merges results

## Consensus Functions

### Propose Status Change
```go
ProposeCheckStatus(
    checkType, checkName, memberName,
    domainName, endpoint string,
    status bool,
    errorText string,
    dataMap map[string]interface{},
    isIPv6 bool,
)
```

### Vote Processing
- Automatic voting based on local observations
- 5ms delay to prevent race conditions
- Agreement determination

### Finalization
- Applies to official results
- Triggers database updates
- Notifies collator nodes

## Helper Functions

### Service Discovery
```go
findCheckByName(checkName, checkType string) (cfg.Check, bool)
findMemberByName(memberName string) (cfg.Member, bool)
findServiceForDomain(domainName string) (cfg.Service, bool)
```

### Node Counting
```go
CountActiveMonitors() int  // Active IBPMonitor nodes
CountActiveDns() int        // Active IBPDns nodes
```

## Usage Examples

### Enable Monitor Role
```go
import nats "github.com/ibp-network/ibp-geodns-libs/nats"

// Initialize connection
nats.Connect()

// Set node identity
nats.State.NodeID = "monitor-us-east-1"
nats.State.ThisNode.ListenAddress = "10.0.0.1"
nats.State.ThisNode.ListenPort = "8080"

// Enable role
nats.EnableMonitorRole()
```

### Propose Status Change
```go
nats.ProposeCheckStatus(
    "endpoint",           // type
    "wss",               // check name
    "provider1",         // member
    "rpc.example.com",   // domain
    "wss://...",         // endpoint
    false,               // offline
    "Connection timeout", // error
    nil,                 // extra data
    false,               // IPv4
)
```

### Request Usage Data
```go
req := nats.UsageRequest{
    StartDate:  "2024-01-01",
    EndDate:    "2024-01-31",
    Domain:     "rpc.example.com",
    MemberName: "provider1",
}

records, err := nats.RequestAllDnsUsage(req, 20*time.Second)
```

## Collator Services

### Hourly Usage Collection
```go
StartUsageCollector()
```
- Runs at top of every hour
- Fetches from all DNS nodes
- Stores with UpsertUsage (idempotent)

### Memory Janitor
```go
StartMemoryJanitor()
```
- Runs every 30 seconds
- Expires stale proposals
- Prevents memory leaks

## Thread Safety
- Connection protected by mutex
- Proposal map with RWMutex
- Concurrent-safe node tracking
- Async message handlers

## Best Practices

1. **Set NodeID** before enabling roles
2. **Handle timeouts** in request-reply patterns
3. **Check node counts** before fan-out requests
4. **Monitor connection state** in logs
5. **Use appropriate timeouts** (20s for data requests)

## Performance
- Async message handling
- Subscription pending limits: 1M messages, 128MB
- Connection pooling via single client
- Efficient deduplication in aggregation

## Error Recovery
- Automatic NATS reconnection
- Proposal timeout handling
- Graceful degradation on partial responses
- Non-blocking consensus operations

## Dependencies
- `github.com/nats-io/nats.go` - NATS client
- `github.com/google/uuid` - Proposal IDs
- Internal libs: config, data, data2, logging