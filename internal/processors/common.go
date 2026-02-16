package processors

// Shared utilities used by custom processors.
// This file contains common HTTP and JSON utilities shared across all enrichment processors.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpClient is a shared HTTP client with a default 5-second timeout.
// Used by all processors to make HTTP requests to external APIs.
// The timeout can be overridden per-request using context.WithTimeout().
var httpClient = &http.Client{Timeout: 5 * time.Second}

// fetchJSON performs an HTTP GET request and parses the response as JSON.
// Used by enrichment processors to fetch data from external APIs.
//
// The function:
//  1. Creates an HTTP GET request with the provided context (for cancellation/timeout)
//  2. Sets the Accept header to "application/json"
//  3. Executes the request and validates the status code
//  4. Parses the response body as JSON into a map
//  5. Returns an error for HTTP errors, parse errors, or empty responses
//
// Error handling:
//   - HTTP status >= 400: Returns error with status code and response body (limited to 1KB)
//   - Network errors: Returns error from HTTP client
//   - JSON parse errors: Returns error from decoder
//   - Empty/null response: Returns "empty body" error
//
// Parameters:
//   - ctx: Context for request cancellation and timeout (use context.WithTimeout())
//   - url: Full URL to fetch (e.g., "http://localhost:1080/asset/550e8400-e29b-41d4-a716-446655440001")
//
// Returns:
//   - map[string]interface{}: Parsed JSON response as a map
//   - error: Any HTTP, network, or parsing errors
//
// Example usage:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
//	defer cancel()
//	data, err := fetchJSON(ctx, "http://api.example.com/asset/123")
//	if err != nil {
//	    log.Printf("Failed to fetch asset: %v", err)
//	    return
//	}
//	fmt.Printf("Asset hostname: %s", data["hostname"])
func fetchJSON(ctx context.Context, url string) (map[string]interface{}, error) {
	// Create HTTP request with context for cancellation/timeout
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Set Accept header to indicate we expect JSON
	req.Header.Set("Accept", "application/json")

	// Execute HTTP request
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// Check for HTTP error status codes
	if res.StatusCode >= 400 {
		// Read up to 1KB of error response body for context
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return nil, fmt.Errorf("http status %d: %s", res.StatusCode, string(b))
	}

	// Decode JSON response into map
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}

	// Validate that response is not empty/null
	if out == nil {
		return nil, errors.New("empty body")
	}

	return out, nil
}
