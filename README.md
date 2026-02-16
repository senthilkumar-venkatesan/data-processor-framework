# Event Processing Framework

A **flexible, composable framework** for building custom event processing pipelines using [Bento](https://warpstreamlabs.github.io/bento/docs/about). Create your own processors, chain them together, and process streaming events however you want.

## What is This?

This is a **framework/template** for building virtual event processing pipelines. It demonstrates how to:

- ✅ **Create custom processors** - Write Go code to process events any way you want
- ✅ **Compose pipelines** - Chain processors together in any order via YAML configuration  
- ✅ **Process or discard events** - Transform, enrich, filter, route, or drop events based on your logic
- ✅ **Stream data** - Consume from Kafka, process in real-time, route to multiple outputs
- ✅ **Extend easily** - Add new processors without modifying existing code

**The included processors are just examples** - you can build your own for any use case!

---

## Example Processors (Demonstrations)

This repository includes example processors to show different patterns:

### 1. **Asset Enricher** ([asset_enricher.go](internal/processors/asset_enricher.go))
- **Pattern**: HTTP API enrichment with nested field extraction
- **What it does**: Enriches OCSF events with device/asset metadata
- **Example use case**: Add machine details, owner info, criticality tags

### 2. **Threat Intel Enricher** ([threat_intel_enricher.go](internal/processors/threat_intel_enricher.go))
- **Pattern**: Selective enrichment with type filtering
- **What it does**: Enriches OCSF observables (IPs, domains, files) with threat intelligence
- **Example use case**: Add IOC reputation, malware family, threat actor attribution

### 3. **Category Filter** ([category_filter.go](internal/processors/category_filter.go))
- **Pattern**: Event filtering/dropping
- **What it does**: Allows/blocks events based on category field
- **Example use case**: Drop test events, filter by event type

**These are just examples!** You can create processors for:
- Data transformation (normalize, reshape, mask PII)
- Validation (schema checking, data quality)
- Enrichment (GeoIP, user lookup, threat intel)
- Filtering (sampling, deduplication, noise reduction)
- Routing logic (smart dynamic routing)
- Alerting (trigger notifications, webhooks)
- _...anything you can code!_

---

## How to Create Your Own Processor

Every processor implements the **Bento Processor interface**. Here's how to build one:

### Step 1: Define Configuration Schema

```go
func myProcessorConfig() *service.ConfigSpec {
    return service.NewConfigSpec().
        Summary("My custom processor description").
        Field(service.NewStringField("my_field").Description("Some config")).
        Field(service.NewIntField("threshold").Default(10))
}
```

### Step 2: Create Processor Struct

```go
type myProcessor struct {
    myField   string
    threshold int
    logger    *service.Logger
}
```

### Step 3: Implement Constructor

```go
func newMyProcessor(conf *service.ParsedConfig, res *service.Resources) (service.Processor, error) {
    myField, err := conf.FieldString("my_field")
    if err != nil {
        return nil, err
    }
    threshold, err := conf.FieldInt("threshold")
    if err != nil {
        return nil, err
    }
    return &myProcessor{
        myField:   myField,
        threshold: threshold,
        logger:    res.Logger(),
    }, nil
}
```

### Step 4: Implement Process Method

```go
func (p *myProcessor) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
    // 1. Parse message
    body, err := msg.AsStructured()
    if err != nil {
        return nil, err
    }
    obj, ok := body.(map[string]interface{})
    if !ok {
        return nil, fmt.Errorf("expected object")
    }
    
    // 2. Your custom logic here!
    // - Transform fields
    // - Call APIs
    // - Validate data
    // - Add enrichment
    
    // Example: Add a custom field
    obj["processed_by"] = "my_processor"
    obj["timestamp"] = time.Now().Unix()
    
    // 3. Return modified message (or drop it by returning empty batch)
    msg.SetStructured(obj)
    return service.MessageBatch{msg}, nil
    
    // To DROP the message:
    // return service.MessageBatch{}, nil
}
```

### Step 5: Register Processor

```go
func init() {
    if err := service.RegisterProcessor("my_processor", myProcessorConfig(), newMyProcessor); err != nil {
        panic(err)
    }
}
```

### Step 6: Add to Pipeline

Edit `config/pipeline.yaml`:

```yaml
pipeline:
  processors:
    - my_processor:
        my_field: "value"
        threshold: 20
    # ... other processors
```

### Step 7: Build and Run

```bash
go build -o data-processor .
./data-processor -c config/pipeline.yaml
```

**That's it!** Your processor now handles every event that flows through the pipeline.

---

## Architecture

```
┌─────────────┐
│ Kafka Input │ events.raw
└──────┬──────┘
       │
       ▼
┌─────────────────────┐
│  Your Processors    │ ← Parse → Enrich → Filter → Transform
│  (any order!)       │ ← You control the pipeline logic
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│ Routing Logic       │ Class-based, priority, custom conditions
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│ Kafka Outputs       │ events.alerts, events.assets, events.ready
└─────────────────────┘
```

**Key concepts:**

- **Each processor sees every message** that reaches it in the pipeline
- **Processors can**:
  - Transform the message (add/remove/modify fields)
  - Keep it (return the message batch)
  - Drop it (return empty batch)
  - Fail it (return error)
- **Order matters** - processors run sequentially in the order defined in YAML
- **Stateless by default** - each message processed independently (add state if needed)

---

## Quick Start

### Prerequisites

- Go 1.21+
- Docker & Docker Compose (for Kafka/Redpanda)

### 1. Start Infrastructure

```bash
# Start Redpanda (Kafka) and Mockserver
docker compose up -d
```

### 2. Build the Processor

```bash
go build -o data-processor .
```

### 3. Run the Pipeline

```bash
./data-processor -c config/pipeline.yaml
```

### 4. Send Test Events

```bash
# Send test OCSF events
cat test-events.json | docker compose exec -T redpanda rpk topic produce events.raw

# Or publish a single event
echo '{"class_uid":2004,"category":"security","observables":[{"name":"evil.exe","type_id":7}]}' | \
  docker compose exec -T redpanda rpk topic produce events.raw
```

### 5. Consume Results

```bash
# View enriched alerts
docker compose exec -T redpanda rpk topic consume events.alerts -f '%v\n' | jq .

# View asset events
docker compose exec -T redpanda rpk topic consume events.assets -f '%v\n' | jq .
```

---

## Configuration

### Pipeline Configuration ([config/pipeline.yaml](config/pipeline.yaml))

```yaml
input:
  kafka:
    addresses: [127.0.0.1:19092]
    topics: [events.raw]
    consumer_group: demo-consumer

pipeline:
  threads: 4  # Concurrent processing
  processors:
    # 1. Parse and initialize
    - mapping: |
        root = if this.type() == "object" { this } else { this.parse_json() }
        root.tags = []
    
    # 2. Your custom processors (any order!)
    - asset_enricher:
        endpoint: http://localhost:1080/asset
        id_field: device.uid
        timeout: 5s
    
    - threat_intel_enricher:
        endpoint: http://localhost:1080/ioc
        timeout: 5s
    
    - category_filter:
        field: category
        include: [security, finance]
        exclude: [test]

output:
  switch:
    cases:
      - check: this.class_uid == 2004  # OCSF Detection Finding
        output:
          kafka:
            addresses: [127.0.0.1:19092]
            topic: events.alerts
      
      - check: this.class_uid == 5001  # OCSF Inventory Info
        output:
          kafka:
            addresses: [127.0.0.1:19092]
            topic: events.assets
      
      - output:  # Default
          kafka:
            addresses: [127.0.0.1:19092]
            topic: events.ready
```

### Environment Variables

```bash
export ASSET_API=http://localhost:1080/asset
export THREAT_INTEL_API=http://localhost:1080/ioc
export LOG_LEVEL=INFO  # DEBUG for verbose logging
```

---

## Project Structure

```
.
├── main.go                          # Entry point - bootstraps Bento
├── config/
│   └── pipeline.yaml                # Pipeline configuration
│   └── mockserver/init.json         # Mock API responses (for testing)
├── internal/processors/
│   ├── asset_enricher.go            # Example: HTTP enrichment processor
│   ├── threat_intel_enricher.go     # Example: Selective enrichment processor
│   ├── category_filter.go           # Example: Filtering processor
│   └── common.go                    # Shared utilities (HTTP fetcher)
├── test-events.json                 # Sample OCSF events
└── docker-compose.yaml              # Kafka + MockServer
```

**Add your processors** to `internal/processors/` and they'll be automatically compiled into the binary.

---

## Use Cases

### Security Event Processing (OCSF)
- Enrich security events with threat intel, asset context, user data
- Filter out false positives
- Route high-priority alerts to SOC team
- **Example**: This repository (OCSF Detection Findings)

### IoT/Telemetry Processing
- Parse device telemetry
- Enrich with device metadata
- Aggregate metrics
- Drop noisy/redundant data

### Log Aggregation
- Normalize logs from different sources
- Mask PII/sensitive data
- Enrich with context (service, environment)
- Route by severity

### Data Quality Pipeline
- Validate schema
- Check data quality rules
- Drop invalid records
- Alert on anomalies

### ETL/Data Integration
- Transform between formats
- Enrich from multiple data sources
- Split/join streams
- Load to multiple destinations

---

## Development

### Run Tests

```bash
go test ./...
```

### Enable Debug Logging

```yaml
# pipeline.yaml
logger:
  level: DEBUG
  format: logfmt
```

### Hot Reload (Development)

```bash
# Rebuild and restart
pkill -f data-processor && go build -o data-processor . && ./data-processor -c config/pipeline.yaml
```

### Clean Build

```bash
rm -f data-processor && go clean -cache && go build -o data-processor .
```

---

## Advanced Topics

### Stateful Processors

Store state across messages (e.g., counting, windowing):

```go
type statefulProcessor struct {
    counter int
    mu      sync.Mutex
}

func (p *statefulProcessor) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
    p.mu.Lock()
    p.counter++
    count := p.counter
    p.mu.Unlock()
    
    // Use count in your logic
    return service.MessageBatch{msg}, nil
}
```

### Async Operations

Use goroutines for parallel operations:

```go
func (p *myProcessor) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
    var wg sync.WaitGroup
    results := make(chan Result, 3)
    
    // Launch parallel HTTP calls
    for _, url := range urls {
        wg.Add(1)
        go func(u string) {
            defer wg.Done()
            data := fetchData(u)
            results <- data
        }(url)
    }
    
    wg.Wait()
    close(results)
    
    // Combine results
    return service.MessageBatch{msg}, nil
}
```

### Error Handling

```go
// Fail the message (stops pipeline, retries)
return nil, fmt.Errorf("critical error: %v", err)

// Log warning, continue processing
p.logger.Warnf("enrichment failed: %v", err)
return service.MessageBatch{msg}, nil

// Drop the message silently
return service.MessageBatch{}, nil
```

---

## Resources

- [Bento Documentation](https://warpstreamlabs.github.io/bento/docs/about)
- [OCSF Schema](https://schema.ocsf.io/)
- [Processor Examples](https://github.com/warpstreamlabs/bento/tree/main/internal/impl)

---

## Contributing

1. Fork the repository
2. Create your processor in `internal/processors/`
3. Add configuration to `config/pipeline.yaml`
4. Test your changes
5. Submit a pull request

**Feel free to add your own processors and share them!**

---

## License

MIT
