# Clear all messages
for topic in $(docker compose exec -T redpanda rpk topic list | tail -n +2 | awk '{print $1}'); do
  docker compose exec redpanda rpk topic alter-config "$topic" --set retention.ms=1
done

sleep 5

# Restore normal retention (7 days = 604800000ms)
for topic in $(docker compose exec -T redpanda rpk topic list | tail -n +2 | awk '{print $1}'); do
  docker compose exec redpanda rpk topic alter-config "$topic" --set retention.ms=604800000
done