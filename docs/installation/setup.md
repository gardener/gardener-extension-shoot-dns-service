# Gardener DNS Management for Shoots

## Introduction
Gardener allows Shoot clusters to request DNS names for Ingresses and Services out of the box. 
To support this the gardener must be installed with the `shoot-dns-service`
extension.
This extension uses the seed's dns management infrastructure to maintain DNS
names for shoot clusters. So, far only the external DNS domain of a shoot
(already used for the kubernetes api server and ingress DNS names) can be used
for managed DNS names.

## Configuration

A general description for configuring the DNS management of the
gardener can be found [here](https://github.com/gardener/gardener/blob/master/docs/extensions/dns.md).

To generally enable the DNS management for shoot objects the 
`shoot-dns-service` extension must be registered by providing an
appropriate [extension registration](https://github.com/gardener/gardener-extension-shoot-dns-service/blob/master/example/controller-registration.yaml) in the garden cluster.

Here it is possible to decide whether the extension should be always available
for all shoots or whether the extension must be separately enabled per shoot.

If the extension should be used for all shoots the registration must set the *globallyEnabled* flag to `true`.

```yaml
spec:
  resources:
    - kind: Extension
      type: shoot-dns-service
      globallyEnabled: true
```

### Deployment of DNS controller manager

If you are using Gardener version >= `1.54`, please make sure to deploy the DNS controller manager by 
adding the `dnsControllerManager` section to the `providerConfig.values` section.

For example:

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerDeployment
metadata:
  name: extension-shoot-dns-service
type: helm
providerConfig:
  chart: ...
  values:
    image:
      ...
    dnsControllerManager:
      configuration:
        cacheTtl: 300
        controllers: dnscontrollers,dnssources
        dnsPoolResyncPeriod: 30m
        #poolSize: 20
        #providersPoolResyncPeriod: 24h
        serverPortHttp: 8080
      createCRDs: false
      deploy: true
      replicaCount: 1
      #resources:
      #  limits:
      #    memory: 1Gi
      #  requests:
      #    cpu: 50m
      #    memory: 500Mi
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
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerDeployment
metadata:
  name: extension-shoot-dns-service
type: helm
providerConfig:
  chart: ...
  values:
    image:
      ...
    dnsProviderReplication:
      enabled: true
```

See [example files (20-* and 30-*)](https://github.com/gardener/external-dns-management/tree/master/examples)
for details for the various provider types.


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

### DNSActivation for DNSOwner

To support migration of the control plane of shoots, the `DNSOwner` created and used for all `DNSEntries` created by the
Shoot-DNS-Service can optionally be activated and deactivated by a DNS record.
If the DNSActivation feature is enabled, the `DNSOwner` will be only be active if the value of the owner DNS record 
managed by Gardener matches the cluster identity of the seed hosting the control plane.
This feature must only be enabled if the Gardener feature gate `UseDNSRecords` is enabled for all seeds. 

By default this feature is enabled and can be disabled in the controller deployment:

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerDeployment
metadata:
  name: extension-shoot-dns-service
type: helm
providerConfig:
  chart: ...
  values:
    image:
      ...
    ownerDnsActivation:
      enabled: false
```

