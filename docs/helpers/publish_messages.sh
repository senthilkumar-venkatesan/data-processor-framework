# Publish messages to kafka
cat test-events.json| docker compose exec -T redpanda rpk topic produce events.raw 