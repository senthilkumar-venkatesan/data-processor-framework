package processors

// Threat intel enricher processor enriches OCSF observables with threat intelligence.
// Only enriches observable types relevant for threat intel: IPs, domains, file names, hashes, URLs, emails.
// Other observable types (usernames, process names, ports, etc.) are skipped to optimize API usage.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/warpstreamlabs/bento/public/service"
)

// init registers the threat_intel_enricher processor with Bento at startup.
// This function is automatically called when the package is imported.
func init() {
	if err := service.RegisterProcessor("threat_intel_enricher", threatIntelConfig(), newThreatIntel); err != nil {
		panic(err)
	}
}

// threatIntelConfig defines the configuration schema for the threat intel enricher.
// Returns a ConfigSpec that specifies what fields can be configured in pipeline.yaml.
func threatIntelConfig() *service.ConfigSpec {
	return service.NewConfigSpec().
		Summary("Enriches events with threat intel lookups").
		Field(service.NewStringField("endpoint").Description("Base URL, e.g. https://intel-api/lookup")).
		Field(service.NewDurationField("timeout").Description("HTTP timeout").Default("5s"))
}

// threatIntel is the processor instance that enriches events with threat intelligence data.
// It maintains configuration and a logger for the lifecycle of the processor.
type threatIntel struct {
	endpoint string          // Base URL for threat intel API (e.g., http://localhost:1080/ioc)
	timeout  time.Duration   // HTTP request timeout (default: 5s)
	logger   *service.Logger // Logger for warnings and errors
}

// newThreatIntel constructs a new threat intel enricher instance from parsed configuration.
// Called by Bento when the processor is initialized in the pipeline.
//
// Parameters:
//   - conf: Parsed configuration from pipeline.yaml
//   - res: Shared resources including logger
//
// Returns:
//   - service.Processor: The initialized processor instance
//   - error: Any configuration parsing errors
func newThreatIntel(conf *service.ParsedConfig, res *service.Resources) (service.Processor, error) {
	endpoint, err := conf.FieldString("endpoint")
	if err != nil {
		return nil, err
	}
	timeout, err := conf.FieldDuration("timeout")
	if err != nil {
		return nil, err
	}
	return &threatIntel{endpoint: endpoint, timeout: timeout, logger: res.Logger()}, nil
}

