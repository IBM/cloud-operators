apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "ibmcloud-operator.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "ibmcloud-operator.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "ibmcloud-operator.selectorLabels" . | nindent 8 }}
    spec:
      serviceAccountName: {{ include "ibmcloud-operator.serviceAccountName" . }}
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      containers:
        - name: kube-rbac-proxy
          image: "{{ .Values.metrics.proxy.image.repository }}:{{ .Values.metrics.proxy.image.tag }}"
          imagePullPolicy: {{ .Values.metrics.proxy.image.pullPolicy }}
          args:
            - --secure-listen-address=0.0.0.0:8443
            - --upstream=http://127.0.0.1:8080/
            - --logtostderr=true
            - --v=10
          ports:
            - name: https
              containerPort: 8443
              protocol: TCP
        - name: manager
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          command:
            - /manager
          args:
            - --metrics-addr=127.0.0.1:8080
            - --enable-leader-election
          env:
            - name: CONTROLLER_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: MAX_CONCURRENT_RECONCILES
              value: {{ .Values.maxConcurrentReconciles | quote }}
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
      terminationGracePeriodSeconds: 10
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
