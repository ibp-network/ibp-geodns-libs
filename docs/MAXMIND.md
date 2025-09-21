# maxmind - GeoIP Database Library

## Overview
The maxmind package manages GeoLite2 databases with automatic updates, providing geolocation, ASN lookup, and distance calculations for the IBP GeoDNS system.

## Key Features
- Auto-download and update of GeoLite2 databases
- HTTP Last-Modified tracking for efficient updates
- Geographic distance calculation (Haversine formula)
- Country, ASN, and network identification
- URL parsing utilities
- Concurrent-safe database access

## Database Management

### Supported Databases
1. **CityLite.mmdb** - Latitude/longitude coordinates
2. **CountryLite.mmdb** - Country codes and names
3. **AsnLite.mmdb** - ASN and network organization

### Auto-Update System
```go
updateMaxmindDatabase() error
```
- Checks remote Last-Modified header
- Compares with local marker file
- Downloads only when updated
- Extracts and replaces atomically

### Update Process
1. HEAD request for Last-Modified
2. Compare with `.CityLite`, `.CountryLite`, `.AsnLite` markers
3. Download tar.gz if outdated
4. Extract to temp directory
5. Atomic rename to final location
6. Update marker file

## Core Functions

### Initialization
```go
Init()
```
- Creates database directory
- Triggers auto-update check
- Loads databases into memory
- Fatal on initialization failure

### Geolocation
```go
GetClientCoordinates(ipStr string) (lat, lon float64)
```
- Returns latitude/longitude for IP
- Returns (0, 0) on error

### Country Information
```go
GetCountryCode(ipStr string) string     // Returns ISO code (e.g., "US")
GetCountryName(ipStr string) string     // Returns name (e.g., "United States")
```

### Network Information
```go
GetAsnAndNetwork(ipStr string) (asn string, network string)
```
- Returns ASN as "AS12345" format
- Returns organization name

### Distance Calculation
```go
Distance(lat1, lon1, lat2, lon2 float64) float64
```
- Haversine formula implementation
- Returns distance in kilometers
- Earth radius: 6371 km

### Network Classification
```go
GetClassC(ipStr string) string
```
- Returns /24 network prefix
- Format: "192.168.1"
- IPv4 only

### URL Parsing
```go
ParseUrl(rawURL string) URLParts
```
Returns structured URL components:
```go
type URLParts struct {
    Protocol  string  // "https://"
    Domain    string  // "example.com"
    Port      string  // "8080"
    Directory string  // "/path"
}
```

## Configuration

### Required Settings
```go
type MaxmindConfig struct {
    MaxmindDBPath string  // Local database directory
    AccountID     string  // MaxMind account ID
    LicenseKey    string  // MaxMind license key
}
```

### Download URLs
```
https://download.maxmind.com/geoip/databases/{edition}/download
```
Editions:
- GeoLite2-City
- GeoLite2-Country
- GeoLite2-ASN

## File Management

### Directory Structure
```
/path/to/maxmind/
├── CityLite.mmdb
├── .CityLite           # Last-Modified marker
├── CountryLite.mmdb
├── .CountryLite        # Last-Modified marker
├── AsnLite.mmdb
└── .AsnLite            # Last-Modified marker
```

### Extraction Process
1. Downloads to `{name}.tar.gz`
2. Extracts to timestamped folder
3. Finds `.mmdb` file recursively
4. Renames to standard name
5. Cleans up temp files

## Usage Examples

### Basic Geolocation
```go
import max "github.com/ibp-network/ibp-geodns-libs/maxmind"

// Initialize once
max.Init()

// Get coordinates
lat, lon := max.GetClientCoordinates("8.8.8.8")

// Get country
country := max.GetCountryCode("8.8.8.8")

// Get network
asn, org := max.GetAsnAndNetwork("8.8.8.8")
```

### Distance Calculation
```go
// Member location
memberLat, memberLon := 37.7749, -122.4194

// Client location
clientLat, clientLon := max.GetClientCoordinates(clientIP)

// Calculate distance
distance := max.Distance(memberLat, memberLon, clientLat, clientLon)
```

### URL Processing
```go
parts := max.ParseUrl("https://rpc.example.com:8545/ws")
// parts.Protocol = "https://"
// parts.Domain = "rpc.example.com"
// parts.Port = "8545"
// parts.Directory = "/ws"
```

## Error Handling

### Database Loading
- Missing databases logged as ERROR
- Non-fatal for individual databases
- Returns empty/zero values on lookup failure

### Update Failures
- Logged but non-fatal
- Continues using existing databases
- Retries on next Init()

### IP Parsing
- Invalid IPs return empty/zero values
- Errors logged with context

## Thread Safety
- Database readers are thread-safe
- No locking needed for lookups
- Atomic file operations for updates

## Best Practices

1. **Initialize early** - Run Init() at startup
2. **Handle empty returns** - Check for empty strings/zeros
3. **Cache results** - Avoid repeated lookups for same IP
4. **Monitor update logs** - Ensure databases stay current
5. **Secure credentials** - Protect MaxMind license key

## Performance
- In-memory database lookups
- O(log n) lookup time
- Minimal memory overhead
- No network calls after initialization

## Cleanup
```go
Close()
```
- Closes all database readers
- Frees memory
- Call on shutdown

## Dependencies
- `github.com/oschwald/maxminddb-golang` - Database reader
- Standard library (archive/tar, compress/gzip, net/http)