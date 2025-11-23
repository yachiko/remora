# Deployment Guide

This guide covers deploying Remora in various environments.

## Deployment Options

1. **Docker** - Single container deployment
2. **Docker Compose** - Local development with database
3. **Kubernetes** - Production orchestration
4. **Cloud Platforms** - AWS, GCP, Azure

---

## Prerequisites

### All Deployments

- GitHub App created and configured
- Database (PostgreSQL, MySQL, or SQLite)
- HTTPS endpoint for webhooks (required for production)

### GitHub App Setup

1. Create GitHub App at https://github.com/settings/apps/new
2. Configure permissions:
   - Issues: Read & Write
   - Pull Requests: Read & Write
   - Metadata: Read
3. Subscribe to webhook events:
   - `issue_comment`
4. Generate private key and download
5. Note your App ID
6. Set webhook URL: `https://your-domain.com/webhook`
7. Set webhook secret

---

## Docker Deployment

See [deployments/docker/README.md](docker/README.md) for detailed Docker instructions.

### Quick Start

```bash
# Build
docker build -t remora:latest -f deployments/docker/Dockerfile .

# Run
docker run -d \
  --name remora \
  -p 8080:8080 \
  --env-file .env \
  remora:latest
```

---

## Kubernetes Deployment

See [deployments/kubernetes/README.md](kubernetes/README.md) for detailed Kubernetes instructions.

### Quick Start

```bash
# Create secrets
kubectl create secret generic remora-secrets \
  --from-literal=DATABASE_USER=remora_user \
  --from-literal=DATABASE_PASSWORD=secure_password \
  --from-literal=GITHUB_APP_ID=123456 \
  --from-file=GITHUB_APP_PRIVATE_KEY=./github-app-key.pem \
  --from-literal=GITHUB_WEBHOOK_SECRET=webhook_secret \
  -n remora

# Deploy
kubectl apply -k deployments/kubernetes/
```

---

## Cloud Platform Deployments

### AWS ECS

1. **Push image to ECR**:
```bash
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin 123456789.dkr.ecr.us-east-1.amazonaws.com
docker tag remora:latest 123456789.dkr.ecr.us-east-1.amazonaws.com/remora:latest
docker push 123456789.dkr.ecr.us-east-1.amazonaws.com/remora:latest
```

2. **Create task definition** with environment variables from Secrets Manager
3. **Create ECS service** with Application Load Balancer
4. **Configure target group** health checks to `/health`

### Google Cloud Run

```bash
# Build and push to GCR
gcloud builds submit --tag gcr.io/PROJECT_ID/remora

# Deploy
gcloud run deploy remora \
  --image gcr.io/PROJECT_ID/remora \
  --platform managed \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars DATABASE_TYPE=postgresql,DATABASE_HOST=... \
  --set-secrets=GITHUB_APP_PRIVATE_KEY=github-key:latest
```

### Azure Container Instances

```bash
# Push to ACR
az acr login --name myregistry
docker tag remora:latest myregistry.azurecr.io/remora:latest
docker push myregistry.azurecr.io/remora:latest

# Deploy
az container create \
  --resource-group myResourceGroup \
  --name remora \
  --image myregistry.azurecr.io/remora:latest \
  --dns-name-label remora-app \
  --ports 8080 \
  --environment-variables \
    DATABASE_TYPE=postgresql \
    DATABASE_HOST=... \
  --secure-environment-variables \
    GITHUB_APP_PRIVATE_KEY=... \
    DATABASE_PASSWORD=...
```

### Heroku

```bash
# Login to Heroku
heroku login
heroku container:login

# Create app
heroku create remora-app

# Add PostgreSQL
heroku addons:create heroku-postgresql:mini

# Set config vars
heroku config:set GITHUB_APP_ID=123456
heroku config:set GITHUB_WEBHOOK_SECRET=secret

# Deploy
heroku container:push web -a remora-app
heroku container:release web -a remora-app
```

---

## Database Setup

### PostgreSQL

#### Local PostgreSQL

