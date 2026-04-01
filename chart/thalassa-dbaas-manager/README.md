# thalassa-dbaas-manager

![Version: 1.0.0](https://img.shields.io/badge/Version-1.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.0](https://img.shields.io/badge/AppVersion-1.0.0-informational?style=flat-square)

Helm chart for deploying the Thalassa Cloud DBaaS Manager.

## Source Code

* <https://github.com/thalassa-cloud/thalassa-dbaas-manager>

## Thalassa authentication

Set `thalassa.enabled: true` and configure **one** of the following (`authMethod`):

| `authMethod` | Description |
|--------------|-------------|
| `tokenExchange` | Federated workload identity: exchanges the pod’s Kubernetes service account JWT for a Thalassa API token via `POST` to `{url}/oidc/token`. Requires `thalassa.tokenExchange.serviceAccountId` (Thalassa service account ID). By default the chart mounts a **projected** service account token (`tokenExchange.projectedToken`) and passes `--thalassa-subject-token-file` to that path; set `projectedToken.enabled: false` and optionally `subjectTokenFile` to use the legacy automounted token path instead. |
| `pat` | Personal access token: the chart mounts your Kubernetes `Secret` (`thalassa.pat.existingSecret`) at `thalassa.pat.mountPath` and passes `--thalassa-token-file` to the mounted key file (see `secretKey` / `secretFilename`). |
| `oidcClientCredentials` | OAuth2 client credentials: `thalassa.oidcClientCredentials.clientId` and client secret mounted from `existingSecret` at `oidcClientCredentials.mountPath` (`--thalassa-client-secret-file`). |


If `thalassa.enabled` is `false`, supply the same flags via `extraArgs` (and any secrets via additional volumes), or use a wrapper that sets flags.

### Examples

**OIDC token exchange (in-cluster)**

```yaml
thalassa:
  enabled: true
  url: "https://api.thalassa.cloud/"
  organisation: "<org-id-or-slug>"
  authMethod: tokenExchange
  tokenExchange:
    serviceAccountId: "<thalassa-service-account-id>"
    # Default: projected SA token volume + --thalassa-subject-token-file. Optional:
    # projectedToken:
    #   audience: "https://api.thalassa.cloud/"  # set if your federated identity requires it
    # subjectTokenFile: "/var/run/secrets/..."   # overrides projected volume
```

When using token exchange, you need a federated identity. Bootstrap this for example through `tcloud`:

```bash
tcloud iam workload-identity-federation bootstrap kubernetes \
  --cluster <cluster-identity> \
  --namespace thalassa-dbaas-manager \
  --service-account thalassa-dbaas-manager \
  --role dbaas:FullAdminAccess
```

The above bootstraps a federated identity for a newly created service account with the thalassa IAM rolebinding `dbaas:FullAdminAccess`, for the Kubernetes serviceaccount `thalassa-dbaas-manager/thalassa-dbaas-manager`. Make sure that the namespace and serviceaccount match with the DBaaS Manager deployment's.

**Personal access token**

```yaml
thalassa:
  enabled: true
  organisation: "<org-id-or-slug>"
  authMethod: pat
  pat:
    existingSecret: thalassa-api-token
    secretKey: token
```

Create the secret first: `kubectl create secret generic thalassa-api-token --from-literal=token=...`

**OIDC client credentials**

```yaml
thalassa:
  enabled: true
  organisation: "<org-id-or-slug>"
  authMethod: oidcClientCredentials
  oidcClientCredentials:
    clientId: "<client-id>"
    existingSecret: thalassa-oauth
    clientSecretKey: client-secret
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| enableServiceMonitor | bool | `false` |  |
| envFrom | list | `[]` |  |
| extraArgs | list | `[]` |  |
| fullnameOverride | string | `""` |  |
| hpa.cpu.averageUtilization | int | `90` |  |
| hpa.cpu.targetType | string | `"Utilization"` |  |
| hpa.enabled | bool | `false` |  |
| hpa.maxReplicas | int | `4` |  |
| hpa.minReplicas | int | `2` |  |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.repository | string | `"ghcr.io/thalassa-cloud/thalassa-dbaas-manager"` |  |
| image.tag | string | `nil` |  |
| imagePullSecrets | object | `{}` |  |
| leaderElection.enabled | bool | `false` |  |
| livenessProbe.failureThreshold | int | `3` |  |
| livenessProbe.httpGet.path | string | `"/healthz"` |  |
| livenessProbe.httpGet.port | string | `"health"` |  |
| livenessProbe.initialDelaySeconds | int | `10` |  |
| livenessProbe.periodSeconds | int | `10` |  |
| livenessProbe.timeoutSeconds | int | `10` |  |
| minAvailable | int | `1` |  |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` |  |
| podAnnotations | object | `{}` |  |
| podSecurityContext.fsGroup | int | `65532` |  |
| podSecurityContext.runAsGroup | int | `65532` |  |
| podSecurityContext.runAsNonRoot | bool | `true` |  |
| podSecurityContext.runAsUser | int | `65532` |  |
| podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| rbac.userRoles.enabled | bool | `true` |  |
| readinessProbe.failureThreshold | int | `3` |  |
| readinessProbe.httpGet.path | string | `"/readyz"` |  |
| readinessProbe.httpGet.port | string | `"health"` |  |
| readinessProbe.initialDelaySeconds | int | `10` |  |
| readinessProbe.periodSeconds | int | `10` |  |
| readinessProbe.timeoutSeconds | int | `10` |  |
| replicaCount | int | `1` |  |
| resources | object | `{}` |  |
| securityContext.allowPrivilegeEscalation | bool | `false` |  |
| securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| securityContext.readOnlyRootFilesystem | bool | `true` |  |
| securityContext.runAsUser | int | `65532` |  |
| service.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` |  |
| thalassa.authMethod | string | `"tokenExchange"` |  |
| thalassa.enabled | bool | `true` |  |
| thalassa.insecure | bool | `false` |  |
| thalassa.oidcClientCredentials.clientId | string | `""` |  |
| thalassa.oidcClientCredentials.clientSecretFilename | string | `"client-secret"` |  |
| thalassa.oidcClientCredentials.clientSecretKey | string | `"client-secret"` |  |
| thalassa.oidcClientCredentials.existingSecret | string | `""` |  |
| thalassa.oidcClientCredentials.mountPath | string | `"/var/run/secrets/thalassa-oidc"` |  |
| thalassa.organisation | string | `""` |  |
| thalassa.pat.existingSecret | string | `""` |  |
| thalassa.pat.mountPath | string | `"/var/run/secrets/thalassa-pat"` |  |
| thalassa.pat.secretFilename | string | `"token"` |  |
| thalassa.pat.secretKey | string | `"token"` |  |
| thalassa.project | string | `""` |  |
| thalassa.region | string | `""` |  |
| thalassa.tokenExchange.accessTokenLifetime | string | `""` |  |
| thalassa.tokenExchange.oidcTokenUrl | string | `""` |  |
| thalassa.tokenExchange.projectedToken.audience | string | `"https://api.thalassa.cloud/"` |  |
| thalassa.tokenExchange.projectedToken.enabled | bool | `true` |  |
| thalassa.tokenExchange.projectedToken.expirationSeconds | int | `3600` |  |
| thalassa.tokenExchange.projectedToken.mountPath | string | `"/var/run/secrets/thalassa-projected-token"` |  |
| thalassa.tokenExchange.projectedToken.path | string | `"token"` |  |
| thalassa.tokenExchange.serviceAccountId | string | `""` |  |
| thalassa.tokenExchange.subjectToken | string | `""` |  |
| thalassa.tokenExchange.subjectTokenFile | string | `""` |  |
| thalassa.url | string | `"https://api.thalassa.cloud/"` |  |
| tolerations | list | `[]` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
