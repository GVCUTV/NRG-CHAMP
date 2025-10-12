// v1
// README.md
# NRG-CHAMP
Official repository for the NRG CHAMP system.

## Description
NRG-CHAMP is a Responsive Goal-driven Cooling and Heating Automated Monitoring Platform.

## Running Gamification Locally

The gamification service can be built and launched alongside Kafka and Ledger via Docker Compose:

1. Build the service image from the repository root:
   ```sh
   docker compose build gamification
   ```
2. Start the service and its dependencies:
   ```sh
   docker compose up -d gamification
   ```
3. Once the container is healthy, access the HTTP API (default port `8085`) from the host, for example:
   ```sh
   curl http://localhost:8085/health
   ```

The compose configuration injects the required environment variables so that gamification can consume ledger epochs from Kafka and expose leaderboards on port `8085`.
