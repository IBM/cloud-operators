apiVersion: v1
kind: Service
metadata:
  name: {{ include "ibmcloud-operator.metricsName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
  {{- with .Values.metrics.service.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  ports:
    - name: https
      port: 8443
      protocol: TCP
      targetPort: https
  selector:
    {{- include "ibmcloud-operator.selectorLabels" . | nindent 4 }}
{{- if .Values.metrics.serviceMonitor.enabled }}
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "ibmcloud-operator.metricsName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
    {{- with .Values.metrics.serviceMonitor.additionalLabels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
  endpoints:
    - path: /metrics
      port: https
  selector:
    matchLabels:
      {{- include "ibmcloud-operator.selectorLabels" . | nindent 6 }}
{{- end }}
