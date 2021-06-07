# DNS Providers

## Introduction

Gardener can manage DNS records on your behalf, so that you can request them via different resource types (see [here](./dns_names.md)) within the shoot cluster. The domains for which you are permitted to request records, are however restricted and depend on the DNS provider configuration.

## Shoot provider

By default, every shoot cluster is equipped with a default provider. It is the very same provider that manages the shoot cluster's `kube-apiserver` public DNS record (DNS address in your Kubeconfig).

```
kind: Shoot
...
dns:
  domain: shoot.project.default-domain.gardener.cloud
```

You are permitted to request any sub-domain of `.dns.domain` that is not already taken (e.g. `api.shoot.project.default-domain.gardener.cloud`, `*.ingress.shoot.project.default-domain.gardener.cloud`) with this provider.

## Additional providers

If you need to request DNS records for domains not managed by the [default provider](#Shoot-provider), additional providers can either
be configured in the shoot specification or added as `DNSProvider` resources to the shoot cluster.

### Additional providers in the shoot specification

To add a providers in the shoot spec, you need set them in the `spec.dns.providers` list.

For example:
```yaml
kind: Shoot
...
spec:
  dns:
    domain: shoot.project.default-domain.gardener.cloud
    providers:
    - secretName: my-aws-account
      type: aws-route53
    - secretName: my-gcp-account
      type: google-clouddns
```

> Please consult the [API-Reference](https://gardener.cloud/documentation/references/core/#core.gardener.cloud/v1beta1.DNSProvider) to get a complete list of supported fields and configuration options.

Referenced secrets should exist in the project namespace in the Garden cluster and must comply with the provider specific credentials format. The **External-DNS-Management** project provides corresponding examples ([20-secret-\<provider-name>-credentials.yaml](https://github.com/gardener/external-dns-management/tree/master/examples)) for known providers.

### Additional providers as resources in the shoot cluster

To add a provider directly in the shoot cluster, provide a `DNSProvider` in any namespace together
with `Secret` containing the credentials.

For example if the domain is hosted with AWS Route 53 (provider type `aws-route53`):
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSProvider
metadata:
  name: my-own-domain
  namespace: my-namespace
spec:
  type: aws-route53
  secretRef:
    name: my-own-domain-credentials
  domains:
    include:
    - my.own.domain.com
---
apiVersion: v1
kind: Secret
metadata:
  name: my-own-domain-credentials
  namespace: my-namespace
type: Opaque
data:
  # replace '...' with values encoded as base64
  AWS_ACCESS_KEY_ID: ...
  AWS_SECRET_ACCESS_KEY: ...
```

The **External-DNS-Management** project provides examples with more details for `DNSProviders` (30-provider-\<provider-name>.yaml)
and credential `Secrets` (20-secret-\<provider-name>.yaml) at [https://github.com/gardener/external-dns-management//examples](https://github.com/gardener/external-dns-management/tree/master/examples)
for all supported provider types.

*Note*: This feature can be disabled in the `ControllerDeployment` with setting
`dnsProviderReplication.enabled` to `false` 