{{/*
Expand the namespace name.
*/}}
{{- define "aiops-platform.namespace" -}}
{{- .Values.namespace.name | default .Release.Namespace -}}
{{- end -}}

{{/*
Common labels.
*/}}
{{- define "aiops-platform.labels" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
app.kubernetes.io/part-of: aiops-platform
helm.sh/chart: {{ printf "%s-%s" .root.Chart.Name .root.Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
Selector labels for a given component name.
*/}}
{{- define "aiops-platform.selectorLabels" -}}
app.kubernetes.io/name: {{ .name }}
{{- end -}}
