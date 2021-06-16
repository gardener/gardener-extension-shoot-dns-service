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
