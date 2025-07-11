#!/bin/bash
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

repo_root="$(readlink -f $(dirname ${0})/..)"
version=$(cat "${repo_root}/VERSION")

cat << EOF > "${repo_root}/example/shoot-dns-service/example/extension-patch.yaml"
# DO NOT EDIT THIS FILE!
# This file is auto-generated by hack/prepare-operator-extension.sh.

apiVersion: operator.gardener.cloud/v1alpha1
kind: Extension
metadata:
  name: extension-shoot-dns-service
spec:
  deployment:
    admission:
      runtimeCluster:
        helm:
          ociRepository:
            ref: europe-docker.pkg.dev/gardener-project/releases/charts/gardener/extensions/admission-shoot-dns-service-runtime:$version
      virtualCluster:
        helm:
          ociRepository:
            ref: europe-docker.pkg.dev/gardener-project/releases/charts/gardener/extensions/admission-shoot-dns-service-application:$version
    extension:
      helm:
        ociRepository:
          ref: europe-docker.pkg.dev/gardener-project/releases/charts/gardener/extensions/shoot-dns-service:$version
EOF

kubectl kustomize "${repo_root}/example/shoot-dns-service/example" -o "${repo_root}/example/extension-shoot-dns-service.yaml"
