#!/bin/bash
# Test script for HTTP receiver
# Sends test events to the HTTP receiver endpoint

ENDPOINT="${1:-http://localhost:8080/events}"

echo "=== Testing HTTP Receiver ==="
echo "Endpoint: $ENDPOINT"
echo

# Test 1: Single event
echo "Test 1: Sending single event..."
curl -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -d '{
    "metadata": {"version": "1.0.0", "uid": "test-001"},
    "class_uid": 2004,
    "severity_id": 4,
    "category_uid": 2,
    "message": "Malware detected",
    "observables": [
      {"name": "evil.exe", "type_id": 7}
    ]
  }' | jq .
echo

# Test 2: Batch of events
echo "Test 2: Sending batch of 3 events..."
curl -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -d '[
    {
      "metadata": {"version": "1.0.0", "uid": "test-002"},
      "class_uid": 2004,
      "severity_id": 5,
      "category_uid": 2,
      "message": "Critical threat detected",
      "observables": [
        {"name": "c2-server.com", "type_id": 4}
      ]
    },
    {
      "metadata": {"version": "1.0.0", "uid": "test-003"},
      "class_uid": 5001,
      "severity_id": 1,
      "category_uid": 5,
      "message": "New asset discovered",
      "device": {"uid": "server-123"}
    },
    {
      "metadata": {"version": "1.0.0", "uid": "test-004"},
      "class_uid": 3001,
      "severity_id": 3,
      "category_uid": 3,
      "message": "Failed login attempt"
    }
  ]' | jq .
echo

# Test 3: High severity event
echo "Test 3: Sending high severity event..."
curl -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -d '{
    "metadata": {"version": "1.0.0", "uid": "test-005"},
    "class_uid": 2004,
    "severity_id": 6,
    "category_uid": 2,
    "message": "Ransomware activity detected",
    "observables": [
      {"name": "10.0.0.1", "type_id": 2},
      {"name": "ransomware.exe", "type_id": 7}
    ]
  }' | jq .
echo

# Test 4: Event with threat intel (already enriched)
echo "Test 4: Sending event with threat intel..."
curl -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -d '{
    "metadata": {"version": "1.0.0", "uid": "test-006"},
    "class_uid": 2004,
    "severity_id": 4,
    "category_uid": 2,
    "message": "Known malicious IP detected",
    "observables": [
      {
        "name": "192.168.1.100",
        "type_id": 2,
        "threat_intel": {
          "severity": "high",
          "threat_type": "C2 Server",
          "source": "threat_feed"
        }
      }
    ]
  }' | jq .
echo

echo "=== Tests Complete ==="
echo
echo "Check the pipeline logs to see tagged events"
echo "Check Kafka topic 'events.tagged' to see the output"
echo
echo "To consume from Kafka:"
echo "  docker compose exec redpanda rpk topic consume events.tagged --format '%v\n'"
