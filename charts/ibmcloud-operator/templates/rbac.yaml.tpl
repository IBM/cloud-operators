apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "ibmcloud-operator.managerRoleName" . }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
rules:
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
  - apiGroups: ["ibmcloud.ibm.com"]
    resources: ["bindings", "bindings/finalizers", "services", "services/finalizers"]
    verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
  - apiGroups: ["ibmcloud.ibm.com"]
    resources: ["bindings/status", "services/status"]
    verbs: ["get", "patch", "update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ include "ibmcloud-operator.managerRoleName" . }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ include "ibmcloud-operator.managerRoleName" . }}
subjects:
  - kind: ServiceAccount
    name: {{ include "ibmcloud-operator.serviceAccountName" . }}
    namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ include "ibmcloud-operator.leaderElectionRoleName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["configmaps/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{ include "ibmcloud-operator.leaderElectionRoleName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ include "ibmcloud-operator.leaderElectionRoleName" . }}
subjects:
  - kind: ServiceAccount
    name: {{ include "ibmcloud-operator.serviceAccountName" . }}
    namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "ibmcloud-operator.proxyRoleName" . }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
rules:
  - apiGroups: ["authentication.k8s.io"]
    resources: ["tokenreviews"]
    verbs: ["create"]
  - apiGroups: ["authorization.k8s.io"]
    resources: ["subjectaccessreviews"]
    verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ include "ibmcloud-operator.proxyRoleName" . }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ include "ibmcloud-operator.proxyRoleName" . }}
subjects:
  - kind: ServiceAccount
    name: {{ include "ibmcloud-operator.serviceAccountName" . }}
    namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "ibmcloud-operator.metricsReaderRoleName" . }}
  labels:
    {{- include "ibmcloud-operator.labels" . | nindent 4 }}
rules:
  - nonResourceURLs: ["/metrics"]
    verbs: ["get"]
