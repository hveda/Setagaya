{{/*
Chart name, honouring nameOverride.
*/}}
{{- define "honryu.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully qualified app name: <release>-<chart>, or just <release> when the
release name already contains the chart name (the usual case here, since
the release is expected to be named "honryu").
*/}}
{{- define "honryu.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Common labels applied to every object this chart renders.
*/}}
{{- define "honryu.labels" -}}
app.kubernetes.io/name: {{ include "honryu.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{/*
Selector labels for a named component (api, calibrator, scheduler, mysql,
prometheus, grafana) -- stable across chart/app version bumps, since
selectors are immutable on an existing Deployment/StatefulSet.
*/}}
{{- define "honryu.selectorLabels" -}}
app.kubernetes.io/name: {{ include "honryu.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/*
The in-cluster ingest URL every sidecar and calibration engine pod pushes
to -- the api Service, resolved via cluster-local DNS. Overridable
(values.ingestUrl) for a values file that names the Service differently.
*/}}
{{- define "honryu.ingestUrl" -}}
{{- if .Values.ingestUrl -}}
{{- .Values.ingestUrl -}}
{{- else -}}
{{- printf "http://%s-api.%s.svc.cluster.local:8080/api/ingest" (include "honryu.fullname" .) .Release.Namespace -}}
{{- end -}}
{{- end -}}
