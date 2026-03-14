# Certctl Quick Start Guide

Get a working certctl deployment from zero to managing certificates in 10 minutes.

## Prerequisites

- **Docker** and **Docker Compose** (recommended), or:
  - Go 1.22+
  - PostgreSQL 14+
  - psql CLI tool

## Option 1: Docker Compose (Fastest)

### 1. Clone & Setup

```bash
git clone https://github.com/shankar0123/certctl.git
cd certctl

# Copy environment template
cp .env.example .env

# Optional: edit .env for custom settings
# nano .env
```

### 2. Start the Stack

```bash
make docker-up

# Wait for services to be healthy (~30 seconds)
docker-compose -f deploy/docker-compose.yml ps
```

You should see:
```
NAME                 STATUS
certctl-postgres     Up (healthy)
certctl-server       Up (healthy)
certctl-agent        Up
```

### 3. Verify Health

```bash
# Server health check
curl http://localhost:8443/health
# Expected: {"status":"healthy"}

# Container logs
make docker-logs-server
```

---

## Option 2: Manual Build & Run

### 1. Clone & Dependencies

```bash
git clone https://github.com/shankar0123/certctl.git
cd certctl

go mod download
```

### 2. Setup PostgreSQL

```bash
# Create database and user
psql -U postgres -h localhost << EOF
CREATE USER certctl WITH PASSWORD 'certctl';
CREATE DATABASE certctl OWNER certctl;
GRANT ALL PRIVILEGES ON DATABASE certctl TO certctl;
EOF

# Verify connection
psql -h localhost -U certctl -d certctl -c "SELECT 1"
```

### 3. Run Migrations

```bash
# Install migrate tool
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Set database URL
export DB_URL="postgres://certctl:certctl@localhost:5432/certctl?sslmode=disable"

# Run migrations
make migrate-up
```

### 4. Start Server

```bash
# Terminal 1: Server
make run

# Expected output:
# 2024-03-14T10:30:00Z server starting version=1.0.0 server_port=8443
```

### 5. Start Agent (Optional)

```bash
# Terminal 2: Agent
export SERVER_URL=http://localhost:8443
export API_KEY=default-api-key
./bin/agent

# Expected output: Agent connecting to http://localhost:8443
```

---

## Walk-Through: Create Your First Certificate

### Step 1: Verify API Access

```bash
curl -X GET http://localhost:8443/health
```

Response:
```json
{"status":"healthy"}
```

### Step 2: Create a Team (Optional)

Teams organize ownership and auditing. For this quick start, we'll use a default team.

```bash
TEAM_ID="default"
```

### Step 3: Register an ACME Issuer

