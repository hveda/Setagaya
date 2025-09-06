#!/bin/bash
kubeconfig="apiVersion: v1
kind: Config
users:
- name: setagaya-user
  user:
    token: TOKEN_HERE
clusters:
- cluster:
    certificate-authority-data: CA_HERE
    server: SERVER_HERE
  name: setagaya-cluster
contexts:
- context:
    cluster: setagaya-cluster
    user: setagaya-user
  name: setagaya-context
current-context: setagaya-context"

# check for namespace
if [ "$1" == "" ]; then
    echo "Provide namespace where setagaya service account exists"
    exit 1
fi

# get token from secret
TOKEN=$(kubectl -n $1 get secrets $(kubectl -n $1 get sa setagaya -o=custom-columns=:."secrets[0].name") -o=custom-columns=:.data.token)
TOKEN=$(echo $TOKEN | base64 -d)
kubeconfig=$(echo "$kubeconfig" | sed 's,TOKEN_HERE,'"$TOKEN"',g')

# get ca.crt from secret
CAcrt=$(kubectl -n $1 get secrets $(kubectl -n $1 get sa setagaya -o=custom-columns=:."secrets[0].name") -o=custom-columns=:.data."ca\.crt" | tr -d '\n')
kubeconfig=$(echo "$kubeconfig" | sed 's,CA_HERE,'"$CAcrt"',g')

# get API server master url
SERVER=$(TERM=dumb kubectl cluster-info | grep "Kubernetes master" | awk '{print $NF}')
kubeconfig=$(echo "$kubeconfig" | sed 's,SERVER_HERE,'"$SERVER"',g')

# export kubeconfig to setagaya/config/kube_configs using context name
FILEPATH="setagaya/config/kube_configs/$(kubectl config current-context)"
echo "$kubeconfig" > "$FILEPATH"