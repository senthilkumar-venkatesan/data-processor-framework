# Kafka / Redpanda + MockServer Quickstart

Topics
- events.raw (input)
- events.high_priority (output if priority > 7)
- events.standard (output otherwise)

Create topics
- docker compose exec redpanda rpk topic create events.raw events.high_priority events.standard

Produce sample messages into events.raw
- docker compose exec -T redpanda rpk topic produce events.raw <<'EOF'
{"asset_id":"a-1","indicator":"bad.com","category":"security","priority":9}
{"asset_id":"a-2","indicator":"evil.org","category":"finance","priority":8}
{"asset_id":"a-3","indicator":"ok.com","category":"security","priority":3}
{"asset_id":"a-4","indicator":"ok.org","category":"finance","priority":5}
EOF

MockServer endpoints (used by enrichers)
- Asset: http://localhost:1080/asset/{id}
- Threat intel: http://localhost:1080/ioc/{indicator}

Start the pipeline (from repo root)
- LOG_LEVEL=DEBUG \
  BENTO_CONFIG_PATH=config/pipeline.yaml \
  go run ./...
  - Optional overrides (defaults already set in pipeline):
    - ASSET_API=http://localhost:1080/asset
    - THREAT_INTEL_API=http://localhost:1080/ioc

Consume outputs (optional)
- docker compose exec redpanda rpk topic consume events.high_priority -f '%v\n'
- docker compose exec redpanda rpk topic consume events.standard -f '%v\n'

Notes
- Broker for host clients: 127.0.0.1:19092 (per docker-compose external listener).
- Do not remove the volume redpanda_data if you want topics and offsets to persist across restarts.
- If MockServer expectations change, run: docker compose restart mockserver.
