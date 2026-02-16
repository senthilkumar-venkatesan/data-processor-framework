#!/bin/bash

echo "=== Publishing NEW test event with threat intel observables ==="
cat <<EOF | docker compose exec -T redpanda rpk topic produce events.raw
{"class_uid": 2004, "category": "security", "severity_id": 5, "finding": {"title": "NEW TEST $(date +%s)"}, "observables": [{"name": "apt-tool.exe", "type_id": 7}, {"name": "c2-server.com", "type_id": 1}, {"name": "10.0.0.1", "type_id": 2}], "device": {"uid": "550e8400-e29b-41d4-a716-446655440006"}}
EOF

echo ""
echo "=== Waiting 2 seconds for processing ==="
sleep 2

echo ""
echo "=== Checking latest alert for threat intel enrichment ==="
docker compose exec -T redpanda rpk topic consume events.alerts --num 1 --offset end --format json 2>/dev/null | \
  jq -r '.value | fromjson | {
    finding: .finding.title,
    observables: .observables | map({name, type_id}),
    threat_intel_count: (.threat_intel_observables | length // 0),
    threat_intel_observables: .threat_intel_observables
  }'
