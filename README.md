# thalassa-dbaas-manager

Provision Thalassa Cloud DBaaS resources (PostgreSQL clusters, databases, roles) from your Kubernetes cluster. This manager is an extension for Thalassa Cloud Kubernetes clusters: it uses the cluster identity and a default subnet so you can declare PostgresCluster, PostgresDatabase, PostgresRole, and backup resources and have them reconciled with the Thalassa DBaaS API.

> **Beta release**
> This project is in intial beta release. This means that the API may change, as well as not ever resource is covered with E2E tests.

## Description

The thalassa-dbaas-manager runs inside a Thalassa Cloud Kubernetes cluster and manages only DBaaS resources. It requires:

- Cluster ID – Thalassa Cloud Kubernetes cluster identity (required).
- Default subnet ID – Thalassa subnet used for new DB clusters when `spec.subnetRef` is not set.

Subnet can still be overridden per cluster via `spec.subnetRef.identity`. Security groups are specified by Thalassa identity in `spec.securityGroupRefs[].identity`. No Kubernetes CRs for Subnet or SecurityGroup are used; identities are resolved from the Thalassa API.

## Getting Started

### Prerequisites

- Go 1.24+
- Docker 17.03+
- kubectl 1.11+
- A Kubernetes cluster (Thalassa Cloud or any cluster with Thalassa API access)

## Deploy with Helm and workload identity federation

For production-style installs from the published OCI chart, the recommended authentication mode is OIDC token exchange: the controller uses the in-cluster Kubernetes service account token as a *subject token*, exchanges it at Thalassa’s token endpoint for an API access token, and calls the IaaS API with that token. That flow is set up with workload identity federation so Thalassa trusts tokens from your cluster’s service account.

### Prerequisites

- Thalassa `tcloud` CLI (or your org’s equivalent) with permission to run workload-identity bootstrap.
- Your organisation ID and cluster identity in Thalassa (from the console or your platform team).
- Helm 3.x and access to the chart registry (`oci://ghcr.io/thalassa-cloud/charts/…`).

### 1. Bootstrap federated identity

Before installing the controller, register the Kubernetes service account that the chart will use (default name `thalassa-dbaas-manager` in namespace `thalassa-dbaas-manager`) with Thalassa IAM. The bootstrap command creates the federated binding and a Thalassa service account used for token exchange; you need its ID for Helm.

Example (adjust cluster ID, namespace, SA name, and role to match your environment):

```bash
export ORGANISATION_ID="<your-org-id>"
export CLUSTER_ID="<your-cluster-id>"

tcloud iam workload-identity-federation bootstrap kubernetes \
  --cluster "$CLUSTER_ID" \
  --namespace thalassa-dbaas-manager \
  --service-account thalassa-dbaas-manager \
  --role dbaas:FullAdminAccess
```

Copy Thalassa service account ID from the command output or UI (for example `sa-…`) into `THALASSA_SERVICE_ACCOUNT_ID` for the next step.

### 2. Helm values (token exchange)

Enable Thalassa, set your organisation, and configure `authMethod: tokenExchange` with the service account ID from bootstrap. The chart mounts a projected service account token and passes `--thalassa-subject-token-file` so the controller never relies on `THALASSA_*` environment variables for client configuration.

```yaml
thalassa:
  enabled: true
  url: "https://api.thalassa.cloud/"
  organisation: "<ORGANISATION_ID>"
  authMethod: tokenExchange
  tokenExchange:
    serviceAccountId: "<THALASSA_SERVICE_ACCOUNT_ID>"
    projectedToken:
      enabled: true
      audience: "https://api.thalassa.cloud/"

serviceAccount:
  create: true
```

You can merge this into a `values.yaml` file or set the same keys with `helm --set` (see below).

### 3. Install CRDs and controller

Install CRDs first, then the controller chart. Pin the chart version you intend to run (examples use a tag; replace with the current release).

```bash
export ORGANISATION_ID="<ORGANISATION_ID>"
export THALASSA_SERVICE_ACCOUNT_ID="<THALASSA_SERVICE_ACCOUNT_ID>"

helm upgrade --install thalassa-dbaas-manager-crds oci://ghcr.io/thalassa-cloud/charts/thalassa-dbaas-manager-crds:<version> \
  --namespace thalassa-dbaas-manager \
  --create-namespace

helm upgrade --install thalassa-dbaas-manager oci://ghcr.io/thalassa-cloud/charts/thalassa-dbaas-manager:<version> \
  --namespace thalassa-dbaas-manager \
  --create-namespace \
  --set thalassa.organisation="$ORGANISATION_ID" \
  --set thalassa.tokenExchange.serviceAccountId="$THALASSA_SERVICE_ACCOUNT_ID" \
  --set enableServiceMonitor=false
```

Other auth methods (personal access token, OAuth2 client credentials) and more chart options are documented in [`chart/thalassa-dbaas-manager/README.md`](chart/thalassa-dbaas-manager/README.md).

Step-by-step copy/paste (concrete namespace, chart versions, and optional `tcloud` profile notes) lives in [`deploy/README.md`](deploy/README.md). For Flux CD, see [`deploy/flux/README.md`](deploy/flux/README.md).

## License

Copyright 2026. Licensed under the Apache License, Version 2.0.
