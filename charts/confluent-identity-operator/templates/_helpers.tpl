{{/*
Expand the name of the chart.
*/}}
{{- define "confluent-identity-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fullname: release-name + chart-name (truncated to 63 chars).
*/}}
{{- define "confluent-identity-operator.fullname" -}}
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
Service account name.
*/}}
{{- define "confluent-identity-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.name }}
{{- .Values.serviceAccount.name }}
{{- else }}
{{- include "confluent-identity-operator.fullname" . }}
{{- end }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "confluent-identity-operator.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "confluent-identity-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "confluent-identity-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "confluent-identity-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Name of the secret holding the Confluent Cloud credentials.
*/}}
{{- define "confluent-identity-operator.credentialsSecretName" -}}
{{- if .Values.confluent.credentials.existingSecret }}
{{- .Values.confluent.credentials.existingSecret }}
{{- else }}
{{- printf "%s-cc-credentials" (include "confluent-identity-operator.fullname" .) }}
{{- end }}
{{- end }}
