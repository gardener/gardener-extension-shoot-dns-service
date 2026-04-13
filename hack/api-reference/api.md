<p>Packages:</p>
<ul>
<li>
<a href="#dns.extensions.gardener.cloud%2fv1alpha1">dns.extensions.gardener.cloud/v1alpha1</a>
</li>
</ul>

<h2 id="dns.extensions.gardener.cloud/v1alpha1">dns.extensions.gardener.cloud/v1alpha1</h2>
<p>

</p>

<h3 id="dnsentry">DNSEntry
</h3>


<p>
(<em>Appears on:</em><a href="#dnsstate">DNSState</a>)
</p>

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
<p></p>
</td>
</tr>
<tr>
<td>
<code>labels</code></br>
<em>
object (keys:string, values:string)
</em>
</td>
<td>
<p></p>
</td>
</tr>
<tr>
<td>
<code>annotations</code></br>
<em>
object (keys:string, values:string)
</em>
</td>
<td>
<p></p>
</td>
</tr>
<tr>
<td>
<code>spec</code></br>
<em>
<a href="#dnsentryspec">DNSEntrySpec</a>
</em>
</td>
<td>
<p></p>
</td>
</tr>

</tbody>
</table>


<h3 id="dnsstate">DNSState
</h3>


<p>
DNSState describes the set of DNS entries maintained by the dns shoot service
for a dedicated shoot cluster used to reconstruct the DNS entry objects after
a migration.
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
<code>entries</code></br>
<em>
<a href="#dnsentry">DNSEntry</a> array
</em>
</td>
<td>
<p></p>
</td>
</tr>

</tbody>
</table>


