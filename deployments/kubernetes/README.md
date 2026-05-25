# Kubernetes Deployment

This directory contains Kubernetes manifests for deploying Remora.

## Prerequisites

- Kubernetes cluster (1.25+)
- kubectl configured
- GitHub App created and configured
- (Optional) External PostgreSQL database - or use included StatefulSet

## Architecture

The deployment includes:
- **Remora application** - Deployment with single replica
- **PostgreSQL database** - StatefulSet with persistent storage (optional - can use external DB)
- **Services** - For both Remora and PostgreSQL
- **ConfigMap** - Application configuration
- **Secrets** - Sensitive credentials
- **ServiceAccount** - RBAC configuration
- **Ingress** - (Optional) External access

## Deployment Steps

### 1. Create Namespace

```bash
kubectl apply -f namespace.yaml
```

### 2. Create Secrets

**Option A: From files**

```bash
kubectl create secret generic remora-secret \
  --from-literal=DATABASE_USER=remora_user \
  --from-literal=DATABASE_PASSWORD=your_secure_password \
  --from-literal=GITHUB_APP_ID=123456 \
  --from-file=GITHUB_APP_PRIVATE_KEY=./github-app-key.pem \
  --from-literal=GITHUB_WEBHOOK_SECRET=your_webhook_secret \
  --from-literal=REMORA_API_SECRET=your_api_secret \
  -n remora
```

**Option B: From YAML (update secret.yaml first)**

```bash
kubectl apply -f secret.yaml
```

### 3. Update ConfigMap

Edit `configmap.yaml` to match your environment:
- Database host and configuration (default: postgres-service.remora.svc.cluster.local)
- Logging level
- Feature flags

```bash
kubectl apply -f configmap.yaml
```

### 4. Deploy PostgreSQL StatefulSet (Optional)

If using the included PostgreSQL StatefulSet:

```bash
kubectl apply -f postgres-statefulset.yaml
```

**Note:** For production, consider using a cloud-managed database (AWS RDS, GCP Cloud SQL, Azure Database) instead.

To skip the StatefulSet and use external PostgreSQL:
- Update `configmap.yaml` with your database host
- Remove `postgres-statefulset.yaml` from `kustomization.yaml`

### 5. Deploy Application

```bash
kubectl apply -f serviceaccount.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
```

### 5. Configure Ingress (Optional)

Update `ingress.yaml` with your domain:

```bash
# Edit ingress.yaml first
kubectl apply -f ingress.yaml
```

### 6. Verify Deployment

```bash
# Check pod status
kubectl get pods -n remora

# Check logs
kubectl logs -n remora -l app=remora -f

# Check service
kubectl get svc -n remora

# Port forward for testing
kubectl port-forward -n remora svc/remora 8080:8080
```

## Using Kustomize

Deploy everything at once:

```bash
# Update kustomization.yaml with your image registry
kubectl apply -k .
```

## Monitoring

### Health Checks

```bash
# Liveness probe
curl http://localhost:8080/health

# Readiness probe
curl http://localhost:8080/ready
```

### Logs

```bash
# Stream logs
kubectl logs -n remora -l app=remora -f

# Get recent logs
kubectl logs -n remora -l app=remora --tail=100
```

### Metrics (if enabled)

```bash
# Port forward to metrics endpoint
kubectl port-forward -n remora svc/remora 8080:8080

# Access metrics
curl http://localhost:8080/metrics
```

## Scaling Considerations

**IMPORTANT**: Remora Phase 1 supports **single instance only**.

The deployment is configured with:
- `replicas: 1`
- `strategy.type: Recreate`

Do not scale to multiple replicas without implementing distributed locking (Phase 14).

## Database Configuration

### Option 1: In-Cluster PostgreSQL (Development/Testing)

The included `postgres-statefulset.yaml` provides a simple PostgreSQL deployment:

```yaml
# Already configured in configmap.yaml
DATABASE_HOST: "postgres-service.remora.svc.cluster.local"
DATABASE_PORT: "5432"
```

**Storage Configuration:**
- Default: 10Gi PersistentVolumeClaim
- Customize storage class by uncommenting `storageClassName` in `postgres-statefulset.yaml`

**Note:** This is suitable for development/testing but not recommended for production.

### Option 2: External PostgreSQL (Production)

Update `configmap.yaml`:

```yaml
DATABASE_HOST: "your-postgres-host.example.com"
DATABASE_PORT: "5432"
DATABASE_SSLMODE: "require"
```

### Option 3: Cloud-Managed Database (AWS RDS, GCP Cloud SQL, etc.)

Update secrets with connection details:

```bash
kubectl create secret generic remora-secrets \
  --from-literal=DATABASE_HOST=your-db-instance.region.rds.amazonaws.com \
  --from-literal=DATABASE_USER=remora_user \
  --from-literal=DATABASE_PASSWORD=secure_password \
  -n remora
```

## Troubleshooting

### Pod not starting

```bash
# Check pod events
kubectl describe pod -n remora -l app=remora

# Check logs
kubectl logs -n remora -l app=remora
```

### Database connection issues

```bash
# Verify secrets
kubectl get secret remora-secrets -n remora -o yaml

# Test database connectivity (from pod)
kubectl exec -it -n remora deployment/remora -- sh
# wget -O- http://localhost:8080/health
```

### GitHub webhook not working

1. Verify secrets contain correct GitHub App credentials
2. Check logs for authentication errors
3. Verify webhook URL in GitHub App settings
4. Check ingress/service configuration

## Security Best Practices

1. **Secrets Management**
   - Use external secret managers (AWS Secrets Manager, HashiCorp Vault)
   - Rotate secrets regularly
   - Never commit secrets to Git

2. **Network Policies**
   - Implement network policies to restrict traffic
   - Only allow necessary ingress/egress

3. **RBAC**
   - Use least-privilege service accounts
   - Restrict API access

4. **TLS/SSL**
   - Always use HTTPS for webhook endpoint
   - Configure cert-manager for automatic certificate management

## Cleanup

```bash
# Delete all resources
kubectl delete namespace remora

# Or using kustomize
kubectl delete -k .
```
