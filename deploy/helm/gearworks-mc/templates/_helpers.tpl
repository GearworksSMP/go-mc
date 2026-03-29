{{/*
Expand the name of the chart.
*/}}
{{- define "gearworks-mc.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "gearworks-mc.fullname" -}}
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
{{- define "gearworks-mc.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "gearworks-mc.labels" -}}
helm.sh/chart: {{ include "gearworks-mc.chart" . }}
{{ include "gearworks-mc.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "gearworks-mc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gearworks-mc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
ServiceAccount name.
*/}}
{{- define "gearworks-mc.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "gearworks-mc.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Database URL.
*/}}
{{- define "gearworks-mc.databaseURL" -}}
postgres://{{ .Values.postgresql.username }}:{{ .Values.postgresql.password }}@{{ .Values.postgresql.host }}:{{ .Values.postgresql.port }}/{{ .Values.postgresql.database }}?sslmode=disable
{{- end }}

{{/*
Redis URL.
*/}}
{{- define "gearworks-mc.redisURL" -}}
redis://{{ .Values.cluster.redis.host }}:{{ .Values.cluster.redis.port }}
{{- end }}
