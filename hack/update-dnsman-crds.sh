#!/bin/bash
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

repo_root="$(readlink -f "$(dirname "${0}")"/..)"

# create temporary directory
tmp_dir=$(mktemp -d)
trap 'rm -rf "${tmp_dir}"' EXIT

# ensure that the go module is initialized
go get github.com/gardener/external-dns-management@$(go list -m -f "{{.Version}}" github.com/gardener/external-dns-management)
dnsman_root=$(go list -m -f "{{.Dir}}" github.com/gardener/external-dns-management)

# collect CRDs from the dnsman module in the right order
crd_dir="${dnsman_root}/pkg/apis/dns/crds"
tmp_crds="${tmp_dir}/crds.yaml"

cat "${crd_dir}/dns.gardener.cloud_dnsentries.yaml" \
    "${crd_dir}/dns.gardener.cloud_dnsannotations.yaml" \
    "${crd_dir}/dns.gardener.cloud_dnsproviders.yaml" \
    "${crd_dir}/dns.gardener.cloud_dnshostedzonepolicies.yaml" \
    > "${tmp_crds}"

echo "Updating file 'example/20-crds.yaml'"
example_crds="${repo_root}/example/20-crds.yaml"
tmp_example_crds="${tmp_dir}/example-crds.yaml"
awk '/name: dnsentries.dns.gardener.cloud/ {exit} {a[NR]=$0} NR>6{print a[NR-6]}' "${example_crds}" > "${tmp_example_crds}"
cat "${tmp_crds}" >> "${tmp_example_crds}"
cp "${tmp_example_crds}" "${example_crds}"

echo "Updating file 'charts/gardener-extension-shoot-dns-service/templates/dnsman-crds.yaml'"
tmp_dnsman_crds="${tmp_dir}/dnsman-crds.yaml"
echo "{{- if and .Values.dnsControllerManager.deploy .Values.dnsControllerManager.createCRDs }}" > "${tmp_dnsman_crds}"
cat "${tmp_crds}" >> "${tmp_dnsman_crds}"
echo "{{- end }}" >> "${tmp_dnsman_crds}"
awk '
  found && $0 ~ /^ {4}controller-gen\.kubebuilder\.io\/version:/ {
    print
    print "    resources.gardener.cloud/keep-object: \"true\""
    print "  labels:"
    print "{{ include \"dnsmanLabels\" . | indent 4 }}"
    found=0
    next
  }
  $0 ~ /^  annotations:$/ {found=1}
  {print}
' "${tmp_dnsman_crds}" > charts/gardener-extension-shoot-dns-service/templates/dnsman-crds.yaml
