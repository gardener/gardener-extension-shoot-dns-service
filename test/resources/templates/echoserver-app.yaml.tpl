{{- if .ServiceTypeLoadBalancer }}
---
apiVersion: v1
kind: Service
metadata:
  name: echoserver
  namespace: {{ .Namespace }}
  labels:
    app: echoserver
  annotations:
    dns.gardener.cloud/class: garden
    dns.gardener.cloud/dnsnames: {{ .EchoName }}.{{ .ShootDnsName }}
spec:
  type: LoadBalancer
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: echoserver
{{- else }}
---
apiVersion: v1
kind: Service
metadata:
  name: echoserver
  namespace: {{ .Namespace }}
  labels:
    app: echoserver
spec:
  type: ClusterIP
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: echoserver
---
{{- if .OldIngress }}
apiVersion: networking.k8s.io/v1beta1
kind: Ingress
metadata:
  annotations:
    dns.gardener.cloud/dnsnames: '*'
    dns.gardener.cloud/class: garden
  name: echoserver
  namespace: {{ .Namespace }}
spec:
  rules:
    - host: {{ .EchoName }}.{{ .ShootDnsName }}
      http:
        paths:
          - backend:
              serviceName: echoserver
              servicePort: 80
            path: /
{{- else }}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    dns.gardener.cloud/dnsnames: '*'
    dns.gardener.cloud/class: garden
  name: echoserver
  namespace: {{ .Namespace }}
spec:
  rules:
    - host: {{ .EchoName }}.{{ .ShootDnsName }}
      http:
        paths:
          - backend:
              service:
                name: echoserver
                port:
                  number: 80
            path: /
            pathType: Prefix
{{- end }}
{{- end }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echoserver
  namespace: {{ .Namespace }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echoserver
  template:
    metadata:
      labels:
        app: echoserver
    spec:
      containers:
      - image: eu.gcr.io/gardener-project/3rd/hashicorp/http-echo:0.2.3
        args: ["-listen=:8080", "-text='Hello World'"]
        name: echoserver
        ports:
        - containerPort: 8080
        securityContext:
          runAsUser: 1000
