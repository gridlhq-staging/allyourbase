# Deployment
<!-- audited 2026-03-20 -->

AYB runs as a single `ayb` binary. In containers, the Dockerfile builds an image with:

- `ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]`
- `CMD ["start", "--foreground"]`

The image also sets `AYB_SERVER_HOST=0.0.0.0`, so `-p 8090:8090` works without adding a custom bind host override.

Public multi-arch images are published at `ghcr.io/allyourbasehq/allyourbase`. The Docker examples below use that package by default and retain a source-build fallback for validating local changes.

## Docker

For the tested two-node compose cell with nginx load balancing, see
[High Availability](./ha.md). This page keeps the single-container and general
runtime configuration details.

### Pull the published image

For local evaluation, you can follow the moving `latest` tag:

```bash
export AYB_IMAGE="ghcr.io/allyourbasehq/allyourbase:latest"
docker pull "$AYB_IMAGE"
```

### Build the image locally (source-build fallback)

```bash
git clone https://github.com/AllyourbaseHQ/allyourbase.git
cd allyourbase
DOCKER_BUILDKIT=1 docker build -t ayb-local .
export AYB_IMAGE="ayb-local"
```

Choose either image path above. Both set `AYB_IMAGE`, which the remaining
single-container examples use.

### Quick start (managed PostgreSQL)

```bash
docker run --rm -p 8090:8090 \
  -e AYB_ADMIN_PASSWORD="change-me-to-a-strong-random-password" \
  "$AYB_IMAGE"
```

This starts `ayb start` in managed PostgreSQL mode (`AYB_DATABASE_URL` unset).

By default, the image keeps public auth and storage disabled. That quick-start
shape is fine for local admin-only exploration, but internet-facing validation
should enable auth explicitly and mount data directories for persistence.

### Auth-enabled + persistent local storage

```bash
mkdir -p ./ayb-pgdata ./ayb-storage

docker run --rm -p 8090:8090 \
  -e AYB_ADMIN_PASSWORD="change-me-to-a-strong-random-password" \
  -e AYB_AUTH_ENABLED=true \
  -e AYB_AUTH_JWT_SECRET="$(openssl rand -hex 32)" \
  -e AYB_STORAGE_ENABLED=true \
  -e AYB_DATABASE_EMBEDDED_DATA_DIR=/ayb_pgdata \
  -e AYB_STORAGE_LOCAL_PATH=/ayb_storage \
  -v "$PWD/ayb-pgdata:/ayb_pgdata" \
  -v "$PWD/ayb-storage:/ayb_storage" \
  "$AYB_IMAGE"
```

This is the recommended shape for Docker smoke validation because it exercises:

- managed PostgreSQL on a bind-mounted data directory
- auth-enabled public API routes
- storage persistence across container restarts

### With external PostgreSQL

```bash
docker run --rm -p 8090:8090 \
  -e AYB_DATABASE_URL="postgresql://user:pass@host:5432/mydb" \
  -e AYB_ADMIN_PASSWORD="change-me-to-a-strong-random-password" \
  "$AYB_IMAGE"
```

### Dynamic port platforms

For platforms that inject a runtime port, set `AYB_SERVER_PORT` and expose/map the same container port.

```bash
docker run --rm -p 8080:8080 \
  -e AYB_SERVER_PORT=8080 \
  -e AYB_ADMIN_PASSWORD="change-me" \
  "$AYB_IMAGE"
```

### Docker Compose

```yaml
services:
  ayb:
    image: ${AYB_IMAGE:?Set AYB_IMAGE to an explicit image tag or digest}
    ports:
      - "8090:8090"
    environment:
      AYB_AUTH_ENABLED: "true"
      AYB_STORAGE_ENABLED: "true"
      AYB_DATABASE_URL: "${AYB_DATABASE_URL}"
      AYB_ADMIN_PASSWORD: "${AYB_ADMIN_PASSWORD}"
      AYB_STORAGE_LOCAL_PATH: "/ayb_storage"
    depends_on:
      postgres:
        condition: service_healthy
    volumes:
      - ayb_storage:/ayb_storage
      # Lets the entrypoint generate one private JWT signing key and keep it
      # across container recreation. Set AYB_AUTH_JWT_SECRET only if you want
      # to supply your own secret instead.
      - ayb_jwt_secret:/home/ayb/.ayb/secrets

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ayb
      POSTGRES_PASSWORD: ayb
      POSTGRES_DB: ayb
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ayb"]
      interval: 5s
      timeout: 3s
      retries: 5

volumes:
  pgdata:
  ayb_storage:
  ayb_jwt_secret:
```

