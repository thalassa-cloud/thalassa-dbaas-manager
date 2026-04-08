# Deploying thalassa-dbaas-manager (Helm, workload identity federation)

This guide walks through installing the published Helm chart with OIDC token exchange backed by Kubernetes workload identity federation in Thalassa. The controller reads Thalassa settings from process flags only (set by the chart); secrets and tokens come from mounted volumes, not `THALASSA_*` environment variables.

## Values snippet (`authMethod: tokenExchange`)

Fill `ORGANISATION_ID` and `THALASSA_SERVICE_ACCOUNT_ID` from the Thalassa console or from the bootstrap output below.

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
      audience: "https://api.thalassa.cloud"

serviceAccount:
  create: true
```

## 1. Bootstrap federated identity

Run before `helm install`. This links your cluster’s Kubernetes service account to Thalassa IAM and returns a Thalassa service account ID required for token exchange.

Set organisation and cluster identifiers for your environment (examples below are illustrative):

```bash
export ORGANISATION_ID="o-<org-id>"
export CLUSTER_ID="k8s-<cluster-id>"

```

Make sure your tcloud is configured to use the same organisation:

```bash
tcloud context organisation "$ORGANISATION_ID"
```

Bootstrap workload identity federation for the namespace and Kubernetes service account the chart will create or use:

```bash
tcloud iam workload-identity-federation bootstrap kubernetes \
  --cluster "$CLUSTER_ID" \
  --namespace thalassa-dbaas-manager \
  --service-account thalassa-dbaas-manager \
  --role dbaas:FullAdminAccess
```

Note the Thalassa service account ID from the command output or UI, for example:

```bash
export THALASSA_SERVICE_ACCOUNT_ID="sa-<serviceaccount-id-returned>"
```

That value must match `thalassa.tokenExchange.serviceAccountId` in Helm values.

## 2. Install CRDs

```bash
helm upgrade --install thalassa-dbaas-manager-crds oci://ghcr.io/thalassa-cloud/charts/thalassa-dbaas-manager-crds:v0.2.2 \
  --namespace thalassa-dbaas-manager \
  --create-namespace
```

## 3. Install the controller

```bash
helm upgrade --install thalassa-dbaas-manager oci://ghcr.io/thalassa-cloud/charts/thalassa-dbaas-manager:v0.2.2 \
  --namespace thalassa-dbaas-manager \
  --create-namespace \
  --set thalassa.organisation="$ORGANISATION_ID" \
  --set thalassa.tokenExchange.serviceAccountId="$THALASSA_SERVICE_ACCOUNT_ID" \
  --set enableServiceMonitor=false
```

Alternatively, pass a `values.yaml` that includes the full `thalassa` block from the top of this file (with `authMethod`, `projectedToken`, and so on) instead of only `--set` for organisation and service account ID.

## Flux CD

Example HelmRepository and HelmRelease manifests (OCI charts, CRDs then controller, `dependsOn` ordering) are in [`flux/`](flux/README.md). Apply with `kubectl apply -k deploy/flux/` or wire the YAML into your Flux `GitRepository` / `Kustomization`.