```bash
# Create database and user
psql -U postgres
CREATE DATABASE remora;
CREATE USER remora_user WITH PASSWORD 'secure_password';
GRANT ALL PRIVILEGES ON DATABASE remora TO remora_user;
```

#### Cloud PostgreSQL

- **AWS RDS**: Use managed PostgreSQL instance
- **Google Cloud SQL**: Use Cloud SQL for PostgreSQL
- **Azure Database**: Use Azure Database for PostgreSQL

Configuration:
```bash
DATABASE_TYPE=postgresql
DATABASE_HOST=your-instance.region.provider.com
DATABASE_PORT=5432
DATABASE_NAME=remora
DATABASE_USER=remora_user
DATABASE_PASSWORD=secure_password
DATABASE_SSLMODE=require
```

### MySQL

#### Local MySQL

```bash
# Create database and user
mysql -u root -p
CREATE DATABASE remora;
CREATE USER 'remora_user'@'%' IDENTIFIED BY 'secure_password';
GRANT ALL PRIVILEGES ON remora.* TO 'remora_user'@'%';
FLUSH PRIVILEGES;
```

#### Cloud MySQL

Similar to PostgreSQL, use managed services.

### SQLite (Development Only)

```bash
DATABASE_TYPE=sqlite
DATABASE_NAME=./remora.db
```

**Warning**: SQLite not recommended for production due to concurrent write limitations.

---

## HTTPS/TLS Configuration

### Let's Encrypt with Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name remora.example.com;

    ssl_certificate /etc/letsencrypt/live/remora.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/remora.example.com/privkey.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Kubernetes with cert-manager

Already configured in `deployments/kubernetes/ingress.yaml`.

### Cloud Load Balancer

Most cloud platforms handle SSL termination at the load balancer level.

---

## Environment Configuration

### Production Environment Variables

```bash
# Database
DATABASE_TYPE=postgresql
DATABASE_HOST=prod-db.example.com
DATABASE_PORT=5432
DATABASE_NAME=remora
DATABASE_USER=remora_user
DATABASE_PASSWORD=<secure-password>
DATABASE_SSLMODE=require

# GitHub App
GITHUB_APP_ID=<your-app-id>
GITHUB_APP_PRIVATE_KEY=<pem-content>
GITHUB_WEBHOOK_SECRET=<webhook-secret>

# Server
REMORA_PORT=8080
ENVIRONMENT=production
LOG_LEVEL=info

# Scheduler
REMORA_SCHEDULER_INTERVAL=5

# Features
REMORA_ERROR_MODE=reaction_only
REMORA_POST_TO_CLOSED=true
REMORA_ENABLE_API=false
```

---

## Monitoring & Observability

### Health Checks

Configure monitoring systems to check:
- Liveness: `GET /health`
- Readiness: `GET /ready`

### Logging

Logs are output to stdout in structured format.

**Aggregation services**:
- CloudWatch Logs (AWS)
- Cloud Logging (GCP)
- Azure Monitor (Azure)
- Datadog, Splunk, etc.

### Metrics

When metrics are enabled (Phase 12), expose:
- `GET /metrics` - Prometheus format

---

## Backup & Disaster Recovery

### Database Backups

#### PostgreSQL

```bash
# Automated backups (cron)
pg_dump -h $DATABASE_HOST -U $DATABASE_USER $DATABASE_NAME > backup-$(date +%Y%m%d).sql

# Restore
psql -h $DATABASE_HOST -U $DATABASE_USER $DATABASE_NAME < backup-20251124.sql
```

#### Cloud Managed Databases

Enable automated backups:
- AWS RDS: Automated backups with point-in-time recovery
- Cloud SQL: Automated backups with retention
- Azure Database: Automated backups

### Disaster Recovery Plan

1. **Database**: Regular backups (daily), retained for 30 days
2. **Secrets**: Store in secret manager with backup
3. **Configuration**: Version controlled in Git
4. **Docker Images**: Tagged and stored in registry

