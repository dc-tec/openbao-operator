{{- define "openbao-operator.name" -}}
openbao-operator
{{- end -}}

{{- define "openbao-operator.operatorVersion" -}}
{{- if .Values.operatorVersion -}}
{{- .Values.operatorVersion -}}
{{- else -}}
{{- .Chart.AppVersion -}}
{{- end -}}
{{- end -}}

{{- define "openbao-operator.managerImage" -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest -}}
{{- else -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
{{- end -}}

{{- define "openbao-operator.sentinelImageRepository" -}}
{{- .Values.sentinel.imageRepository -}}
{{- end -}}
