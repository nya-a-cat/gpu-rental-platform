{{- define "gpu-platform-addon.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "gpu-platform-addon.fullname" -}}
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

{{- define "gpu-platform-addon.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "gpu-platform-addon.labels" -}}
helm.sh/chart: {{ include "gpu-platform-addon.chart" . }}
app.kubernetes.io/name: {{ include "gpu-platform-addon.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: hub-manager
{{- end }}

{{- define "gpu-platform-addon.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gpu-platform-addon.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: hub-manager
{{- end }}

{{- define "gpu-platform-addon.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "gpu-platform-addon.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- required "serviceAccount.name is required when serviceAccount.create is false" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "gpu-platform-addon.image" -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag }}
{{- end }}
