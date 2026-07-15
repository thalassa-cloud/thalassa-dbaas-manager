# Flux CD (Helm) example

Example manifests to install **thalassa-dbaas-manager** and **thalassa-dbaas-manager-crds** from the published OCI charts using [Flux](https://fluxcd.io/) `HelmRepository` + `HelmRelease`.

## Layout

| File | Purpose |
|------|---------|
| [`helmrepository.yaml`](helmrepository.yaml) | OCI source `oci://ghcr.io/thalassa-cloud/charts` |
| [`helmrelease-controller-crds.yaml`](helmrelease-controller-crds.yaml) | CRDs chart (install first) |
| [`helmrelease-controller.yaml`](helmrelease-controller.yaml) | Controller; `dependsOn` the CRDs release |

Both releases use `targetNamespace: thalassa-dbaas-manager` and `install.createNamespace: true`.

## Prerequisites

- Flux [installed](https://fluxcd.io/flux/installation/) on the cluster (at least source-controller and helm-controller).
- Workload identity federation bootstrapped for the controller service account; see [deploy/README.md](../README.md).

## Apply

Point `KUBECONFIG` at the cluster, then:

```bash
kubectl apply -f deploy/flux/
```

Or reference this directory from a Flux `Kustomization` / monorepo path.

## Values and secrets

The example uses **fake** IDs under `spec.values`. Replace `organisation` and `tokenExchange.serviceAccountId` with real values from Thalassa Cloud after bootstrap.

## API versions

If your Flux install uses newer APIs, you can bump:

- `HelmRepository`: `source.toolkit.fluxcd.io/v1` when available
- `HelmRelease`: `helm.toolkit.fluxcd.io/v2` when available

Check `kubectl api-resources | grep flux` and the [Flux Helm docs](https://fluxcd.io/flux/components/helm/helmreleases/) for your cluster version.
