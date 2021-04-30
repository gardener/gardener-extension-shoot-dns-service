---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
    dns.gardener.cloud/class: garden
  name: custom
  namespace: {{ .Namespace }}
spec:
  dnsName: {{ .DNSName }}
  ttl: 600
  targets:
  - 8.8.8.8