**RTO**: 1 hour (Recovery Time Objective)  
**RPO**: 24 hours (Recovery Point Objective)

---

## Security Best Practices

### Secrets Management

**Do NOT**:
- Commit secrets to Git
- Use default passwords
- Share secrets in plain text

**Do**:
- Use secret managers (AWS Secrets Manager, HashiCorp Vault)
- Rotate secrets regularly
- Use strong, unique passwords
- Enable encryption at rest

### Network Security

1. **Firewall**: Only expose port 443 (HTTPS)
2. **HTTPS Only**: Always use TLS for webhooks
3. **Private Database**: Don't expose database to internet
4. **VPC/Private Network**: Deploy in private subnet

### Container Security

1. **Non-root User**: Already configured in Dockerfile
2. **Read-only Filesystem**: Enable if possible
3. **Resource Limits**: Set CPU and memory limits
4. **Security Scanning**: Scan images for vulnerabilities

---

## Scaling Considerations

### Single Instance (Phase 1)

Remora Phase 1 is designed for **single instance only**.

**Limitations**:
- No horizontal scaling
- No distributed locking
- In-process scheduler

**Sufficient for**:
- Small to medium teams
- Hundreds of repositories
- Thousands of reminders

### Future Scaling (Phase 14)

Multi-instance support requires:
- Distributed locking (Redis)
- Leader election
- Shared state management

---

## Troubleshooting

### Common Issues

#### Database Connection Failed

```bash
# Check connection
psql -h $DATABASE_HOST -U $DATABASE_USER -d $DATABASE_NAME

# Verify credentials
echo $DATABASE_PASSWORD

# Check network
ping $DATABASE_HOST
```

#### GitHub Webhook Not Working

1. Verify webhook URL in GitHub App settings
2. Check webhook signature validation
3. Review logs for errors
4. Test endpoint: `curl -X POST https://your-domain.com/webhook`

#### High Memory Usage

```bash
# Check container stats
docker stats

# Reduce memory if needed
docker run --memory="512m" ...
```

#### Scheduler Not Firing Reminders

1. Check logs for scheduler errors
2. Verify database connectivity
3. Check for overdue reminders: Query database
4. Ensure GitHub API credentials are valid

---

## Maintenance

### Updates

```bash
# Pull new image
docker pull your-registry/remora:latest

# Recreate container
docker-compose up -d --force-recreate

# Or in Kubernetes
kubectl rollout restart deployment/remora -n remora
```

### Database Migrations

Remora uses GORM auto-migrations. On startup:
1. Connects to database
2. Runs migrations automatically
3. Creates/updates schema as needed

**No manual migration needed**.

---

## Cost Estimation

### Infrastructure Costs

**Minimal Deployment** (single small instance):
- Compute: $10-20/month (1 vCPU, 512MB RAM)
- Database: $15-30/month (managed PostgreSQL)
- Load Balancer: $10-20/month (if needed)
- **Total**: ~$35-70/month

**Cloud Platform Examples**:
- **AWS**: t3.micro ECS + RDS db.t3.micro = ~$40/month
- **GCP**: Cloud Run + Cloud SQL = ~$30/month
- **Azure**: Container Instance + Database = ~$35/month
- **Heroku**: Hobby Dynos + Mini PostgreSQL = ~$16/month

---

## Checklist

### Pre-Deployment

- [ ] GitHub App created and configured
- [ ] Database provisioned
- [ ] Environment variables configured
- [ ] Secrets stored securely
- [ ] HTTPS/TLS certificate obtained
- [ ] Domain configured

### Post-Deployment

- [ ] Health check responding (200 OK)
- [ ] Webhook endpoint accessible from GitHub
- [ ] Test reminder creation
- [ ] Verify reminder fires
- [ ] Logs aggregation configured
- [ ] Monitoring/alerting set up
- [ ] Backup strategy implemented
- [ ] Documentation updated

---

## Support

For deployment issues:
1. Check logs
2. Verify configuration
3. Test endpoints manually
4. Review documentation
5. Open GitHub issue with logs and configuration (redact secrets!)
