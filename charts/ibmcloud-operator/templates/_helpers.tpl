{{- define "ibmcloud-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "ibmcloud-operator.fullname" -}}
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

{{- define "ibmcloud-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "ibmcloud-operator.labels" -}}
helm.sh/chart: {{ include "ibmcloud-operator.chart" . }}
{{ include "ibmcloud-operator.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "ibmcloud-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ibmcloud-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end }}

{{- define "ibmcloud-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "ibmcloud-operator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "ibmcloud-operator.managerRoleName" -}}
{{- printf "%s-manager" ((include "ibmcloud-operator.fullname" .) | trunc 55 | trimSuffix "-") }}
{{- end }}

{{- define "ibmcloud-operator.proxyRoleName" -}}
{{- printf "%s-proxy" ((include "ibmcloud-operator.fullname" .) | trunc 57 | trimSuffix "-") }}
{{- end }}

{{- define "ibmcloud-operator.leaderElectionRoleName" -}}
{{- printf "%s-leader-election" ((include "ibmcloud-operator.fullname" .) | trunc 47 | trimSuffix "-") }}
{{- end }}

{{- define "ibmcloud-operator.metricsReaderRoleName" -}}
{{- printf "%s-metrics-reader" ((include "ibmcloud-operator.fullname" .) | trunc 48 | trimSuffix "-") }}
{{- end }}

{{- define "ibmcloud-operator.metricsName" -}}
{{- printf "%s-metrics" ((include "ibmcloud-operator.fullname" .) | trunc 55 | trimSuffix "-") }}
{{- end }}
