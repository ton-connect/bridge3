
````yaml

````

## Usage

````bash
# Start everything
docker-compose -f docker-compose-kafka.yml up -d

# Check logs
docker-compose -f docker-compose-kafka.yml logs -f kafka
docker-compose -f docker-compose-kafka.yml logs -f kafka-ui

# Stop everything
docker-compose -f docker-compose-kafka.yml down

# Stop and remove volumes (clean slate)
docker-compose -f docker-compose-kafka.yml down -v
````

## Access

- **Kafka**: `localhost:9092`
- **Kafka UI**: `http://localhost:8080`

## Test It

````bash
# Wait for services to be healthy, then create a topic
docker exec kafka kafka-topics --create --topic sse-messages --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1

# Send a test message
echo "Hello from Docker Kafka!" | docker exec -i kafka kafka-console-producer --topic sse-messages --bootstrap-server localhost:9092
````

```
kafka-topics \
  --create \
  --topic bridge-messages \
  --bootstrap-server localhost:9092 \
  --partitions 1000 \
  --replication-factor 1 \
  --config retention.ms=600000 \
  --config segment.ms=300000 \
  --config cleanup.policy=delete
```