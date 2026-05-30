{{/*
Expand the name of the chart.
*/}}
{{- define "govatars.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "govatars.fullname" -}}
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
{{- define "govatars.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "govatars.selectorLabels" -}}
app.kubernetes.io/name: {{ include "govatars.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Postgres DSN (аналог GOVATARS_POSTGRES_DSN из docker-compose).
*/}}
{{- define "govatars.postgresDSN" -}}
postgres://{{ .Values.postgres.user }}:{{ .Values.postgres.password }}@{{ .Values.postgres.host }}:{{ .Values.postgres.port }}/{{ .Values.postgres.database }}?sslmode=disable
{{- end }}

{{/*
RabbitMQ URL (аналог GOVATARS_RABBITMQ_URL из docker-compose).
*/}}
{{- define "govatars.rabbitmqURL" -}}
amqp://{{ .Values.rabbitmq.user }}:{{ .Values.rabbitmq.password }}@{{ .Values.rabbitmq.host }}:{{ .Values.rabbitmq.port }}/
{{- end }}

{{/*
Общие переменные окружения server и worker.
Переопределяют значения из config.yaml так же, как environment в docker-compose.
*/}}
{{- define "govatars.commonEnv" -}}
- name: GOVATARS_POSTGRES_DSN
  value: {{ include "govatars.postgresDSN" . | quote }}
- name: GOVATARS_RABBITMQ_URL
  value: {{ include "govatars.rabbitmqURL" . | quote }}
- name: GOVATARS_S3_ENDPOINT
  value: {{ printf "%s:%v" .Values.minio.host .Values.minio.port | quote }}
- name: GOVATARS_S3_ACCESS_KEY
  value: {{ .Values.minio.rootUser | quote }}
- name: GOVATARS_S3_SECRET_KEY
  value: {{ .Values.minio.rootPassword | quote }}
- name: OTEL_RESOURCE_ATTRIBUTES
  value: "deployment.environment=development"
{{- end }}
