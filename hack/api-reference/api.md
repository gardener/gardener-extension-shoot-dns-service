<p>Packages:</p>
<ul>
<li>
<a href="#dns.extensions.gardener.cloud%2fv1alpha1">dns.extensions.gardener.cloud/v1alpha1</a>
</li>
</ul>
<h2 id="dns.extensions.gardener.cloud/v1alpha1">dns.extensions.gardener.cloud/v1alpha1</h2>
<p>
<p>Package v1alpha1 contains the vSphere provider API resources.</p>
</p>
Resource Types:
<ul><li>
<a href="#dns.extensions.gardener.cloud/v1alpha1.DNSState">DNSState</a>
</li></ul>
<h3 id="dns.extensions.gardener.cloud/v1alpha1.DNSState">DNSState
</h3>
<p>
<p>DNSState describes the set of DNS entries maintained by the dns shoot service
for a dedicated shoot cluster used to reconstruct the DNS entry objects after
a migration.</p>
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
dns.extensions.gardener.cloud/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code></br>
string
</td>
<td><code>DNSState</code></td>
</tr>
<tr>
<td>
<code>entries</code></br>
<em>
<a href="#dns.extensions.gardener.cloud/v1alpha1.*..DNSEntry">
[]*..DNSEntry
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="dns.extensions.gardener.cloud/v1alpha1.DNSEntry">DNSEntry
</h3>
<p>
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
<code>name</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>labels</code></br>
<em>
map[string]string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>annotations</code></br>
<em>
map[string]string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>spec</code></br>
<em>
github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1.DNSEntrySpec
</em>
</td>
<td>
<br/>
<br/>
<table>
</table>
</td>
</tr>
</tbody>
</table>
<hr/>
<p><em>
Generated with <a href="https://github.com/ahmetb/gen-crd-api-reference-docs">gen-crd-api-reference-docs</a>
</em></p>
