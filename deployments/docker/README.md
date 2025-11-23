# Docker Deployment

This directory contains Docker configurations for building and running Remora.

## Quick Start

### Prerequisites

- Docker 20.10+
- Docker Compose (optional, for local development)

### Build Docker Image

```bash
# From project root
make docker-build

# Or directly with docker
docker build -t remora:latest -f deployments/docker/Dockerfile .
```

### Run with Docker

#### Option 1: Docker Run (SQLite - Simplest)

```bash
docker run --rm \
  -p 8080:8080 \
  -e DATABASE_TYPE=sqlite \
  -e DATABASE_NAME=/data/remora.db \
  -e GITHUB_APP_ID=your_app_id \
  -e GITHUB_APP_PRIVATE_KEY="$(cat github-app-key.pem)" \
  -e GITHUB_WEBHOOK_SECRET=your_webhook_secret \
  -e ENVIRONMENT=production \
  -e LOG_LEVEL=info \
  -v $(pwd)/data:/data \
  remora:latest
```

#### Option 2: Docker Run with External Database

```bash
docker run --rm \
  -p 8080:8080 \
  -e DATABASE_TYPE=postgresql \
  -e DATABASE_HOST=your-postgres-host \
  -e DATABASE_PORT=5432 \
  -e DATABASE_NAME=remora \
  -e DATABASE_USER=remora_user \
  -e DATABASE_PASSWORD=secure_password \
  -e DATABASE_SSLMODE=require \
  -e GITHUB_APP_ID=your_app_id \
  -e GITHUB_APP_PRIVATE_KEY="$(cat github-app-key.pem)" \
  -e GITHUB_WEBHOOK_SECRET=your_webhook_secret \
  -e ENVIRONMENT=production \
  -e LOG_LEVEL=info \
  remora:latest
```

#### Option 3: Docker Run with .env file

```bash
# Create .env file first (see .env.example)
docker run --rm \
  -p 8080:8080 \
  --env-file .env \
  remora:latest
```

### Run with Docker Compose

#### PostgreSQL (Default)

```bash
# Start PostgreSQL and Remora
docker-compose -f deployments/docker/docker-compose.yml up

# Run in background
docker-compose -f deployments/docker/docker-compose.yml up -d

# View logs
docker-compose -f deployments/docker/docker-compose.yml logs -f remora

# Stop services
docker-compose -f deployments/docker/docker-compose.yml down
```

#### MySQL (Alternative)

```bash
# Start MySQL and Remora
docker-compose -f deployments/docker/docker-compose.yml --profile mysql up

# Run in background
docker-compose -f deployments/docker/docker-compose.yml --profile mysql up -d
```

## Configuration

### Environment Variables

See `.env.example` for full list of configuration options.

**Required**:
- `GITHUB_APP_ID` - Your GitHub App ID
- `GITHUB_APP_PRIVATE_KEY` - GitHub App private key (PEM format)
- `GITHUB_WEBHOOK_SECRET` - Webhook secret for signature validation

**Database** (choose one):

For SQLite:
```bash
DATABASE_TYPE=sqlite
DATABASE_NAME=/data/remora.db
```

For PostgreSQL:
```bash
DATABASE_TYPE=postgresql
DATABASE_HOST=postgres-host
DATABASE_PORT=5432
DATABASE_NAME=remora
DATABASE_USER=remora_user
DATABASE_PASSWORD=secure_password
DATABASE_SSLMODE=require
```

For MySQL:
```bash
DATABASE_TYPE=mysql
DATABASE_HOST=mysql-host
DATABASE_PORT=3306
DATABASE_NAME=remora
DATABASE_USER=remora_user
DATABASE_PASSWORD=secure_password
```

### Volumes

#### SQLite Persistence

```bash
docker run -v $(pwd)/data:/data ...
```

#### PostgreSQL/MySQL (Docker Compose)

Data is automatically persisted in named volumes:
- `postgres_data` - PostgreSQL data
- `mysql_data` - MySQL data

