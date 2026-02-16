package processors

// Asset enricher processor fetches asset metadata from an HTTP endpoint and attaches it to events.
// It supports nested field paths (e.g., "device.uid") for OCSF events and adds contextual tags.

import (
	"context"
	"fmt"
	"time"

	"github.com/warpstreamlabs/bento/public/service"
)

// init registers the asset_enricher processor with Bento at startup.
// This function is automatically called when the package is imported.
func init() {
	if err := service.RegisterProcessor("asset_enricher", assetEnricherConfig(), newAssetEnricher); err != nil {
		panic(err)
	}
}

// assetEnricherConfig defines the configuration schema for the asset enricher.
// Returns a ConfigSpec that specifies what fields can be configured in pipeline.yaml.
func assetEnricherConfig() *service.ConfigSpec {
	return service.NewConfigSpec().
		Summary("Enriches events with asset metadata from an HTTP service").
		Field(service.NewStringField("endpoint").Description("Base URL, e.g. https://asset-api/assets")).
		Field(service.NewStringField("id_field").Description("Field containing asset id").Default("asset_id")).
		Field(service.NewDurationField("timeout").Description("HTTP timeout").Default("5s"))
}

// assetEnricher is the processor instance that enriches events with asset/device metadata.
// It maintains configuration and a logger for the lifecycle of the processor.
type assetEnricher struct {
	endpoint string          // Base URL for asset API (e.g., http://localhost:1080/asset)
	idField  string          // Field path for asset ID (supports nested like "device.uid")
	timeout  time.Duration   // HTTP request timeout (default: 5s)
	logger   *service.Logger // Logger for warnings and errors
}

// newAssetEnricher constructs a new asset enricher instance from parsed configuration.
// Called by Bento when the processor is initialized in the pipeline.
//
// Parameters:
//   - conf: Parsed configuration from pipeline.yaml
//   - res: Shared resources including logger
//
// Returns:
//   - service.Processor: The initialized processor instance
//   - error: Any configuration parsing errors
func newAssetEnricher(conf *service.ParsedConfig, res *service.Resources) (service.Processor, error) {
	endpoint, err := conf.FieldString("endpoint")
	if err != nil {
		return nil, err
	}
	idField, err := conf.FieldString("id_field")
	if err != nil {
		return nil, err
	}
	timeout, err := conf.FieldDuration("timeout")
	if err != nil {
		return nil, err
	}
	return &assetEnricher{endpoint: endpoint, idField: idField, timeout: timeout, logger: res.Logger()}, nil
}

