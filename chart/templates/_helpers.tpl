{{/*
Expand the name of the chart.
*/}}
{{- define "kube-binpacking-exporter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "kube-binpacking-exporter.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if or (contains $name .Release.Name) (contains .Release.Name $name) }}
{{- $name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kube-binpacking-exporter.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "kube-binpacking-exporter.labels" -}}
helm.sh/chart: {{ include "kube-binpacking-exporter.chart" . }}
{{ include "kube-binpacking-exporter.selectorLabels" . }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "kube-binpacking-exporter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kube-binpacking-exporter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use.
*/}}
{{/*
Whether leader election should be active.
Auto-enabled when replicaCount > 1 (to prevent duplicate metrics),
or explicitly via leaderElection.enabled.
*/}}
{{- define "kube-binpacking-exporter.leaderElectionEnabled" -}}
{{- if or .Values.leaderElection.enabled (gt (int .Values.replicaCount) 1) -}}
true
{{- end -}}
{{- end }}

{{/*
Convert filter.nodeSelector (matchLabels + matchExpressions) to a CLI label selector string.
Returns empty string if nodeSelector is empty or not set.
*/}}
{{- define "kube-binpacking-exporter.nodeSelectorString" -}}
{{- $parts := list -}}
{{- if .Values.filter -}}
{{- if .Values.filter.nodeSelector -}}
{{- range $key, $value := .Values.filter.nodeSelector.matchLabels -}}
  {{- $parts = append $parts (printf "%s=%s" $key $value) -}}
{{- end -}}
{{- range .Values.filter.nodeSelector.matchExpressions -}}
  {{- if eq .operator "In" -}}
    {{- $parts = append $parts (printf "%s in (%s)" .key (join "," .values)) -}}
  {{- else if eq .operator "NotIn" -}}
    {{- $parts = append $parts (printf "%s notin (%s)" .key (join "," .values)) -}}
  {{- else if eq .operator "Exists" -}}
    {{- $parts = append $parts .key -}}
  {{- else if eq .operator "DoesNotExist" -}}
    {{- $parts = append $parts (printf "!%s" .key) -}}
  {{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- join "," $parts -}}
{{- end }}

{{- define "kube-binpacking-exporter.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kube-binpacking-exporter.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