Populate `AYB_DATABASE_URL` and `AYB_ADMIN_PASSWORD` from an uncommitted `.env`
file or your platform secret manager instead of hardcoding them in
`compose.yaml`. For auth, either:

- mount `ayb_jwt_secret` and let the image entrypoint generate and persist a private signing key
- set `AYB_AUTH_JWT_SECRET` from your secret manager if you prefer to manage the signing key yourself

## Bare metal / VPS

### Install

```bash
curl -fsSLo /tmp/ayb-install.sh https://install.allyourbase.io/install.sh
sh /tmp/ayb-install.sh
```

The installer places the binary at `~/.ayb/bin/ayb` by default.

### systemd service

Create `/etc/systemd/system/ayb.service`:

```ini
[Unit]
Description=Allyourbase
After=network.target

[Service]
Type=simple
User=ayb
Group=ayb
WorkingDirectory=/home/ayb
ExecStart=/home/ayb/.ayb/bin/ayb start
Restart=always
RestartSec=5
EnvironmentFile=/etc/ayb/ayb.env

[Install]
WantedBy=multi-user.target
```

Create `/etc/ayb/ayb.env` with the runtime secrets and restrict it to root:

```bash
sudo install -d -m 0750 /etc/ayb
sudo sh -c 'cat > /etc/ayb/ayb.env <<\"EOF\"
AYB_DATABASE_URL=postgresql://ayb:password@localhost:5432/ayb
AYB_ADMIN_PASSWORD=replace-with-a-secure-random-password
EOF'
sudo chmod 600 /etc/ayb/ayb.env
```

Then enable and start:

```bash
sudo systemctl enable ayb
sudo systemctl start ayb
```

## Required and recommended runtime variables

- Required for external PostgreSQL: `AYB_DATABASE_URL`
- Strongly recommended: `AYB_ADMIN_PASSWORD`
- Required for public auth flows: `AYB_AUTH_ENABLED=true` plus either `AYB_AUTH_JWT_SECRET` or, in the container image, a mounted JWT secret volume for the entrypoint-managed key
- Required for storage API flows: `AYB_STORAGE_ENABLED=true`
- Often required on managed platforms: `AYB_SERVER_PORT`

## Production hardening checklist

Before exposing AYB beyond a local development host, close these checks against the same deployment artifact you plan to run:

- Pin `AYB_IMAGE` to an explicit release tag or immutable digest. Do not use the moving `latest` tag for production deployments.
- Enable [Authentication](/guide/authentication) for public auth flows and apply Row-level security (RLS) policies from the [Security](/guide/security) guide to tables that can be reached by user traffic.
- Review [Configuration](/guide/configuration) before changing bind settings. `internal/config/config_defaults_sections.go` defaults the server host to loopback; `AYB_SERVER_HOST=0.0.0.0` or any other non-loopback exposure needs auth plus TLS, firewall rules, and reverse proxy controls.
- Persist secrets outside the checkout with your platform secret manager or the patterns in [Secrets](/guide/secrets). Do not bake admin passwords, JWT signing secrets, database URLs, or provider keys into images or committed files.
- Run a backup and restore rehearsal using the [Backups](/guide/backups) guide before you accept production writes. A backup that has not been restored into a clean environment is only an unproven artifact.
- Establish a launch load profile from your expected traffic shape, then run the synced safe smoke and soak commands in `tests/load/README.md`. The internal `_dev/performance_baseline.md` measurements are development-local observations, so use them only as a prompt to measure your own deployment rather than as production capacity claims.

## PostGIS

AYB can run with PostGIS in either mode:

- External PostgreSQL: use a PostGIS-enabled server/image and run `CREATE EXTENSION postgis;`
- Managed PostgreSQL: enable PostGIS in config via `[managed_pg] postgis = true` (or include `"postgis"` in `managed_pg.extensions`)

Managed PostgreSQL extension availability depends on the PostgreSQL build behind that runtime. If you need an extension outside the managed build's default set, use an external PostgreSQL service with that extension already installed.

## Health check

```bash
curl http://127.0.0.1:8090/health
```

`/health` behavior:

- `200` with `{"status":"ok","database":"ok"}` when DB is reachable
- `200` with `{"status":"ok","database":"not configured"}` when no DB pool is configured
- `503` with `{"status":"degraded","database":"unreachable"}` when DB checks fail

Use this endpoint for container and load-balancer probes.
