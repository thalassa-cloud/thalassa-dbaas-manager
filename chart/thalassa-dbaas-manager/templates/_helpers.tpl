{{/*
Expand the name of the chart.
*/}}
{{- define "dbaasManager.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "dbaasManager.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "dbaasManager.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}

{{- define "dbaasManager.baseLabels" -}}
helm.sh/chart: {{ include "dbaasManager.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "dbaasManager.labels" -}}
{{ include "dbaasManager.baseLabels" . }}
{{ include "dbaasManager.selectorLabels" . }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "dbaasManager.selectorLabels" -}}
app.kubernetes.io/name: {{ include "dbaasManager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: operator
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "dbaasManager.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "dbaasManager.name" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Labels for optional end-user ClusterRoles (thalassa:dbaas:reader, etc.) — not the operator workload.
*/}}
{{- define "dbaasManager.userClusterRoleLabels" -}}
{{ include "dbaasManager.baseLabels" . }}
app.kubernetes.io/name: {{ include "dbaasManager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: user-rbac
{{- end }}
