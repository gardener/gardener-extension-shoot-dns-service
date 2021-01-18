{{-  define "image" -}}
  {{- if hasPrefix "sha256:" .Values.image.tag }}
  {{- printf "%s@%s" .Values.image.repository .Values.image.tag }}
  {{- else }}
  {{- printf "%s:%s" .Values.image.repository .Values.image.tag }}
  {{- end }}
{{- end }}

{{- define "priorityclassversion" -}}
{{- if semverCompare ">= 1.14-0" .Capabilities.KubeVersion.GitVersion -}}
scheduling.k8s.io/v1
{{- else if semverCompare ">= 1.11-0" .Capabilities.KubeVersion.GitVersion -}}
scheduling.k8s.io/v1beta1
{{- else -}}
scheduling.k8s.io/v1alpha1
{{- end -}}
{{- end -}}

{{- define "leaderelectionid" -}}
extension-shoot-dns-service-leader-election
{{- end -}}