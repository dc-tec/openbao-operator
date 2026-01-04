{{/*
Expand the name of the chart.
*/}}
{{- define "openbao-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "openbao-operator.fullname" -}}
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
{{- define "openbao-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Operator version (for status reporting)
*/}}
{{- define "openbao-operator.operatorVersion" -}}
{{- if .Values.operatorVersion -}}
{{- .Values.operatorVersion -}}
{{- else -}}
{{- .Chart.AppVersion -}}
{{- end -}}
{{- end -}}

{{/*
Manager image reference
*/}}
{{- define "openbao-operator.managerImage" -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest -}}
{{- else -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
{{- end -}}

{{/*
Sentinel image repository
*/}}
{{- define "openbao-operator.sentinelImageRepository" -}}
{{- .Values.sentinel.imageRepository -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "openbao-operator.labels" -}}
helm.sh/chart: {{ include "openbao-operator.chart" . }}
app.kubernetes.io/name: {{ include "openbao-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
{{- end -}}

{{/*
Controller selector labels
*/}}
{{- define "openbao-operator.controllerSelectorLabels" -}}
app.kubernetes.io/name: {{ include "openbao-operator.name" . }}
app.kubernetes.io/component: controller
{{- end -}}

{{/*
Provisioner selector labels
*/}}
{{- define "openbao-operator.provisionerSelectorLabels" -}}
app.kubernetes.io/name: {{ include "openbao-operator.name" . }}
app.kubernetes.io/component: provisioner
{{- end -}}

{{/*
Controller service account name
*/}}
{{- define "openbao-operator.controllerServiceAccountName" -}}
{{- printf "%s-controller" (include "openbao-operator.fullname" .) -}}
{{- end -}}

{{/*
Provisioner service account name
*/}}
{{- define "openbao-operator.provisionerServiceAccountName" -}}
{{- printf "%s-provisioner" (include "openbao-operator.fullname" .) -}}
{{- end -}}

{{/*
Provisioner delegate service account name
*/}}
{{- define "openbao-operator.provisionerDelegateServiceAccountName" -}}
{{- printf "%s-provisioner-delegate" (include "openbao-operator.fullname" .) -}}
{{- end -}}
