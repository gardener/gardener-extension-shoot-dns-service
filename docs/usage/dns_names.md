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

## Before you start
You should :
- Have created a shoot cluster
- Have created and correctly configured a DNS Provider(Please consult [this page](./dns_providers.md) for more information)
- Have a basic understanding of DNS (see link under [References](#references))

There are 2 types of DNS that you can use within Kubernetes : 
- internal (usually managed by coreDNS)
- external (managed by a public DNS provider). 

This page, and the extension, exclusively works for external DNS handling.

Gardener allows 2 way of managing your external DNS:
- Manually, which means your are in charge of creating / maintaining your Kubernetes related DNS entries
- Via the Gardener DNS extension

## Gardener DNS extension
The managed external DNS records feature of the Gardener clusters makes all this easier. You do not need DNS service provider specific knowledge, and in fact you do not need to leave your cluster at all to achieve that. You simply annotate the Ingress or Service that needs its DNS records managed and they will be automatically created and managed by Gardener.

Managed external DNS records are suported with the following DNS provider types:
- aws-route53
- azure-dns
- azure-private-dns
- google-clouddns
- openstack-designate
- alicloud-dns

### Request DNS records for Ingress resources

To request a DNS name for an Ingress or Service object in the shoot cluster
it must be annotated with the DNS class `garden` and an annotation denoting
the desired DNS names.

Example for an annotated Ingress resource:

```yaml
apiVersion: networking.k8s.io/v1beta1
kind: Ingress
metadata:
  name: amazing-ingress
  annotations:
    # Let Gardener manage external DNS records for this Ingress.
    dns.gardener.cloud/dnsnames: special.example.com # Use "*" to collects domains names from .spec.rules[].host
    dns.gardener.cloud/ttl: "600"
    dns.gardener.cloud/class: garden
spec:
  rules:
  - host: special.example.com
    http:
      paths:
      - backend:
        serviceName: amazing-svc
        servicePort: 8080
```

Keep in mind that ingress resources are ignored unless an ingress controller is setup. Gardener does not provide an ingress controller by default . See the [Kubernetes documentation](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/) and [Gardener documentation](https://github.com/gardener/documentation/blob/master/website/documentation/guides/applications/service-access/_index.md#loadbalancer-vs-ingress) for more details.

### Request DNS records for service type LoadBalancer

Example for an annotated Service (it must have the type `LoadBalancer`) resource:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: amazing-svc
  annotations:
    # Let Gardener manage external DNS records for this Service.
    dns.gardener.cloud/dnsnames: special.example.com
    dns.gardener.cloud/ttl: "600"
    dns.gardener.cloud/class: garden
spec:
  selector:
    app: amazing-app
  ports:
    - protocol: TCP
      port: 80
      targetPort: 8080
  type: LoadBalancer
```

#### Creating a DNSEntry resource explicitly
It is also possible to create a DNS entry via the kubernetes resource called `DNSEntry`:
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
    # Let Gardener manage this DNS entry.
    dns.gardener.cloud/class: garden
  name: special-dnsentry
  namespace: default
spec:
  dnsName: special.example.com
  ttl: 600
  targets:
  - 1.2.3.4
```

You can check the status of the `DNSentry` with
```bash
$ kubectl get dnsentry
NAME          DNS                                                            TYPE          PROVIDER      STATUS    AGE
mydnsentry    special.example.com     aws-route53   default/aws   Ready     24s
```
As soon as the status of the entry is `Ready`, the provider has accepted the new DNS record. Depending on the provider and your DNS settings and cache, **it may take up to 24 hours for the new entry to be propagated over all internet**.

More examples can be found [here](https://github.com/gardener/external-dns-management/blob/master/examples/)

### Request DNS records for Service/Ingress resources using a DNSAnnotation resource

In rare cases it may not be possible to add annotations to a `Service` or `Ingress` resource object.
E.g. the helm chart used to deploy the resource may no be adaptable for some reasons or some automation is used,
which always restores the original content of the resource object by dropping any additional annotations.

In these cases, it is recommended to use an additional `DNSAnnotation` resource in order to have more flexibility that `DNSentry resources`. The `DNSAnnotation` resource makes the DNS shoot service behave as if annotations have been added to the referenced 
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


Please note this only works for annotations known to the DNS shoot service, i.e with keys `dns.gardener.cloud/...`.

For more details, see also [DNSAnnotation objects](https://github.com/gardener/external-dns-management#dnsannotation-objects)

### Accepted External DNS Records Annotations

Here are all of the accepted annotation related to the DNS extension:
```bash
- dns.gardener.cloud/dnsnames # Mandatory, accepts a comma-separated list of DNS names if multiple names are required
- dns.gardener.cloud/class # Mandatory, dns extension class name (usually "garden")
- dns.gardener.cloud/ttl # Recommended, Time-To-Live of the DNS record
- dns.gardener.cloud/cname-lookup-interval # Optional, lookup interval for CNAMEs that must be resolved to IP (in seconds)
- dns.gardener.cloud/realms # Optional, for restricting provider access for shoot DNS entries
```


If one of the accepted DNS names is a direct subname of the shoot's ingress
domain, this is already handled by the standard wildcard entry for the ingress
domain. Therefore this name should be excluded from the *dnsnames* list in the
annotation. If only this DNS name is configured in the ingress, no explicit 
DNS entry is required, and the DNS annotations should be omitted at all.

## Troubleshooting
### General DNS tools
To check the DNS resolution, use the `nslookup` or ``dig`` command.
```bash
$ nslookup special.your-domain.com
```
or with dig
```bash
$ dig +short special.example.com
Depending on your network settings, you may get a successful response faster using a public DNS server (e.g. 8.8.8.8, 8.8.4.4, or 1.1.1.1)

dig @8.8.8.8 +short special.example.com
```

### DNS record events

The DNS controller publishes Kubernetes events for the resource which requested the DNS record (Ingress, Service, DNSEntry). These events reveal more information about the DNS requests being processed and are especially useful to check any kind of misconfiguration, e.g. requests for a domain you don't own.

Events for a successfully created DNS record:
```
$ kubectl describe service my-service

Events:
  Type    Reason          Age                From                    Message
  ----    ------          ----               ----                    -------
  Normal  dns-annotation  19s                dns-controller-manager  special.example.com: dns entry is pending
  Normal  dns-annotation  19s (x3 over 19s)  dns-controller-manager  special.example.com: dns entry pending: waiting for dns reconciliation
  Normal  dns-annotation  9s (x3 over 10s)   dns-controller-manager  special.example.com: dns entry active
```

Please note, events vanish after their retention period (usually `1h`).

### DNSEntry status

`DNSEntry` resources offer a `.status` sub-resource which can be used to check the current state of the object.

Status of a erroneous `DNSEntry`.
```
  status:
    message: No responsible provider found
    observedGeneration: 3
    provider: remote
    state: Error
```

## References
- [DNSEntry and DNSProvider](https://github.com/gardener/gardener/blob/master/docs/extensions/dns.md)
- [Understanding DNS](https://www.cloudflare.com/en-ca/learning/dns/what-is-dns)
- [Kubernetes Internal DNS](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/)
- [DNSEntry API (Golang)](https://github.com/gardener/external-dns-management/blob/master/pkg/apis/dns/v1alpha1/dnsentry.go)
- [Managing Certificates with Gardener](https://github.com/gardener/gardener-extension-shoot-cert-service/blob/master/docs/usage/request_cert.md)