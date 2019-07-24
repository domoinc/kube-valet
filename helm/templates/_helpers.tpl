{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "kube-valet.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kube-valet.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kube-valet.chart" -}}
{{- end -}}

{{- define "kube-valet.webhook-config" -}}
apiVersion: admissionregistration.k8s.io/v1beta1
kind: MutatingWebhookConfiguration
metadata:
  name: scheduling.kube-valet.io
webhooks:
- name: scheduling.kube-valet.io
  # Allow namespace exclusion via a label
  namespaceSelector:
    matchExpressions:
    - key: kube-valet.io/ignore
      operator: DoesNotExist
{{- if not .Values.global }}
    - key: kube-valet.io/enabled
      operator: Exists
{{- end }}
  failurePolicy: Fail
  clientConfig:
{{- if not .Values.tls.auto }}
    caBundle: {{ .Files.Get .Values.tls.caPath | b64enc }}
{{- else }}
    caBundle: __AUTO_TLS_CA_BUNDLE__
{{- end }}
    service:
      namespace: kube-valet
      name: kube-valet
      path: /mutate
  rules:
  - operations: ["CREATE"]
    apiGroups: [""]
    apiVersions: ["*"]
    resources: ["pods"]
{{- end -}}
