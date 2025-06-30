# Gardener DNS Management for Shoots

## Introduction
Gardener allows Shoot clusters to request DNS names for Ingresses and Services out of the box. 
To support this the gardener must be installed with the `shoot-dns-service`
extension.
This extension uses the seed's dns management infrastructure to maintain DNS
names for shoot clusters. So, far only the external DNS domain of a shoot
(already used for the kubernetes api server and ingress DNS names) can be used
for managed DNS names.

## Operator Extension

Using an operator extension resource (`extension.operator.gardener.cloud`) is the recommended way to deploy the `shoot-dns-service` extension.

An example of an `operator` extension resource can be found at [extension-shoot-dns-service.yaml](../../example/extension-shoot-dns-service.yaml).

It is possible to decide whether the extension should be always available for all shoots or whether the extension must be separately enabled per shoot.
To enable the extension for all shoots, the `autoEnable` field must be set to `[shoot]` in the `Extension` resource.

```yaml
apiVersion: operator.gardener.cloud/v1alpha1
kind: Extension
metadata:
  annotations:
    security.gardener.cloud/pod-security-enforce: baseline
  name: extension-shoot-dns-service
spec:
  deployment:
    admission:
      runtimeCluster:
        helm:
          ociRepository:
            ref: ... # OCI reference to the Helm chart
      virtualCluster:
        helm:
          ociRepository:
            ref: ... # OCI reference to the Helm chart
    extension:
      helm:
        ociRepository:
          ref: ... # OCI reference to the Helm chart

  resources:
  - autoEnable:
    - shoot # if set, the extension is enabled for all shoots by default
    clusterCompatibility:
    - shoot
    kind: Extension
    type: shoot-dns-service
    workerlessSupported: true
```

### Providing Base Domains usable for a Shoot

So, far only the external DNS domain of a shoot already used
for the kubernetes api server and ingress DNS names can be used for managed
DNS names. This is either the shoot domain as subdomain of the default domain
configured for the gardener installation, or a dedicated domain with dedicated
access credentials configured for a dedicated shoot via the shoot manifest.

Alternatively, you can specify `DNSProviders` and its credentials
`Secret` directly in the shoot, if this feature is enabled.
By default, `DNSProvider` replication is disabled, but it can be enabled globally in the `ControllerDeployment`
or for a shoot cluster in the shoot manifest (details see further below). 

```yaml
apiVersion: operator.gardener.cloud/v1alpha1
kind: Extension
metadata:
  name: extension-shoot-dns-service
spec:
  extension:
    values:
      dnsProviderReplication:
        enabled: true
```

### Shoot Feature Gate

If the shoot DNS feature is not globally enabled by default (depends on the 
extension registration on the garden cluster), it must be enabled per shoot.

To enable the feature for a shoot, the shoot manifest must explicitly add the
`shoot-dns-service` extension.

```yaml
...
spec:
  extensions:
    - type: shoot-dns-service
...
```

#### Enable/disable DNS provider replication for a shoot

The DNSProvider` replication feature enablement can be overwritten in the
shoot manifest, e.g.

```yaml
Kind: Shoot
...
spec:
  extensions:
    - type: shoot-dns-service
      providerConfig:
        apiVersion: service.dns.extensions.gardener.cloud/v1alpha1
        kind: DNSConfig
        dnsProviderReplication:
          enabled: true
...
```
