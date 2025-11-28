{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "service.name" -}}
{{- default .Chart.Name .Values.serviceName | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "service.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create next generation config
*/}}
{{- define "next-generation-config"}}
apiVersion: config.dns.gardener.cloud/v1alpha1
kind: DNSManagerConfiguration
class: {{ .Values.nextGeneration.dnsClass }}
logFormat: json
logLevel: info
clientConnection:
  kubeconfig: /var/run/secrets/gardener.cloud/shoot/generic-kubeconfig/kubeconfig
controlPlaneClientConnection:
  kubeconfig: IN-CLUSTER
controllers:
  dnsProvider:
    namespace: {{ .Release.Namespace }}
    migrationMode: true
  source:
    sourceClass: {{ .Values.dnsClass }}
    targetClass: {{ .Values.nextGeneration.dnsClass }}
    targetNamespace: {{ .Release.Namespace }}
    targetLabels:
      gardener.cloud/shoot-id: {{ .Values.creatorLabelValue }}
    targetClusterID: {{ .Values.seedId }}
    sourceClusterID: {{ .Values.shootId }}
    {{ if .Values.dnsProviderReplication.enabled }}
    dnsProviderReplication: true
    {{ end }}
leaderElection:
  leaderElect: true
  resourceName: shoot-dns-service
  resourceNamespace: kube-system
deployCRDs: true
conditionalDeployCRDs: true
addShootNoCleanupLabelToCRDs: true
{{- end}}