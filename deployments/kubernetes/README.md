# Kubernetes Deployment

This directory contains Kubernetes manifests for deploying Remora.

## Prerequisites

- Kubernetes cluster (1.25+)
- kubectl configured
- PostgreSQL database (external or in-cluster)
- GitHub App created and configured

## Deployment Steps

### 1. Create Namespace

```bash
kubectl apply -f namespace.yaml
```

### 2. Create Secrets

**Option A: From files**

```bash
kubectl create secret generic remora-secrets \
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
- Database host and configuration
- Logging level
- Feature flags

```bash
kubectl apply -f configmap.yaml
```

### 4. Deploy Application

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

### External PostgreSQL

Update `configmap.yaml`:

```yaml
DATABASE_HOST: "your-postgres-host.example.com"
DATABASE_PORT: "5432"
DATABASE_SSLMODE: "require"
```

### Cloud-Managed Database (AWS RDS, GCP Cloud SQL, etc.)

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