Create a certificate issuer configuration (Let's Encrypt staging for this demo):

```bash
curl -X POST http://localhost:8443/api/v1/issuers \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "default",
    "name": "lets-encrypt-staging",
    "type": "acme",
    "config": {
      "directory_url": "https://acme-staging-v02.api.letsencrypt.org/directory",
      "email": "admin@example.com"
    }
  }'
```

Response (save the `issuer_id`):
```json
{
  "id": "issuer-abc123",
  "name": "lets-encrypt-staging",
  "type": "acme",
  "created_at": "2024-03-14T10:30:00Z"
}
```

Store the issuer ID:
```bash
ISSUER_ID="issuer-abc123"
```

### Step 4: Register an Agent

Agents handle certificate requests and deployment. Register one:

```bash
curl -X POST http://localhost:8443/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "default",
    "name": "quickstart-agent",
    "description": "Local development agent"
  }'
```

Response (save the `api_key` and `id`):
```json
{
  "id": "agent-xyz789",
  "name": "quickstart-agent",
  "api_key": "ey...",
  "registered_at": "2024-03-14T10:30:00Z",
  "status": "registered"
}
```

Store the agent details:
```bash
AGENT_ID="agent-xyz789"
AGENT_API_KEY="ey..."
```

### Step 5: Create a Deployment Target

Targets are where certificates will be deployed (NGINX, F5, etc.). For this demo, we'll skip actual deployment:

```bash
curl -X POST http://localhost:8443/api/v1/targets \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "default",
    "agent_id": "'$AGENT_ID'",
    "name": "example-nginx",
    "type": "nginx",
    "config": {
      "host": "nginx.example.com",
      "ssh_user": "deploy",
      "ssh_key": "/path/to/key",
      "cert_path": "/etc/nginx/ssl"
    }
  }'
```

Response:
```json
{
  "id": "target-def456",
  "name": "example-nginx",
  "agent_id": "agent-xyz789",
  "type": "nginx",
  "status": "pending_validation"
}
```

Store the target ID:
```bash
TARGET_ID="target-def456"
```

### Step 6: Create a Managed Certificate

Now the main event—request a certificate to be issued and managed:

```bash
curl -X POST http://localhost:8443/api/v1/certificates \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "default",
    "domain": "api.example.com",
    "issuer_id": "'$ISSUER_ID'",
    "target_ids": ["'$TARGET_ID'"],
    "renewal_days_before": 30,
    "auto_deploy": true
  }'
```

Response:
```json
{
  "id": "cert-ghi012",
  "domain": "api.example.com",
  "issuer_id": "issuer-abc123",
  "status": "pending",
  "created_at": "2024-03-14T10:30:00Z",
  "expires_at": null,
  "renewal_at": null
}
```

Store the certificate ID:
```bash
CERT_ID="cert-ghi012"
```

### Step 7: Check Certificate Status

Poll the certificate status as issuance progresses:

```bash
curl -X GET http://localhost:8443/api/v1/certificates/$CERT_ID \
  -H "Content-Type: application/json"
```

Response (will change over time):
```json
{
  "id": "cert-ghi012",
  "domain": "api.example.com",
  "status": "issued",
  "expires_at": "2024-06-12T10:30:00Z",
  "issued_by": "issuer-abc123",
  "deployed_to": [
    {
      "target_id": "target-def456",
      "status": "success",
      "deployed_at": "2024-03-14T10:30:30Z"
    }
  ]
}
```

### Step 8: View Audit Trail

See all actions related to your certificate:

```bash
curl -X GET "http://localhost:8443/api/v1/audit/logs?resource_id=$CERT_ID" \
  -H "Content-Type: application/json"
```

Response:
```json
{
  "logs": [
    {
      "id": "audit-001",
      "timestamp": "2024-03-14T10:30:00Z",
      "actor": {
        "type": "api",
        "id": "client-001"
      },
      "action": "certificate_created",
      "resource": {
        "type": "certificate",
        "id": "cert-ghi012"
      },
      "status": "success"
    },
    {
      "id": "audit-002",
      "timestamp": "2024-03-14T10:30:10Z",
      "actor": {
        "type": "agent",
        "id": "agent-xyz789"
      },
      "action": "certificate_issued",
      "resource": {
        "type": "certificate",
        "id": "cert-ghi012"
      },
      "status": "success",
      "details": {
        "issuer": "lets-encrypt-staging",
        "expiry": "2024-06-12"
      }
    },
    {
      "id": "audit-003",
      "timestamp": "2024-03-14T10:30:25Z",
      "actor": {
        "type": "system",
        "id": "scheduler"
      },
      "action": "certificate_deployed",
      "resource": {
        "type": "certificate",
        "id": "cert-ghi012"
      },
      "status": "success",
      "details": {
        "deployed_to": "example-nginx"
      }
    }
  ]
}
```

### Step 9: Trigger Manual Renewal (Optional)

To manually trigger certificate renewal:

```bash
curl -X POST http://localhost:8443/api/v1/certificates/$CERT_ID/renew \
  -H "Content-Type: application/json"
```

The scheduler will automatically check for renewals every hour. Certificates within 30 days of expiry are renewed automatically.

---

## Development Mode

For development with hot reload and database browser:

```bash
# Install tools
make install-tools

# Start dev stack (includes PgAdmin at localhost:5050)
make docker-up-dev

# View logs
make docker-logs-server
make docker-logs-agent

# Admin credentials for PgAdmin:
# Email: admin@example.com (default, see .env)
# Password: admin (default, see .env)

# Access PgAdmin: http://localhost:5050
# Add server: postgres, port 5432, user certctl, password certctl
```

---

## Testing the Flow End-to-End

Here's a complete script to test the full flow:

```bash
#!/bin/bash
set -e

API="http://localhost:8443"
TEAM="default"

echo "1. Creating ACME issuer..."
ISSUER=$(curl -s -X POST $API/api/v1/issuers \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "'$TEAM'",
    "name": "letsencrypt-staging",
    "type": "acme",
    "config": {
      "directory_url": "https://acme-staging-v02.api.letsencrypt.org/directory",
      "email": "test@example.com"
    }
  }' | jq -r '.id')

echo "   Issuer: $ISSUER"

echo "2. Registering agent..."
AGENT=$(curl -s -X POST $API/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "'$TEAM'",
    "name": "test-agent"
  }' | jq -r '.id')

echo "   Agent: $AGENT"

echo "3. Creating certificate..."
CERT=$(curl -s -X POST $API/api/v1/certificates \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "'$TEAM'",
    "domain": "test-'$(date +%s)'.example.com",
    "issuer_id": "'$ISSUER'",
    "renewal_days_before": 30
  }' | jq -r '.id')

echo "   Certificate: $CERT"

echo "4. Checking status..."
curl -s -X GET $API/api/v1/certificates/$CERT | jq '.status'

echo "5. Viewing audit trail..."
curl -s -X GET "$API/api/v1/audit/logs?resource_id=$CERT" | jq '.logs | length'

echo "Done!"
```

Save as `test.sh`, make executable, and run:
```bash
chmod +x test.sh
./test.sh
```

---

## Common Issues

### Server Won't Start

```bash
# Check database connection
psql -h localhost -U certctl -d certctl -c "SELECT 1"

# View logs
make docker-logs-server

# Check environment
env | grep DB_
```

### Agent Can't Connect

```bash
# Verify server is running
curl http://localhost:8443/health

# Check agent logs
docker logs certctl-agent

# Verify API key is correct
echo $AGENT_API_KEY
```

### Certificate Stays "Pending"

```bash
# Check if agent is registered
curl http://localhost:8443/api/v1/agents

# Check agent logs for errors
make docker-logs-agent

# View certificate details
curl http://localhost:8443/api/v1/certificates/$CERT_ID

# Check audit trail
curl "http://localhost:8443/api/v1/audit/logs?resource_id=$CERT_ID"
```

---

## Next Steps

1. **Read** [docs/architecture.md](architecture.md) to understand the design
2. **Explore** the [API](../README.md#api-overview) for more operations
3. **Build** a [custom connector](connectors.md) for your infrastructure
4. **Deploy** to production using [docs/k8s-deployment.md](k8s-deployment.md) (coming soon)

---

For more help, see [README.md](../README.md#troubleshooting) or open an issue on GitHub.
