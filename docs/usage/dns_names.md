---
title: DNS Names
---

# Request DNS Names in Shoot Clusters

## Introduction
Within a shoot cluster, it is possible to request DNS records via the following resource types:
- [Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/)
- [Service](https://kubernetes.io/docs/concepts/services-networking/service/)
- [DNSEntry](https://github.com/gardener/external-dns-management/blob/master/README.md#the-model)

It is necessary that the Gardener installation your shoot cluster runs in is equipped with a `shoot-dns-service` extension. This extension uses the seed's dns management infrastructure to maintain DNS names for shoot clusters. Please ask your Gardener operator if the extension is available in your environment.

## Shoot Feature Gate

In some Gardener setups the `shoot-dns-service` extension is not enabled globally and thus must be configured per shoot cluster. Please adapt the shoot specification by the configuration shown below to activate the extension individually.

```yaml
kind: Shoot
...
spec:
  extensions:
    - type: shoot-dns-service
...
```

## DNS providers, domain scope

Gardener can only manage DNS records on your behalf if you have proper DNS providers in place. Please consult [this page](./dns_providers.md) for more information.

## Request DNS records via Service/Ingress resources

To request a DNS name for an Ingress or Service object in the shoot cluster
it must be annotated with the DNS class `garden` and an annotation denoting
the desired DNS names.

Example for an annotated Ingress resource:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    dns.gardener.cloud/dnsnames: '*' # collects domains names from .spec.rules[].host
    dns.gardener.cloud/class: garden
    # If you are delegating the certificate management to Gardener, uncomment the following line
    #cert.gardener.cloud/purpose: managed
  name: test-ingress
  namespace: default
spec:
  rules:
    - host: test.ingress.my-dns-domain.com
      http:
        paths:
          - backend:
              service:
                name: my-service
                port:
                  number: 9000
            path: /
            pathType: Prefix
  tls:
    - hosts:
        - test.ingress.my-dns-domain.com
      secretName: my-cert-secret-name
```

For a Service (it must have the type `LoadBalancer`) this looks like this:

```yaml
apiVersion: v1
kind: Service
metadata:
  annotations:
    dns.gardener.cloud/class: garden
    dns.gardener.cloud/dnsnames: my.subdomain.for.some.domain.cloud
  name: my-service
  namespace: default
spec:
  ports:
    - port: 80
      protocol: TCP
      targetPort: 80
  type: LoadBalancer
```

The *dnsnames* annotation accepts a comma-separated list of DNS names, if
multiple names are required.

For an Ingress, the DNS names are already declared in the specification.
Nevertheless the *dnsnames* annotation must be present. Here a subset of the 
DNS names of the ingress can be specified. If DNS names for all names are
desired, the value `all` can be used.

If one of the accepted DNS names is a direct subname of the shoot's ingress
domain, this is already handled by the standard wildcard entry for the ingress
domain. Therefore this name should be excluded from the *dnsnames* list in the
annotation. If only this DNS name is configured in the ingress, no explicit 
DNS entry is required, and the DNS annotations should be omitted at all.

More examples can be found [here](https://github.com/gardener/external-dns-management/blob/master/examples/)

### Request DNS records for Service/Ingress resources using a DNSAnnotation resource

In rare cases it may not be possible to add annotations to a `Service` or `Ingress` resource object.
E.g. the helm chart used to deploy the resource may no be adaptable for some reasons or some automation is used,
which always restores the original content of the resource object by dropping any additional annotations.

In these cases you may use an additional `DNSAnnotation` resource.

The `DNSAnnotation` resource makes the DNS shoot service behave as if annotations have been added to the referenced 
resource.

For the Ingress example shown above, you can create a `DNSAnnotation` resource alternatively to provide
the annotations.

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSAnnotation
metadata:
  annotations:
    dns.gardener.cloud/class: garden
  name: test-ingress-annotation
  namespace: default
spec:
  resourceRef:
    kind: Ingress
    apiVersion: networking.k8s.io/v1
    name: test-ingress
    namespace: default
  annotations:
    dns.gardener.cloud/dnsnames: '*'
    dns.gardener.cloud/class: garden    
```

Note that the DNSAnnotation resource itself needs the `dns.gardener.cloud/class=garden` annotation.

No annotations in the Ingress resource would be needed.s

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-ingress
  namespace: default
spec:
  ...
```

Please note this only works for anotations known to the DNS shoot service, i.e with keys `dns.gardener.cloud/...`.

For more details, see also [DNSAnnotation objects](https://github.com/gardener/external-dns-management#dnsannotation-objects)

## Request DNS records via DNSEntry resources

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
    dns.gardener.cloud/class: garden
  name: dns
  namespace: default
spec:
  dnsName: "my.subdomain.for.shootsomain.cloud"
  ttl: 600
  # txt records, either text or targets must be specified
# text:
# - foo-bar
  targets:
  # target records (CNAME or A records)
  - 8.8.8.8
```

## DNS record events

The DNS controller publishes Kubernetes events for the resource which requested the DNS record (Ingress, Service, DNSEntry). These events reveal more information about the DNS requests being processed and are especially useful to check any kind of misconfiguration, e.g. requests for a domain you don't own.

Events for a successfully created DNS record:
```
$ kubectl -n default describe service my-service

Events:
  Type    Reason          Age                From                    Message
  ----    ------          ----               ----                    -------
  Normal  dns-annotation  19s                dns-controller-manager  my.subdomain.for.shootsomain.cloud: dns entry is pending
  Normal  dns-annotation  19s (x3 over 19s)  dns-controller-manager  my.subdomain.for.shootsomain.cloud: dns entry pending: waiting for dns reconciliation
  Normal  dns-annotation  9s (x3 over 10s)   dns-controller-manager  my.subdomain.for.shootsomain.cloud: dns entry active
```

Please note, events vanish after their retention period (usually `1h`).

## DNSEntry status

`DNSEntry` resources offer a `.status` sub-resource which can be used to check the current state of the object.

Status of a erroneous `DNSEntry`.
```
  status:
    message: No responsible provider found
    observedGeneration: 3
    provider: remote
    state: Error
```
