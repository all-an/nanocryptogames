# nim_nano_crypto_local_db

Local PostgreSQL setup for Nano Crypto Games development.

## 1. Start Postgres

```bash
docker run -d \
  --name nano_crypto_games_local_db_container \
  -e POSTGRES_USER=nano \
  -e POSTGRES_PASSWORD=nano \
  -e POSTGRES_DB=nano_crypto_games_local_db \
  -p 5432:5432 \
  postgres:16
```

## Useful commands

```bash
# Stop the container
docker stop nano_crypto_games_local_db_container

# Start it again
docker start nano_crypto_games_local_db_container

# Connect with psql
docker exec -it nano_crypto_games_local_db_container psql -U nano -d nano_crypto_games_local_db
```

## Connection string

```
postgres://nano:nano@localhost:5432/nano_crypto_games_local_db
```
