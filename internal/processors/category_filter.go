package processors

// Category filter processor drops or keeps events based on include/exclude category lists.
// Used to filter events by category field (e.g., only allow "security" events, or drop "test" events).

import (
	"context"
	"fmt"

	"github.com/warpstreamlabs/bento/public/service"
)

// init registers the category_filter processor with Bento at startup.
// This function is automatically called when the package is imported.
func init() {
	if err := service.RegisterProcessor("category_filter", categoryFilterConfig(), newCategoryFilter); err != nil {
		panic(err)
	}
}

// categoryFilterConfig defines the configuration schema for the category filter.
// Returns a ConfigSpec that specifies what fields can be configured in pipeline.yaml.
func categoryFilterConfig() *service.ConfigSpec {
	return service.NewConfigSpec().
		Summary("Filters events by category include/exclude lists").
		Field(service.NewStringField("field").Description("Field to read category from").Default("category")).
		Field(service.NewStringListField("include").Description("Allow only these categories").Default([]string{})).
		Field(service.NewStringListField("exclude").Description("Drop these categories").Default([]string{}))
}

// categoryFilter is the processor instance that filters messages by category.
type categoryFilter struct {
	field   string              // Field name to read category from (e.g., "category")
	include map[string]struct{} // Allowlist: if not empty, only these categories pass through
	exclude map[string]struct{} // Denylist: these categories are dropped
}

// newCategoryFilter constructs a new category filter instance from parsed configuration.
// Called by Bento when the processor is initialized in the pipeline.
//
// Converts include/exclude string lists into maps for O(1) lookup performance.
//
// Parameters:
//   - conf: Parsed configuration from pipeline.yaml
//   - _: Shared resources (unused)
//
// Returns:
//   - service.Processor: The initialized processor instance
//   - error: Any configuration parsing errors
func newCategoryFilter(conf *service.ParsedConfig, _ *service.Resources) (service.Processor, error) {
	field, err := conf.FieldString("field")
	if err != nil {
		return nil, err
	}
	inc, err := conf.FieldStringList("include")
	if err != nil {
		return nil, err
	}
	exc, err := conf.FieldStringList("exclude")
	if err != nil {
		return nil, err
	}

	// Build include map for fast lookup
	ci := make(map[string]struct{}, len(inc))
	for _, v := range inc {
		ci[v] = struct{}{}
	}

	// Build exclude map for fast lookup
	ce := make(map[string]struct{}, len(exc))
	for _, v := range exc {
		ce[v] = struct{}{}
	}

	return &categoryFilter{field: field, include: ci, exclude: ce}, nil
}

// Process filters messages based on include/exclude category rules.
// This is called for every message that flows through the processor.
//
// Filtering logic:
//  1. If include list is not empty: only messages with matching category pass through
//  2. If exclude list is not empty: messages with matching category are dropped
//  3. If neither list is set: all messages pass through
//  4. If category field is missing or not a string: message passes through unchanged
//
// Examples:
//   - include: ["security", "admin"] → only security and admin events pass
//   - exclude: ["test", "debug"] → test and debug events are dropped
//   - include: ["security"], exclude: ["test"] → only security events pass (test excluded if present)
//
// Parameters:
//   - ctx: Context for cancellation
//   - msg: The message to filter
//
// Returns:
//   - service.MessageBatch: Empty batch (message dropped) or batch containing the message (passed)
//   - error: Any processing errors
func (p *categoryFilter) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	// Parse message body as structured JSON
	body, err := msg.AsStructured()
	if err != nil {
		return nil, err
	}
	obj, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("category_filter expects object")
	}

	// Extract category field value
	raw, ok := obj[p.field]
	if !ok {
		// Category field missing, allow message through
		return service.MessageBatch{msg}, nil
	}
	val, ok := raw.(string)
	if !ok {
		// Category field is not a string, allow message through
		return service.MessageBatch{msg}, nil
	}

	// Apply include filter (allowlist)
	if len(p.include) > 0 {
		if _, ok := p.include[val]; !ok {
			// Category not in include list, drop message
			return service.MessageBatch{}, nil
		}
	}

	// Apply exclude filter (denylist)
	if len(p.exclude) > 0 {
		if _, ok := p.exclude[val]; ok {
			// Category in exclude list, drop message
			return service.MessageBatch{}, nil
		}
	}

	// Message passed all filters, allow through
	return service.MessageBatch{msg}, nil
}

// Close performs cleanup when the processor is shut down.
// Currently no cleanup is needed, but this method is required by the Processor interface.
func (p *categoryFilter) Close(ctx context.Context) error { return nil }
