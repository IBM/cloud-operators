{{- if .Values.serviceAccount.create }}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "ibmcloud-operator.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
  {{- with .Values.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
