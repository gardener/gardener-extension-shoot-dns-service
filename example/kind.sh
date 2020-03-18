#!/bin/bash -e

match()
{
  if [ "${2#??-shoot-}" == "$2" ]; then
    [ -z "$1" ]
  else
    [ -n "$1" ]
  fi
}

deploy()
{
  for f in example/??-*.yaml; do
    echo "$f"
    if match "$1" "$f"; then
      echo kubectl "$@" apply -f "$f"
      kubectl "$@" apply -f "$f"
      if [ "${f%-crds.yaml}" != "$f" ]; then
        sleep 5
      fi
    fi
  done
}

case "$1" in
  create) ~/contrib/kind/bin/kind create cluster --name bibo ;;
  delete) ~/contrib/kind/bin/kind delete cluster --name bibo ;;
  deploy) deploy;;
  shoot) deploy --kubeconfig example/kubeconfig;;
  expose) kubectl -n shoot--foo--bar port-forward service/kube-apiserver 32223:443;;
  *) echo "invalid command $1" >&2
     exit 1;;
esac
