<p>Packages:</p>
<ul>
<li>
<a href="#service.dns.extensions.gardener.cloud%2fv1alpha1">service.dns.extensions.gardener.cloud/v1alpha1</a>
</li>
</ul>

<h2 id="service.dns.extensions.gardener.cloud/v1alpha1">service.dns.extensions.gardener.cloud/v1alpha1</h2>
<p>

</p>

<h3 id="dnsconfig">DNSConfig
</h3>


<p>
DNSConfig configuration resource
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
<code>dnsProviderReplication</code></br>
<em>
<a href="#dnsproviderreplication">DNSProviderReplication</a>
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
<a href="#dnsprovider">DNSProvider</a> array
</em>
</td>
<td>
<em>(Optional)</em>
<p>Providers is a list of additional DNS providers that shall be enabled for this shoot cluster.<br />The primary ("external") provider at `spec.dns.provider` is added automatically</p>
</td>
</tr>
<tr>
<td>
<code>syncProvidersFromShootSpecDNS</code></br>
<em>
boolean
</em>
</td>
<td>
<em>(Optional)</em>
<p>SyncProvidersFromShootSpecDNS is an optional flag for migrating and synchronising the providers given in the<br />shoot manifest at section `spec.dns.providers`. If true, any direct changes on the `providers` section<br />are overwritten with the content of section `spec.dns.providers`.</p>
</td>
</tr>
<tr>
<td>
<code>useNextGenerationController</code></br>
<em>
boolean
</em>
</td>
<td>
<em>(Optional)</em>
<p>UseNextGenerationController is an optional flag to enable the next generation DNS controller for this shoot cluster.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="dnsincludeexclude">DNSIncludeExclude
</h3>


<p>
(<em>Appears on:</em><a href="#dnsprovider">DNSProvider</a>)
</p>

<p>
DNSIncludeExclude contains information about which domains shall be included/excluded.
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
string array
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
string array
</em>
</td>
<td>
<em>(Optional)</em>
<p>Exclude is a list of domains that shall be excluded.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="dnsprovider">DNSProvider
</h3>


<p>
(<em>Appears on:</em><a href="#dnsconfig">DNSConfig</a>)
</p>

<p>
DNSProvider contains information about a DNS provider.
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
<a href="#dnsincludeexclude">DNSIncludeExclude</a>
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
<p>SecretName is a name of a secret containing credentials for the stated domain and the<br />provider.<br />Deprecated: This field is deprecated and will be removed in the future. Please use `Credentials` instead.</p>
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
<p>Credentials is the name of the resource reference containing the credentials for the provider.<br />It is an alternative to SecretName and can reference either a secret or a workload identity.</p>
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
<a href="#dnsincludeexclude">DNSIncludeExclude</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Zones contains information about which hosted zones shall be included/excluded for this provider.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="dnsproviderreplication">DNSProviderReplication
</h3>


<p>
(<em>Appears on:</em><a href="#dnsconfig">DNSConfig</a>)
</p>

<p>
DNSProviderReplication contains enablement for replication of DNSProviders from shoot cluster to control plane
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
boolean
</em>
</td>
<td>
<p>Enabled if true, the replication of DNSProviders from shoot cluster to the control plane is enabled</p>
</td>
</tr>

</tbody>
</table>


