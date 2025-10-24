#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

repo_root="$(readlink -f $(dirname ${0})/../..)"
export KUBECONFIG=$repo_root/gardener/dev-setup/gardenlet/components/kubeconfigs/seed-local/kubeconfig

kubectl delete ns knot-dns
