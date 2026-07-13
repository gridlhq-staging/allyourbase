# High Availability
<!-- audited 2026-07-13 -->

AYB high-availability deployments use multiple identical AYB nodes behind a
load balancer, with durable state owned by shared infrastructure. The runnable
example in this repository is the local compose cell at
`deploy/compose/docker-compose.yml`; use `deploy/compose/README.md` for the
exact start, health, upstream-check, and teardown commands.

## Tested compose cell

The compose cell starts:

- `postgres:16` for the shared AYB database
- MinIO for S3-compatible object storage
- two identical AYB containers built from the repository `Dockerfile`
- nginx as the HTTP load balancer

Both AYB containers run the same image and differ only in their
`AYB_DATABASE_URL` application name. They share the same Postgres database,
MinIO bucket, auth configuration, and storage configuration. nginx is the only
published entrypoint, so health traffic reaches AYB through the same load
balancer path clients would use.

Run the proof with:

```bash
make test-cell
```

The cell test exercises `/health` through nginx, proves requests reach both AYB
upstream containers, and verifies cross-node traffic against the shared database
and object storage services.

## What this proves

The one-host compose proof verifies the AYB product shape needed for an HA cell:

- multiple AYB nodes can run the same container image with config-only
  differences
- a load balancer can route `/health` and API traffic to those nodes
- both nodes use the same Postgres database as their system of record
- storage API traffic can use shared S3-compatible object storage
- `make test-cell` can validate the topology without manual QA

This is the source-of-truth topology for local HA validation. Do not copy the
compose contents into a separate recipe; change `deploy/compose/docker-compose.yml`
and `deploy/compose/README.md` when the runnable cell changes.

## What this does not prove

The compose cell intentionally runs on one host. It does not certify:

- physically separate AYB hosts
- external Postgres failover behavior
- replicated or highly available object storage
- cloud load-balancer health-policy behavior
- supervisor or orchestrator restart policy

Those guarantees belong to deployment infrastructure around AYB. Keep Postgres
failover, object-store replication, TLS termination, host placement, and process
supervision in the platform that operates the cell. AYB nodes should remain
identical, stateless application containers pointed at the same durable services.

## Related guides

- [Deployment](./deployment.md) covers the container image, required runtime
  variables, external Postgres, and `/health` response contract.
- [Read Replicas](./replicas.md) covers AYB read-replica routing and manual
  database failover operations. It does not replace an external HA Postgres
  manager for production failover.
