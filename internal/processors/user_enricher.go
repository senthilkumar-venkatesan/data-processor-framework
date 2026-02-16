package processors

import (
	"context"
	"fmt"
	"time"

	"github.com/warpstreamlabs/bento/public/service"
)

// User enricher fetches user metadata from an HTTP endpoint and attaches it to events.
func init() {
	if err := service.RegisterProcessor("user_enricher", userEnricherConfig(), newUserEnricher); err != nil {
		panic(err)
	}
}

func userEnricherConfig() *service.ConfigSpec {
	return service.NewConfigSpec().
		Summary("Enriches events with user metadata from an HTTP service").
		Field(service.NewStringField("endpoint").Description("Base URL, e.g. https://user-api/users")).
		Field(service.NewStringField("id_field").Description("Field containing user id").Default("user_id")).
		Field(service.NewDurationField("timeout").Description("HTTP timeout").Default("5s"))
}

type userEnricher struct {
	endpoint string
	idField  string
	timeout  time.Duration
	logger   *service.Logger
}

// newUserEnricher constructs the user enricher from parsed config and shared resources.
func newUserEnricher(conf *service.ParsedConfig, res *service.Resources) (service.Processor, error) {
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
	return &userEnricher{endpoint: endpoint, idField: idField, timeout: timeout, logger: res.Logger()}, nil
}

// Process enriches a message with user metadata when a user id is present.
func (p *userEnricher) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	body, err := msg.AsStructured()
	if err != nil {
		return nil, err
	}
	obj, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("user_enricher expects object")
	}

	rawID, ok := obj[p.idField]
	if !ok {
		return service.MessageBatch{msg}, nil
	}
	userID, ok := rawID.(string)
	if !ok || userID == "" {
		return service.MessageBatch{msg}, nil
	}

	c, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	url := fmt.Sprintf("%s/%s", p.endpoint, userID)
	data, err := fetchJSON(c, url)
	if err != nil {
		p.logger.Warnf("user_enrich failed for %s: %v", userID, err)
		msg.MetaSet("user_enrich_error", err.Error())
		return service.MessageBatch{msg}, nil
	}

	obj["user"] = data
	msg.SetStructured(obj)
	return service.MessageBatch{msg}, nil
}

func (p *userEnricher) Close(ctx context.Context) error { return nil }
