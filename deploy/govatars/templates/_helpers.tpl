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
initContainer that waits for dependencies (postgres / rabbitmq / minio).
*/}}
{{- define "govatars.waitForDeps" -}}
- name: wait-for-deps
  image: {{ .Values.waitForDependencies.image }}
  command:
    - sh
    - -c
    - |
      for dep in {{ printf "%s:%v" .Values.postgres.host .Values.postgres.port }} {{ printf "%s:%v" .Values.rabbitmq.host .Values.rabbitmq.port }} {{ printf "%s:%v" .Values.minio.host .Values.minio.port }}; do
        host="${dep%%:*}"; port="${dep##*:}"
        echo "Waiting for ${host}:${port}..."
        until nc -w2 "$host" "$port" </dev/null 2>/dev/null; do
          echo "${host}:${port} is unavailable, retry in 2s"
          sleep 2
        done
        echo "${host}:${port} is available"
      done
{{- end }}

{{/*
App secrets from K8s Secret (deploy/secrets chart).
*/}}
{{- define "govatars.postgresDSNEnv" -}}
- name: GOVATARS_POSTGRES_DSN
  valueFrom:
    secretKeyRef:
      name: {{ .Values.secrets.app }}
      key: {{ .Values.secrets.keys.postgresDSN }}
{{- end }}

{{/*
Common envs for server and worker.
*/}}
{{- define "govatars.commonEnv" -}}
{{- include "govatars.postgresDSNEnv" . }}
- name: GOVATARS_RABBITMQ_URL
  valueFrom:
    secretKeyRef:
      name: {{ .Values.secrets.app }}
      key: {{ .Values.secrets.keys.rabbitmqURL }}
- name: GOVATARS_S3_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .Values.secrets.app }}
      key: {{ .Values.secrets.keys.s3AccessKey }}
- name: GOVATARS_S3_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .Values.secrets.app }}
      key: {{ .Values.secrets.keys.s3SecretKey }}
- name: GOVATARS_S3_ENDPOINT
  value: {{ printf "%s:%v" .Values.minio.host .Values.minio.port | quote }}
- name: OTEL_RESOURCE_ATTRIBUTES
  value: "deployment.environment=development"
{{- end }}
