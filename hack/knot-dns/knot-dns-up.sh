#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

repo_root="$(readlink -f $(dirname ${0})/../..)"
export KUBECONFIG=$repo_root/gardener/dev-setup/gardenlet/components/kubeconfigs/seed-local/kubeconfig

PASSWORD="123456"
PASSWORD_BASE64=$(echo -n $PASSWORD | base64 -w0)

wait_for_service()
{
  timeout=15

  echo looking up IP address for knot-dns service
  for ((i=0; i<$timeout; i++)); do
    set +e
    SERVICE_IP_ADDRESS=$(kubectl get svc knot-dns -n knot-dns -ojsonpath='{.spec.clusterIP}' 2> /dev/null)
    set -e
    if [ -n "$SERVICE_IP_ADDRESS" ]; then
      echo
      echo "knot-dns service IP address: $SERVICE_IP_ADDRESS"
      return 0
    fi
    echo -n .
    sleep 1
  done
  echo failed
  return 1
}

kubectl apply -f $(dirname ${0})/knot-dns-service.yaml
wait_for_service

sed -e "s/#secret-injection/$PASSWORD_BASE64/g" $(dirname ${0})/knot-dns.yaml.template | \
  sed -e "s/#server-injection/$SERVICE_IP_ADDRESS/g" | \
  kubectl apply -f -
