package inputs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/warpstreamlabs/bento/public/service"
)

// HTTPReceiver receives events via HTTP POST requests
// Supports both single events and batches
func init() {
	err := service.RegisterInput(
		"http_receiver",
		service.NewConfigSpec().
			Summary("HTTP server that receives events via POST requests").
			Description("Accepts JSON events in request body. Supports both single events and arrays of events.").
			Field(service.NewStringField("address").
				Default("0.0.0.0:8080").
				Description("Address to bind the HTTP server to")).
			Field(service.NewStringField("path").
				Default("/events").
				Description("HTTP path to accept events on")).
			Field(service.NewDurationField("timeout").
				Default("30s").
				Description("Maximum time to wait for events before returning empty")).
			Field(service.NewIntField("max_batch_size").
				Default(100).
				Description("Maximum number of events to accept in a single request")),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Input, error) {
			address, err := conf.FieldString("address")
			if err != nil {
				return nil, err
			}
			path, err := conf.FieldString("path")
			if err != nil {
				return nil, err
			}
			timeout, err := conf.FieldDuration("timeout")
			if err != nil {
				return nil, err
			}
			maxBatchSize, err := conf.FieldInt("max_batch_size")
			if err != nil {
				return nil, err
			}

			return &HTTPReceiver{
				address:      address,
				path:         path,
				timeout:      timeout,
				maxBatchSize: maxBatchSize,
				eventChan:    make(chan []byte, 100),
				logger:       mgr.Logger(),
			}, nil
		})
	if err != nil {
		panic(err)
	}
}

// HTTPReceiver is an HTTP server that receives events
type HTTPReceiver struct {
	address      string
	path         string
	timeout      time.Duration
	maxBatchSize int
	eventChan    chan []byte
	server       *http.Server
	logger       *service.Logger
	wg           sync.WaitGroup
}

// Connect starts the HTTP server
func (h *HTTPReceiver) Connect(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(h.path, h.handleEvents)
	mux.HandleFunc("/health", h.handleHealth)

	h.server = &http.Server{
		Addr:         h.address,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.logger.Infof("HTTP receiver listening on http://%s%s", h.address, h.path)
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			h.logger.Errorf("HTTP server error: %v", err)
		}
	}()

	return nil
}

// handleHealth provides a health check endpoint
func (h *HTTPReceiver) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// handleEvents processes incoming HTTP POST requests with events
func (h *HTTPReceiver) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Errorf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Check if it's a batch (array) or single event
	var events []json.RawMessage
	if err := json.Unmarshal(body, &events); err != nil {
		// Not an array, try as single event
		var singleEvent json.RawMessage
		if err := json.Unmarshal(body, &singleEvent); err != nil {
			h.logger.Errorf("Invalid JSON: %v", err)
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		events = []json.RawMessage{singleEvent}
	}

	// Validate batch size
	if len(events) > h.maxBatchSize {
		h.logger.Warnf("Batch size %d exceeds maximum %d", len(events), h.maxBatchSize)
		http.Error(w, fmt.Sprintf("Batch size exceeds maximum of %d", h.maxBatchSize),
			http.StatusRequestEntityTooLarge)
		return
	}

	// Queue events for processing
	accepted := 0
	for _, event := range events {
		select {
		case h.eventChan <- []byte(event):
			accepted++
		default:
			// Channel full, reject request
			h.logger.Warnf("Event queue full, accepted %d/%d events", accepted, len(events))
			http.Error(w, "Event queue full, try again later", http.StatusServiceUnavailable)
			return
		}
	}

	h.logger.Debugf("Accepted %d events from %s", accepted, r.RemoteAddr)

	// Success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	response := map[string]interface{}{
		"status":   "accepted",
		"count":    accepted,
		"received": time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

// Read returns the next event from the queue
func (h *HTTPReceiver) Read(ctx context.Context) (*service.Message, service.AckFunc, error) {
	select {
	case event := <-h.eventChan:
		msg := service.NewMessage(event)
		
		// Add metadata about ingestion
		msg.MetaSet("http.received_at", time.Now().Format(time.RFC3339))
		
		return msg, func(ctx context.Context, err error) error {
			// Acknowledgement - could track delivery here
			return nil
		}, nil
		
	case <-time.After(h.timeout):
		// Timeout waiting for events - return nil to allow graceful shutdown check
		return nil, nil, service.ErrNotConnected
		
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

// Close shuts down the HTTP server
func (h *HTTPReceiver) Close(ctx context.Context) error {
	h.logger.Info("Shutting down HTTP receiver")
	
	if h.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		if err := h.server.Shutdown(shutdownCtx); err != nil {
			h.logger.Errorf("Error shutting down HTTP server: %v", err)
		}
	}
	
	h.wg.Wait()
	close(h.eventChan)
	
	return nil
}
