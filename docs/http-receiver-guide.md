# HTTP Receiver: Batch Ingestion with Tagging

This example demonstrates receiving events via HTTP, adding tags based on payload content, and writing to Kafka.

## Architecture

```
HTTP Client → HTTP Receiver → Payload Tagger → Kafka Topic
             (port 8080)      (add tags)       (events.tagged)
```

## Components

### 1. HTTP Receiver Input (`http_receiver`)
- **Location:** `internal/inputs/http_receiver.go`
- **Purpose:** Accept events via HTTP POST requests
- **Features:**
  - Accepts single events or batches (arrays)
  - Configurable address, path, and batch size
  - Health check endpoint at `/health`
  - Returns confirmation with event count

**Configuration:**
```yaml
input:
  http_receiver:
    address: "0.0.0.0:8080"      # Bind address
    path: "/events"               # HTTP path
    timeout: 30s                  # Read timeout
    max_batch_size: 100           # Max events per request
```

### 2. Payload Tagger Processor (`payload_tagger`)
- **Location:** `internal/processors/payload_tagger.go`
- **Purpose:** Examine payload and add metadata tags
- **Features:**
  - Automatic OCSF field mapping (class_uid → category)
  - Severity mapping (severity_id → priority)
  - Content detection (observables, threat intel, etc.)
  - Extensible tag rules

**Configuration:**
```yaml
processors:
  - payload_tagger:
      tag_fields:                 # Fields to examine
        - class_uid
        - severity_id
        - category_uid
      add_timestamp_tag: true     # Add ingestion timestamp
      add_source_tag: true        # Add source identifier
```

**Tags Added:**
| Field | Tag Examples |
|-------|-------------|
| `class_uid: 2004` | `tag.category=detection`, `tag.type=alert` |
| `class_uid: 5001` | `tag.category=asset`, `tag.type=inventory` |
| `severity_id: 5` | `tag.severity=critical`, `tag.priority=critical` |
| `severity_id: 3` | `tag.severity=medium`, `tag.priority=medium` |
| `observables` present | `tag.has_observables=true`, `tag.observable_count=2` |
| `threat_intel` present | `tag.has_threat_intel=true`, `tag.threat_detected=true` |

### 3. Kafka Output
- **Purpose:** Write tagged events to Kafka topic
- **Features:**
  - Includes all tags as message headers
  - Configurable compression and batching
  - High throughput with `max_in_flight`

## Quick Start

### 1. Build the processor
```bash
go build -o data-processor .
```

### 2. Start Kafka/Redpanda
```bash
docker compose up -d
```

### 3. Create the output topic
```bash
docker compose exec redpanda rpk topic create events.tagged
```

### 4. Start the HTTP receiver pipeline
```bash
./data-processor -c config/http-to-kafka.yaml
```

You should see:
```
INFO HTTP receiver listening on http://0.0.0.0:8080/events
```

### 5. Send test events

**Single event:**
```bash
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '{
    "metadata": {"uid": "evt-001"},
    "class_uid": 2004,
    "severity_id": 4,
    "message": "Malware detected"
  }'
```

**Response:**
```json
{
  "status": "accepted",
  "count": 1,
  "received": "2026-02-17T10:30:00Z"
}
```

**Batch of events:**
```bash
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '[
    {"class_uid": 2004, "severity_id": 5, "message": "Critical alert"},
    {"class_uid": 5001, "severity_id": 1, "message": "New asset"},
    {"class_uid": 3001, "severity_id": 3, "message": "Auth failure"}
  ]'
```

**Using test script:**
```bash
./test-http-receiver.sh
```

### 6. Verify events in Kafka
```bash
# Consume from Kafka topic
docker compose exec redpanda rpk topic consume events.tagged --format '%v\n'

# View with headers (tags)
docker compose exec redpanda rpk topic consume events.tagged \
  --format 'Headers: %h\nValue: %v\n---\n'
```

## Example Event Flow

**Input (HTTP POST):**
```json
{
  "metadata": {"uid": "evt-123"},
  "class_uid": 2004,
  "severity_id": 5,
  "category_uid": 2,
  "message": "Ransomware detected",
  "observables": [
    {
      "name": "ransomware.exe",
      "type_id": 7,
      "threat_intel": {
        "severity": "critical",
        "threat_type": "Ransomware"
      }
    }
  ]
}
```

**Tags Added (message metadata):**
```
tag.source: http_receiver
tag.ingested_at: 2026-02-17T10:30:00Z
tag.class_uid: 2004
tag.severity_id: 5
tag.category_uid: 2
tag.category: detection
tag.type: alert
tag.severity: critical
tag.priority: critical
tag.has_observables: true
tag.observable_count: 1
tag.has_threat_intel: true
tag.threat_detected: true
```

