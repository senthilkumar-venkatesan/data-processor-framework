# Publish messages to kafka
cat test-events.json| docker compose exec -T redpanda rpk topic produce events.raw 

jq -s '.' test-events.json | curl -X POST http://localhost:9081/events \
  -H "Content-Type: application/json" \
  -d @-
{"count":41,"received":"2026-02-17T10:25:32+05:30","status":"accepted"}