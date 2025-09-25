# Utility scripts for Kafka
This directory contains utility scripts for managing and interacting with Kafka services. These scripts can help with tasks such as monitoring, configuration, and maintenance of Kafka clusters.

## Topics
### List all Kafka topics
From inside the Kafka container:
```bash
kafka-topics.sh --bootstrap-server localhost:9092 --list
```
or else directly via Docker:
```bash
docker exec -it kafka kafka-topics.sh --bootstrap-server localhost:9092 --list
```

### Describe a Kafka topic
From inside the Kafka container:
```bash
kafka-topics.sh --bootstrap-server localhost:9092 --describe --topic <topic_name>
```
or else directly via Docker:
```bash
docker exec -it kafka kafka-topics.sh --bootstrap-server localhost:9092 --describe --topic <topic_name>
```
## Consumer
### Consume messages from a Kafka topic
From inside the Kafka container:
```bash
kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic <topic_name>
```
or else directly via Docker:
```bash
docker exec -it kafka kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic <topic_name>
```