**Output (Kafka):**
- **Topic:** `events.tagged`
- **Headers:** All tags (for filtering/routing)
- **Body:** Original event (unchanged)

## Use Cases

### 1. API Integration
Receive events from external systems via REST API:
```bash
# Application sends events
curl -X POST http://data-processor:8080/events \
  -H "Content-Type: application/json" \
  -d @events.json
```

### 2. Batch Uploads
Process large batches efficiently:
```bash
# Upload 100 events at once
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d @batch-events.json
```

### 3. Tag-Based Routing
Use tags for downstream routing:

**Routing by severity:**
```yaml
output:
  switch:
    cases:
      - check: 'meta("tag.priority") == "critical"'
        output:
          kafka:
            topic: events.critical
      
      - check: 'meta("tag.priority") == "high"'
        output:
          kafka:
            topic: events.high
      
      - output:
          kafka:
            topic: events.normal
```

**Routing by category:**
```yaml
output:
  switch:
    cases:
      - check: 'meta("tag.category") == "detection"'
        output:
          kafka:
            topic: events.alerts
      
      - check: 'meta("tag.category") == "asset"'
        output:
          kafka:
            topic: events.assets
      
      - output:
          kafka:
            topic: events.other
```

### 4. Filtering
Filter events based on tags before writing:
```yaml
pipeline:
  processors:
    - payload_tagger:
        tag_fields: [class_uid, severity_id]
    
    # Only keep high/critical events
    - bloblang: |
        root = if meta("tag.priority") == "high" || meta("tag.priority") == "critical" {
          this
        } else {
          deleted()
        }

output:
  kafka:
    topic: events.important
```

## Advanced Configuration

### High Throughput Setup
```yaml
input:
  http_receiver:
    address: "0.0.0.0:8080"
    path: "/events"
    timeout: 5s
    max_batch_size: 1000  # Accept larger batches

pipeline:
  threads: 8              # Parallel processing
  processors:
    - payload_tagger:
        tag_fields: [class_uid, severity_id]

output:
  kafka:
    addresses: ["localhost:9092"]
    topic: events.tagged
    max_in_flight: 50     # High concurrency
    compression: lz4      # Fast compression
    batching:
      count: 500          # Batch writes
      period: 1s
```

### Multiple Endpoints
Receive different event types on different paths:
```yaml
input:
  broker:
    inputs:
      - http_receiver:
          address: "0.0.0.0:8080"
          path: "/alerts"
      
      - http_receiver:
          address: "0.0.0.0:8080"
          path: "/assets"
      
      - http_receiver:
          address: "0.0.0.0:8080"
          path: "/logs"
```

### Authentication (Using Standard HTTP Input)
For production, use Bento's standard `http_server` with authentication:
```yaml
input:
  http_server:
    address: "0.0.0.0:8443"
    path: /events
    cert_file: /path/to/cert.pem
    key_file: /path/to/key.pem
    sync_response:
      status: "202"
```

## Monitoring

### Health Check
```bash
curl http://localhost:8080/health
# Response: {"status":"ok"}
```

### Metrics
Check pipeline logs for:
- Events received per request
- Tagging operations
- Kafka write success/failure

### Debug Logging
Enable debug logs to see all tags:
```yaml
pipeline:
  processors:
    - payload_tagger:
        tag_fields: [class_uid, severity_id]
    
    - log:
        level: DEBUG
        message: |
          Event: ${! json("metadata.uid") }
          Tags: category=${! meta("tag.category") } severity=${! meta("tag.severity") }
```

## Troubleshooting

**Events not accepted:**
- Check HTTP response: `curl -v http://localhost:8080/events`
- Verify JSON format: `cat event.json | jq .`
- Check max_batch_size limit

**Tags not appearing:**
- Enable debug logging to see tag operations
- Verify field names match your event structure
- Check Kafka headers: `rpk topic consume --format '%h\n'`

**Performance issues:**
- Increase `threads` in pipeline config
- Increase `max_in_flight` for Kafka output
- Enable batching in Kafka output
- Use compression (snappy or lz4)

## Next Steps

1. **Customize tags:** Modify `payload_tagger.go` for your use case
2. **Add routing:** Use tags for conditional routing
3. **Add authentication:** Implement auth in HTTP receiver
4. **Add validation:** Validate events before tagging
5. **Add metrics:** Export metrics for monitoring

## See Also

- [Custom Components Guide](custom-components.md)
- [HTTP Receiver Source](../internal/inputs/http_receiver.go)
- [Payload Tagger Source](../internal/processors/payload_tagger.go)
- [Test Script](../test-http-receiver.sh)
