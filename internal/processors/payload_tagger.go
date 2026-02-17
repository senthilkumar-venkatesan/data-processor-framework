package processors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/warpstreamlabs/bento/public/service"
)

// PayloadTagger examines event payload and adds tags based on content
func init() {
	err := service.RegisterBatchProcessor(
		"payload_tagger",
		service.NewConfigSpec().
			Summary("Adds tags to events based on payload content").
			Description("Examines event fields and adds appropriate tags for routing, filtering, or categorization.").
			Field(service.NewStringListField("tag_fields").
				Default([]string{"class_uid", "severity_id", "category_uid"}).
				Description("Fields to examine for tag generation")).
			Field(service.NewBoolField("add_timestamp_tag").
				Default(true).
				Description("Add a tag with ingestion timestamp")).
			Field(service.NewBoolField("add_source_tag").
				Default(false).
				Description("Add tag indicating source (http_receiver)")),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchProcessor, error) {
			tagFields, err := conf.FieldStringList("tag_fields")
			if err != nil {
				return nil, err
			}
			addTimestamp, err := conf.FieldBool("add_timestamp_tag")
			if err != nil {
				return nil, err
			}
			addSource, err := conf.FieldBool("add_source_tag")
			if err != nil {
				return nil, err
			}

			return &PayloadTagger{
				tagFields:    tagFields,
				addTimestamp: addTimestamp,
				addSource:    addSource,
				logger:       mgr.Logger(),
			}, nil
		})
	if err != nil {
		panic(err)
	}
}

// PayloadTagger adds tags based on payload inspection
type PayloadTagger struct {
	tagFields    []string
	addTimestamp bool
	addSource    bool
	logger       *service.Logger
}

// ProcessBatch adds tags to all messages in a batch
func (p *PayloadTagger) ProcessBatch(ctx context.Context, batch service.MessageBatch) ([]service.MessageBatch, error) {
	for _, msg := range batch {
		if err := p.tagMessage(msg); err != nil {
			p.logger.Warnf("Failed to tag message: %v", err)
			// Continue processing other messages
		}
	}
	return []service.MessageBatch{batch}, nil
}

// tagMessage examines a message and adds appropriate tags
func (p *PayloadTagger) tagMessage(msg *service.Message) error {
	// Parse message as JSON
	data, err := msg.AsBytes()
	if err != nil {
		return fmt.Errorf("failed to get message bytes: %w", err)
	}

	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Add source tag if enabled
	if p.addSource {
		msg.MetaSet("tag.source", "http_receiver")
	}

	// Add timestamp tag if enabled
	if p.addTimestamp {
		if receivedAt, exists := msg.MetaGet("http.received_at"); exists {
			msg.MetaSet("tag.ingested_at", receivedAt)
		}
	}

	// Examine configured fields and add tags
	for _, field := range p.tagFields {
		if value, exists := event[field]; exists {
			tagKey := fmt.Sprintf("tag.%s", field)
			tagValue := fmt.Sprintf("%v", value)
			msg.MetaSet(tagKey, tagValue)

			// Add semantic tags based on known OCSF fields
			p.addSemanticTags(msg, field, value)
		}
	}

	// Add content-based tags
	p.addContentTags(msg, event)

	return nil
}

// addSemanticTags adds meaningful tags based on OCSF field values
func (p *PayloadTagger) addSemanticTags(msg *service.Message, field string, value interface{}) {
	switch field {
	case "class_uid":
		classUID, ok := value.(float64)
		if !ok {
			return
		}
		
		// Map class_uid to categories
		switch int(classUID) {
		case 2004:
			msg.MetaSet("tag.category", "detection")
			msg.MetaSet("tag.type", "alert")
		case 5001:
			msg.MetaSet("tag.category", "asset")
			msg.MetaSet("tag.type", "inventory")
		case 4001, 4002, 4003:
			msg.MetaSet("tag.category", "network")
			msg.MetaSet("tag.type", "activity")
		case 3001, 3002:
			msg.MetaSet("tag.category", "authentication")
		case 1001, 1002, 1003:
			msg.MetaSet("tag.category", "system")
			msg.MetaSet("tag.type", "process")
		}

	case "severity_id":
		severity, ok := value.(float64)
		if !ok {
			return
		}
		
		// Map severity to priority
		switch int(severity) {
		case 1:
			msg.MetaSet("tag.severity", "informational")
			msg.MetaSet("tag.priority", "low")
		case 2:
			msg.MetaSet("tag.severity", "low")
			msg.MetaSet("tag.priority", "low")
		case 3:
			msg.MetaSet("tag.severity", "medium")
			msg.MetaSet("tag.priority", "medium")
		case 4:
			msg.MetaSet("tag.severity", "high")
			msg.MetaSet("tag.priority", "high")
		case 5, 6:
			msg.MetaSet("tag.severity", "critical")
			msg.MetaSet("tag.priority", "critical")
		}

	case "category_uid":
		categoryUID, ok := value.(float64)
		if !ok {
			return
		}
		
		// Map category_uid to domain
		switch int(categoryUID) {
		case 1:
			msg.MetaSet("tag.domain", "system")
		case 2:
			msg.MetaSet("tag.domain", "findings")
		case 3:
			msg.MetaSet("tag.domain", "identity")
		case 4:
			msg.MetaSet("tag.domain", "network")
		case 5:
			msg.MetaSet("tag.domain", "discovery")
		}
	}
}

// addContentTags examines event content for additional tagging
func (p *PayloadTagger) addContentTags(msg *service.Message, event map[string]interface{}) {
	// Check for observables (IOCs)
	if observables, ok := event["observables"].([]interface{}); ok && len(observables) > 0 {
		msg.MetaSet("tag.has_observables", "true")
		msg.MetaSet("tag.observable_count", fmt.Sprintf("%d", len(observables)))

		// Check if any observables have threat intel
		for _, obs := range observables {
			if obsMap, ok := obs.(map[string]interface{}); ok {
				if _, hasThreatIntel := obsMap["threat_intel"]; hasThreatIntel {
					msg.MetaSet("tag.has_threat_intel", "true")
					msg.MetaSet("tag.threat_detected", "true")
					break
				}
			}
		}
	}

	// Check for enrichments
	if _, hasAsset := event["asset"]; hasAsset {
		msg.MetaSet("tag.enriched", "asset")
	}

	// Check for specific fields indicating importance
	if status, ok := event["status_id"].(float64); ok {
		if int(status) == 1 { // Success
			msg.MetaSet("tag.status", "success")
		} else {
			msg.MetaSet("tag.status", "failure")
		}
	}

	// Add message hash for deduplication tracking
	if msgID, ok := event["metadata"].(map[string]interface{}); ok {
		if uid, ok := msgID["uid"].(string); ok {
			msg.MetaSet("tag.event_id", uid)
		}
	}
}

// Close cleans up resources
func (p *PayloadTagger) Close(ctx context.Context) error {
	p.logger.Debug("Payload tagger closed")
	return nil
}
