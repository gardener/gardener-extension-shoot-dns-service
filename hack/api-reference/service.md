<p>Packages:</p>
<ul>
<li>
<a href="#service.dns.extensions.gardener.cloud%2fv1alpha1">service.dns.extensions.gardener.cloud/v1alpha1</a>
</li>
</ul>
<h2 id="service.dns.extensions.gardener.cloud/v1alpha1">service.dns.extensions.gardener.cloud/v1alpha1</h2>
<p>
<p>Package v1alpha1 contains the DNS Shoot Service extension.</p>
</p>
Resource Types:
<ul><li>
<a href="#service.dns.extensions.gardener.cloud/v1alpha1.DNSConfig">DNSConfig</a>
</li></ul>
<h3 id="service.dns.extensions.gardener.cloud/v1alpha1.DNSConfig">DNSConfig
</h3>
<p>
<p>DNSConfig configuration resource</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code></br>
string</td>
<td>
<code>
service.dns.extensions.gardener.cloud/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code></br>
string
</td>
<td><code>DNSConfig</code></td>
</tr>
<tr>
<td>
<code>dnsProviderReplication</code></br>
<em>
<a href="#service.dns.extensions.gardener.cloud/v1alpha1.DNSProviderReplication">
DNSProviderReplication
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DNSProviderReplication contains enablement for replication of DNSProviders from shoot cluster to control plane</p>
</td>
</tr>
<tr>
<td>
<code>providers</code></br>
<em>
<a href="#service.dns.extensions.gardener.cloud/v1alpha1.DNSProvider">
[]DNSProvider
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Providers is a list of additional DNS providers that shall be enabled for this shoot cluster.
The primary (&ldquo;external&rdquo;) provider at <code>spec.dns.provider</code> is added automatically</p>
</td>
</tr>
<tr>
<td>
<code>syncProvidersFromShootSpecDNS</code></br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>SyncProvidersFromShootSpecDNS is an optional flag for migrating and synchronising the providers given in the
shoot manifest at section <code>spec.dns.providers</code>. If true, any direct changes on the <code>providers</code> section
are overwritten with the content of section <code>spec.dns.providers</code>.</p>
</td>
</tr>
<tr>
<td>
<code>useNextGenerationController</code></br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>UseNextGenerationController is an optional flag to enable the next generation DNS controller for this shoot cluster.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="service.dns.extensions.gardener.cloud/v1alpha1.DNSIncludeExclude">DNSIncludeExclude
</h3>
<p>
(<em>Appears on:</em>
<a href="#service.dns.extensions.gardener.cloud/v1alpha1.DNSProvider">DNSProvider</a>)
</p>
<p>
<p>DNSIncludeExclude contains information about which domains shall be included/excluded.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>include</code></br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Include is a list of domains that shall be included.</p>
</td>
</tr>
<tr>
<td>
<code>exclude</code></br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Exclude is a list of domains that shall be excluded.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="service.dns.extensions.gardener.cloud/v1alpha1.DNSProvider">DNSProvider
</h3>
<p>
(<em>Appears on:</em>
<a href="#service.dns.extensions.gardener.cloud/v1alpha1.DNSConfig">DNSConfig</a>)
</p>
<p>
<p>DNSProvider contains information about a DNS provider.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>domains</code></br>
<em>
<a href="#service.dns.extensions.gardener.cloud/v1alpha1.DNSIncludeExclude">
DNSIncludeExclude
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Domains contains information about which domains shall be included/excluded for this provider.</p>
</td>
</tr>
<tr>
<td>
<code>secretName</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SecretName is a name of a secret containing credentials for the stated domain and the
provider.</p>
</td>
</tr>
<tr>
<td>
<code>credentials</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Credentials is the name of the resource reference containing the credentials for the provider.
It is an alternative to SecretName and can reference either a secret or a workload identity.</p>
</td>
</tr>
<tr>
<td>
<code>type</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Type is the DNS provider type.</p>
</td>
</tr>
<tr>
<td>
<code>zones</code></br>
<em>
<a href="#service.dns.extensions.gardener.cloud/v1alpha1.DNSIncludeExclude">
DNSIncludeExclude
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Zones contains information about which hosted zones shall be included/excluded for this provider.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="service.dns.extensions.gardener.cloud/v1alpha1.DNSProviderReplication">DNSProviderReplication
</h3>
<p>
(<em>Appears on:</em>
<a href="#service.dns.extensions.gardener.cloud/v1alpha1.DNSConfig">DNSConfig</a>)
</p>
<p>
<p>DNSProviderReplication contains enablement for replication of DNSProviders from shoot cluster to control plane</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code></br>
<em>
bool
</em>
</td>
<td>
<p>Enabled if true, the replication of DNSProviders from shoot cluster to the control plane is enabled</p>
</td>
</tr>
</tbody>
</table>
<hr/>
<p><em>
Generated with <a href="https://github.com/ahmetb/gen-crd-api-reference-docs">gen-crd-api-reference-docs</a>
</em></p>
