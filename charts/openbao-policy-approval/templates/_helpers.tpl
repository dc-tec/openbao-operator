{{- define "approval.serviceAccount" -}}
{{- default (printf "%s-approver" .Release.Name) .Values.serviceAccount.name -}}
{{- end -}}

{{- define "approval.target" -}}
{{- printf "%s/%s" .Values.cluster.namespace .Values.cluster.name | sha256sum | trunc 16 -}}
{{- end -}}

{{- define "approval.bundle" -}}
{{- if not (regexMatch "^v[1-9][0-9]*/(rolling-update|blue-green)(-backup)?$" .id) -}}
{{- fail (printf "invalid approval bundle ID %q" .id) -}}
{{- end -}}
{{- $body := .root.Files.Get (printf "bundles/%s.hcl" .id) -}}
{{- if not $body -}}{{- fail (printf "bundle %q is not shipped in this chart" .id) -}}{{- end -}}
{{- $body -}}
{{- end -}}

{{- define "approval.prerequisiteAnnotations" -}}
{{- if eq .Values.gitops "argocd" }}
annotations:
  argocd.argoproj.io/sync-wave: "-2"
{{- end -}}
{{- end -}}

{{- define "approval.validate" -}}
{{- if or (empty .Values.cluster.name) (empty .Values.cluster.namespace) -}}
{{- fail "cluster.name and cluster.namespace are required" -}}
{{- end -}}
{{- if eq .Release.Namespace .Values.cluster.namespace -}}
{{- fail "install the approval chart in an administration namespace outside the managed cluster namespace" -}}
{{- end -}}
{{- if not (has .Values.gitops (list "plain" "argocd")) -}}
{{- fail "gitops must be plain or argocd" -}}
{{- end -}}
{{- if and .Values.tls.caBundle .Values.tls.caConfigMapName -}}
{{- fail "choose tls.caBundle or tls.caConfigMapName" -}}
{{- end -}}
{{- if ne (empty .Values.approval.from) (empty .Values.approval.to) -}}
{{- fail "approval.from and approval.to must be selected together" -}}
{{- end -}}
{{- if and .Values.approval.to (not (regexMatch "^[^@[:space:]]+@sha256:[a-f0-9]{64}$" .Values.image)) -}}
{{- fail "image must be pinned as repository@sha256:<digest> when requesting approval" -}}
{{- end -}}
{{- end -}}
