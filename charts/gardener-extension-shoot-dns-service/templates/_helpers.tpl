{{-  define "image" -}}
  {{- if .Values.image.ref }}
    {{- .Values.image.ref }}
  {{- else }}
    {{- if hasPrefix "sha256:" .Values.image.tag }}
    {{- printf "%s@%s" .Values.image.repository .Values.image.tag }}
    {{- else }}
    {{- printf "%s:%s" .Values.image.repository .Values.image.tag }}
    {{- end }}
  {{- end }}
{{- end }}

{{-  define "dnsmanImage" -}}
  {{- if hasPrefix "sha256:" .Values.dnsControllerManager.image.tag }}
  {{- printf "%s@%s" .Values.dnsControllerManager.image.repository .Values.dnsControllerManager.image.tag }}
  {{- else }}
  {{- printf "%s:%s" .Values.dnsControllerManager.image.repository .Values.dnsControllerManager.image.tag }}
  {{- end }}
{{- end }}

{{- define "leaderelectionid" -}}
extension-shoot-dns-service-leader-election
{{- end -}}

{{- define "labels" -}}
app.kubernetes.io/name: gardener-extension-{{ .Values.serviceName }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "dnsmanLabels" -}}
app.kubernetes.io/name: gardener-extension-{{ .Values.serviceName }}
app.kubernetes.io/instance: dns-controller-manager
{{- end -}}

{{- define "dnsmanRemoteAccessLoadBalancerType" -}}
{{- if not .Values.dnsControllerManager.remoteaccess.service -}}
ClusterIP
{{- else -}}
{{ .Values.dnsControllerManager.remoteaccess.service.type | default "ClusterIP" }}
{{- end }}
{{- end -}}
