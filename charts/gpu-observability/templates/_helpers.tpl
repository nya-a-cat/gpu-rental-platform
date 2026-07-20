{{- define "gpu-observability.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "gpu-observability.fullname" -}}
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

{{- define "gpu-observability.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "gpu-observability.componentLabels" -}}
helm.sh/chart: {{ include "gpu-observability.chart" .root }}
app.kubernetes.io/name: {{ include "gpu-observability.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/version: {{ .root.Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "gpu-observability.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gpu-observability.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "gpu-observability.image" -}}
{{- if .digest }}
{{- printf "%s@%s" .repository .digest }}
{{- else }}
{{- required "image.tag is required when image.digest is empty" .tag | printf "%s:%s" .repository }}
{{- end }}
{{- end }}