// Process enriches a single message with asset metadata and contextual tags.
// This is called for every message that flows through the processor.
//
// The processor:
//  1. Extracts the asset ID from a field path (supports nested like "device.uid")
//  2. Calls the asset API to retrieve full device/asset details
//  3. Adds the asset data to the "asset" field
//  4. Adds contextual tags based on OCSF event type and asset properties
//
// OCSF-aware tagging:
//   - Detection Finding (class_uid 2004): Tags for severity, findings
//   - Process Activity (class_uid 1007): Tags for EDR events
//   - Observables: Tags when IOCs are present
//   - Asset properties: Tags based on owner, criticality
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - msg: The message to enrich
//
// Returns:
//   - service.MessageBatch: Batch containing the enriched message
//   - error: Any processing errors (nil for warnings, which are logged)
func (p *assetEnricher) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	// Parse message body as structured JSON
	body, err := msg.AsStructured()
	if err != nil {
		return nil, err
	}
	obj, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("asset_enricher expects object")
	}

	// Extract asset ID from nested path (e.g., "device.uid" → extracts from obj["device"]["uid"])
	assetID := extractNestedField(obj, p.idField)
	if assetID == "" {
		// No asset ID found, return message unchanged
		return service.MessageBatch{msg}, nil
	}

	// Fetch asset metadata from HTTP API with timeout
	c, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	url := fmt.Sprintf("%s/%s", p.endpoint, assetID)
	data, err := fetchJSON(c, url)
	if err != nil {
		// Log warning but don't fail the message
		p.logger.Warnf("asset_enrich failed for %s: %v", assetID, err)
		msg.MetaSet("asset_enrich_error", err.Error())
		return service.MessageBatch{msg}, nil
	}

	// Add asset data to message
	obj["asset"] = data

	// === CONTEXTUAL TAGGING ===
	// Initialize tags array (preserve existing tags if present)
	tags := []interface{}{}
	if existingTags, ok := obj["tags"].([]interface{}); ok {
		tags = existingTags
	}

	// OCSF Detection Finding tagging (class_uid: 2004)
	// These are security alerts/findings from detection systems
	if classUID, ok := obj["class_uid"].(float64); ok && classUID == 2004 {
		tags = append(tags, "ocsf_detection_finding")

		// Tag by severity (1=Info, 2=Low, 3=Medium, 4=High, 5=Critical)
		if severityID, ok := obj["severity_id"].(float64); ok {
			if severityID >= 4 {
				tags = append(tags, "high_severity")
			}
			if severityID == 5 {
				tags = append(tags, "critical_severity")
			}
		}

		// Tag if finding has a title (indicates structured finding data)
		if finding, ok := obj["finding"].(map[string]interface{}); ok {
			if title, ok := finding["title"].(string); ok && title != "" {
				tags = append(tags, "has_finding_title")
			}
		}
	}

	// OCSF Process Activity tagging (class_uid: 1007)
	// These are EDR (Endpoint Detection and Response) process events
	if classUID, ok := obj["class_uid"].(float64); ok && classUID == 1007 {
		tags = append(tags, "ocsf_process_activity")
		tags = append(tags, "edr_event")
	}

	// Tag if observables are present (IOCs like files, IPs, domains, hashes)
	if observables, ok := obj["observables"].([]interface{}); ok && len(observables) > 0 {
		tags = append(tags, "has_observables")
	}

	// Tag based on asset properties returned from API
	// These are examples - customize based on your asset API schema
	if owner, ok := data["owner"].(string); ok && owner == "IT" {
		tags = append(tags, "it_asset")
	}
	if criticality, ok := data["criticality"].(string); ok && criticality == "high" {
		tags = append(tags, "critical_asset")
	}

	// Update message with tags
	obj["tags"] = tags
	msg.SetStructured(obj)
	return service.MessageBatch{msg}, nil
}

// Close performs cleanup when the processor is shut down.
// Currently no cleanup is needed, but this method is required by the Processor interface.
func (p *assetEnricher) Close(ctx context.Context) error { return nil }

// extractNestedField extracts a value from a nested field path like "device.uid".
// Supports dot-notation paths to traverse nested objects.
//
// Example:
//   - fieldPath: "device.uid"
//   - obj: {"device": {"uid": "550e8400-e29b-41d4-a716-446655440001"}}
//   - returns: "550e8400-e29b-41d4-a716-446655440001"
//
// Parameters:
//   - obj: The object to extract from
//   - fieldPath: Dot-separated path (e.g., "device.uid", "user.name")
//
// Returns:
//   - string: The extracted value, or empty string if not found
func extractNestedField(obj map[string]interface{}, fieldPath string) string {
	// Split field path by dots (e.g., "device.uid" → ["device", "uid"])
	parts := splitFieldPath(fieldPath)

	// Navigate through nested objects
	current := interface{}(obj)
	for _, part := range parts {
		// Type assert to map at each level
		m, ok := current.(map[string]interface{})
		if !ok {
			return "" // Not a map, path is invalid
		}
		// Get next level
		current, ok = m[part]
		if !ok {
			return "" // Field doesn't exist
		}
	}

	// Convert final value to string
	if str, ok := current.(string); ok {
		return str
	}
	return "" // Value exists but is not a string
}

// splitFieldPath splits a dot-separated path into individual parts.
// Handles simple dot notation without escaping.
//
// Example:
//   - "device.uid" → ["device", "uid"]
//   - "user.profile.email" → ["user", "profile", "email"]
//   - "simple" → ["simple"]
//
// Parameters:
//   - path: Dot-separated field path
//
// Returns:
//   - []string: Array of path components
func splitFieldPath(path string) []string {
	var result []string
	current := ""

	// Iterate through each character
	for _, ch := range path {
		if ch == '.' {
			// Found a separator, save current part if not empty
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			// Build up current part
			current += string(ch)
		}
	}

	// Add final part if present
	if current != "" {
		result = append(result, current)
	}

	return result
}