## Health Checks

```bash
# Check if container is healthy
docker ps

# Manual health check
curl http://localhost:8080/health

# Readiness check
curl http://localhost:8080/ready
```

## Logs

```bash
# Follow logs
docker logs -f container_name

# With docker-compose
docker-compose -f deployments/docker/docker-compose.yml logs -f remora

# View last 100 lines
docker logs --tail 100 container_name
```

## Multi-stage Build Details

The Dockerfile uses a multi-stage build:

### Stage 1: Builder
- Base: `golang:1.25-alpine`
- Compiles Go binary with static linking
- Includes all build dependencies

### Stage 2: Runtime
- Base: `alpine:3.19`
- Minimal runtime image (~20MB)
- Only includes:
  - Compiled binary
  - CA certificates (for HTTPS)
  - Timezone data
- Runs as non-root user (UID 1000)

## Security Features

1. **Non-root User**: Container runs as user `remora` (UID 1000)
2. **Static Binary**: No dynamic dependencies, reduced attack surface
3. **Minimal Base**: Alpine Linux with only essential packages
4. **Health Checks**: Built-in Docker health check
5. **Read-only Filesystem**: Can be run with `--read-only` flag

## Production Deployment

### Best Practices

1. **Use External Database**: Don't use SQLite in production
2. **Enable TLS**: Always use HTTPS for webhook endpoint
3. **Resource Limits**: Set CPU and memory limits
4. **Secrets Management**: Use Docker secrets or secret managers
5. **Monitoring**: Collect logs and metrics
6. **Backups**: Regular database backups

### Example Production Run

```bash
docker run -d \
  --name remora \
  --restart unless-stopped \
  --memory="512m" \
  --cpus="0.5" \
  -p 8080:8080 \
  --env-file /secure/path/.env \
  -v /var/log/remora:/var/log/remora \
  remora:latest
```

### Docker Compose Production

Update `docker-compose.yml`:

```yaml
services:
  remora:
    image: your-registry/remora:v1.0.0
    restart: always
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 512M
```

## Troubleshooting

### Container won't start

```bash
# Check logs
docker logs container_name

# Inspect container
docker inspect container_name

# Check health
docker inspect --format='{{json .State.Health}}' container_name
```

### Database connection issues

```bash
# Test from within container
docker exec -it container_name sh
wget -O- http://localhost:8080/health

# Check network
docker network ls
docker network inspect bridge
```

### GitHub webhook not working

1. Verify `GITHUB_WEBHOOK_SECRET` is correct
2. Check logs for signature validation errors
3. Ensure container is accessible from internet
4. Verify GitHub App webhook URL configuration

## Development

### Local Development with Docker

```bash
# Build and run
docker-compose -f deployments/docker/docker-compose.yml up --build

# Rebuild after code changes
docker-compose -f deployments/docker/docker-compose.yml up --build --force-recreate
```

### Debug Mode

```bash
docker-compose -f deployments/docker/docker-compose.yml up
# Logs show DEBUG level output
```

## Image Size Optimization

Current image size: ~30-40 MB

- Go binary: ~15-20 MB
- Alpine base: ~7 MB
- CA certs + timezone data: ~5 MB

## Cleanup

```bash
# Stop and remove containers
docker-compose -f deployments/docker/docker-compose.yml down

# Remove volumes (WARNING: deletes data)
docker-compose -f deployments/docker/docker-compose.yml down -v

# Remove images
docker rmi remora:latest
```

## Registry Push

```bash
# Tag for registry
docker tag remora:latest your-registry/remora:v1.0.0
docker tag remora:latest your-registry/remora:latest

# Push to registry
docker push your-registry/remora:v1.0.0
docker push your-registry/remora:latest
```

## Support

For issues and questions:
- Check logs first: `docker logs -f container_name`
- Review environment variables
- Verify database connectivity
- Check GitHub App configuration
