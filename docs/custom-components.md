# Custom Components Reference

This framework supports three types of custom components:

## 1. Custom Processors
Process/transform messages as they flow through the pipeline.

**Use cases:**
- Enrichment (adding data from APIs)
- Filtering (include/exclude logic)
- Transformation (modify fields)
- Validation (check data quality)

**Example:** `internal/processors/threat_intel_enricher.go`

## 2. Custom Inputs
Read data from custom sources into the pipeline.

**Use cases:**
- Custom APIs or protocols
- Proprietary data formats
- Test data generation
- Database polling
- IoT device connections

**Example:** `internal/inputs/example_input.go.example`

**Key methods:**
- `Connect()` - Establish connection to source
- `Read()` - Fetch next message
- `Close()` - Clean up resources

## 3. Custom Outputs
Write processed data to custom destinations.

**Use cases:**
- Custom APIs or protocols
- Proprietary storage systems
- Custom formatting/logging
- Database writes
- Alert systems

**Example:** `internal/outputs/example_output.go.example`

**Key methods:**
- `Connect()` - Establish connection to destination
- `Write()` - Send message to destination
- `Close()` - Clean up resources

## Registration Pattern

All three follow the same pattern:

```go
// In init() function:
service.RegisterProcessor("my_processor", spec, constructor)
service.RegisterInput("my_input", spec, constructor)
service.RegisterOutput("my_output", spec, constructor)
```

**Configuration spec** defines:
- Summary/description
- Configuration fields with types and defaults
- Validation rules

**Constructor function** creates instance from config.

## How to Add Custom Components

### Step 1: Create the component file

**For Processor:**
```go
// internal/processors/my_processor.go
package processors

import "github.com/warpstreamlabs/bento/public/service"

func init() {
    service.RegisterProcessor("my_processor", spec, constructor)
}

type MyProcessor struct { /* fields */ }

func (p *MyProcessor) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
    // Your logic here
    return service.MessageBatch{msg}, nil
}

func (p *MyProcessor) Close(ctx context.Context) error {
    return nil
}
```

**For Input:**
```go
// internal/inputs/my_input.go
package inputs

import "github.com/warpstreamlabs/bento/public/service"

func init() {
    service.RegisterInput("my_input", spec, constructor)
}

type MyInput struct { /* fields */ }

func (i *MyInput) Connect(ctx context.Context) error { /* connect */ }
func (i *MyInput) Read(ctx context.Context) (*service.Message, service.AckFunc, error) { /* read */ }
func (i *MyInput) Close(ctx context.Context) error { /* cleanup */ }
```

**For Output:**
```go
// internal/outputs/my_output.go
package outputs

import "github.com/warpstreamlabs/bento/public/service"

func init() {
    service.RegisterOutput("my_output", spec, constructor)
}

type MyOutput struct { /* fields */ }

func (o *MyOutput) Connect(ctx context.Context) error { /* connect */ }
func (o *MyOutput) Write(ctx context.Context, msg *service.Message) error { /* write */ }
func (o *MyOutput) Close(ctx context.Context) error { /* cleanup */ }
```

### Step 2: Import in main.go

```go
import (
    _ "github.com/data-processor-framework/internal/processors"
    _ "github.com/data-processor-framework/internal/inputs"
    _ "github.com/data-processor-framework/internal/outputs"
)
```

### Step 3: Use in configuration

```yaml
input:
  my_input:
    config_field: value

pipeline:
  processors:
    - my_processor:
        config_field: value

output:
  my_output:
    config_field: value
```

### Step 4: Build and run

```bash
go build -o data-processor .
./data-processor -c config/pipeline.yaml
```

## Advanced Patterns

### Batching Input
Read multiple messages at once:
```go
func (i *MyInput) Read(ctx context.Context) (*service.Message, service.AckFunc, error) {
    // Return batch
    batch := service.MessageBatch{msg1, msg2, msg3}
    return batch[0], ackFunc, nil
}
```

### Parallel Output
Control concurrency with maxInFlight:
```go
service.RegisterOutput("my_output", spec, 
    func(conf *service.ParsedConfig, mgr *service.Resources) (service.Output, int, error) {
        return output, 10, nil  // maxInFlight = 10 (parallel writes)
    })
```

### Batched Output
Write multiple messages efficiently:
```go
func (o *MyOutput) WriteBatch(ctx context.Context, batch service.MessageBatch) error {
    // Write all messages in one operation
    return nil
}
```

### Stateful Components
Maintain state across messages:
```go
type StatefulProcessor struct {
    cache  map[string]interface{}
    mutex  sync.RWMutex
    logger *service.Logger
}

func (p *StatefulProcessor) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
    p.mutex.Lock()
    defer p.mutex.Unlock()
    
    // Use cache
    if val, ok := p.cache[key]; ok {
        // Cache hit
    }
    
    return service.MessageBatch{msg}, nil
}
```

## Best Practices

1. **Error Handling**: Return errors instead of panicking
2. **Context Awareness**: Respect context cancellation
3. **Logging**: Use provided logger for debugging
4. **Resource Cleanup**: Always implement Close() properly
5. **Configuration Validation**: Validate config in constructor
6. **Thread Safety**: Use mutexes for shared state
7. **Timeouts**: Set reasonable timeouts for external calls
8. **Testing**: Write unit tests for your components

## See Also

- [Bento Documentation](https://warpstreamlabs.github.io/bento/docs/about)
- [Standard Components](https://warpstreamlabs.github.io/bento/docs/components/inputs/about)
- Example implementations in `internal/processors/`
