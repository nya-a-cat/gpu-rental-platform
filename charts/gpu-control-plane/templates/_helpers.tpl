{{- define "gpu-control-plane.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "gpu-control-plane.fullname" -}}
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

{{- define "gpu-control-plane.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "gpu-control-plane.labels" -}}
helm.sh/chart: {{ include "gpu-control-plane.chart" . }}
app.kubernetes.io/name: {{ include "gpu-control-plane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: control-plane
{{- end }}

{{- define "gpu-control-plane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gpu-control-plane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: control-plane
{{- end }}
{{- define "gpu-control-plane.migrationLabels" -}}
helm.sh/chart: {{ include "gpu-control-plane.chart" . }}
app.kubernetes.io/name: {{ include "gpu-control-plane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: migration
{{- end }}


{{- define "gpu-control-plane.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "gpu-control-plane.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- required "serviceAccount.name is required when serviceAccount.create is false" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "gpu-control-plane.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{- define "gpu-control-plane.migrationJobName" -}}
{{- printf "%s-migrate" (include "gpu-control-plane.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