// Process enriches a single message with threat intel data.
// This is called for every message that flows through the processor.
//
// The processor looks for an "observables" array in OCSF events and selectively enriches observables:
//   - Filters observables by type_id to only enrich threat-intel-relevant types
//   - Enriches: IP addresses (2), domains (4), file names (7), hashes (8), URLs (23), emails (5)
//   - Skips: User names (3), process names (9), ports (14), user agents (22), and other non-IOC types
//   - Extracts each observable's "name" field (IOC value like "evil.exe", "192.168.1.100")
//   - Looks up threat intel for each qualifying observable via HTTP API
//   - Adds results to "threat_intel_observables" array
//
// If no observables are present or none qualify for enrichment, the message passes through unchanged.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - msg: The message to enrich
//
// Returns:
//   - service.MessageBatch: Batch containing the enriched message
//   - error: Any processing errors (nil for warnings, which are logged)
func (p *threatIntel) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	p.logger.Infof("===== THREAT INTEL ENRICHER CALLED =====")
	
	// Parse message body as structured JSON
	body, err := msg.AsStructured()
	if err != nil {
		return nil, err
	}
	obj, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("threat_intel_enricher expects object")
	}
	
	p.logger.Infof("Message parsed, checking for observables array...")

	// === OCSF MODE: Check for observables array ===
	// OCSF events contain an "observables" array with IOCs (Indicators of Compromise)
	// Example: [{"name": "evil.exe", "type_id": 7}, {"name": "192.168.1.100", "type_id": 2}]
	if observables, ok := obj["observables"].([]interface{}); ok && len(observables) > 0 {
		p.logger.Infof("Processing %d observables for threat intel enrichment", len(observables))
		enrichedCount := 0

		// Define observable types that should be enriched with threat intelligence
		// Only these types are relevant for threat intel lookups to avoid unnecessary API calls
		threatIntelTypes := map[float64]bool{
			1:  true, // Hostname - Check for malicious hostnames, C2 servers
			2:  true, // IP Address - Check for malicious IPs, command & control servers
			4:  true, // Domain Name - Check for malicious domains, phishing sites
			5:  true, // Email Address - Check for known threat actor emails
			7:  true, // File Name - Check for known malware file names
			8:  true, // File Hash - Check hash reputation (MD5, SHA1, SHA256)
			23: true, // URL - Check for malicious URLs, exploit kits
		}
		// Note: Observable types NOT enriched:
		//   3 (User Name), 9 (Process Name), 14 (Port), 22 (User Agent)
		//   These are typically not found in threat intel feeds or are benign

		// Process each observable in the array (in-place enrichment)
		for i, obs := range observables {
			p.logger.Infof("Processing observable #%d: %+v", i, obs)
			observable, ok := obs.(map[string]interface{})
			if !ok {
				p.logger.Warnf("Observable #%d is not a map, skipping", i)
				continue // Skip if not a valid object
			}

			// Check if this observable type should be enriched with threat intel
			// Extract type_id - it can be stored as int, int64, float64, or json.Number depending on JSON parser
			var typeID float64
			var hasType bool
			switch v := observable["type_id"].(type) {
			case float64:
				typeID = v
				hasType = true
			case int:
				typeID = float64(v)
				hasType = true
			case int64:
				typeID = float64(v)
				hasType = true
			case json.Number:
				// json.Number is used when Decoder.UseNumber() is enabled
				if i64, err := strconv.ParseInt(string(v), 10, 64); err == nil {
					typeID = float64(i64)
					hasType = true
				} else if f64, err := strconv.ParseFloat(string(v), 64); err == nil {
					typeID = f64
					hasType = true
				}
			}
			
			p.logger.Infof("Observable #%d: hasType=%v, typeID=%v, inFilter=%v", i, hasType, typeID, threatIntelTypes[typeID])
			if !hasType || !threatIntelTypes[typeID] {
				// Skip observable types that aren't relevant for threat intelligence
				p.logger.Infof("Skipping observable #%d: type_id=%v (not in threat intel filter)", i, typeID)
				continue
			}

			// Extract the observable name (IOC value like "evil.exe" or "192.168.1.100")
			name, ok := observable["name"].(string)
			if !ok || name == "" {
				continue // Skip if name is missing or empty
			}

			p.logger.Infof("Enriching observable: %s (type_id=%v)", name, typeID)

			// Lookup threat intel for this observable with timeout
			c, cancel := context.WithTimeout(ctx, p.timeout)
			url := fmt.Sprintf("%s/%s", p.endpoint, name)
			data, err := fetchJSON(c, url)
			cancel() // Always cancel context to release resources

			if err != nil {
				// Log warning but continue processing other observables
				p.logger.Warnf("threat_intel lookup failed for %s: %v", name, err)
				continue
			}

			p.logger.Infof("Successfully enriched %s with threat intel", name)

			// Add threat intel data directly to the observable (in-place enrichment)
			// This is OCSF-compliant - adding contextual data to observables
			observable["threat_intel"] = data
			enrichedCount++
		}

		// Log enrichment summary
		if enrichedCount > 0 {
			p.logger.Infof("Enriched %d observables with threat intelligence", enrichedCount)
		} else {
			p.logger.Warnf("No observables were successfully enriched")
		}

		msg.SetStructured(obj)
		return service.MessageBatch{msg}, nil
	}

	// No observables present, return message unchanged
	return service.MessageBatch{msg}, nil
}

// Close performs cleanup when the processor is shut down.
// Currently no cleanup is needed, but this method is required by the Processor interface.
func (p *threatIntel) Close(ctx context.Context) error { return nil }
