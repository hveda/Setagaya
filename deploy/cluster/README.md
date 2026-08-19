# Cluster prerequisites — talos-homelab

The Honryu chart (phase 16, `deploy/charts/honryu`) installs **only** the
`honryu` namespace and its workloads. Everything on this page is cluster
infrastructure the chart expects to already exist (spec Non-goals: "The chart
does not install cluster infrastructure"). Three of the four were installed
by hand before this phase; their versions below are **captured from the live
cluster** (`kubectl --context admin@talos-homelab`), not guessed.

Cluster: single control-plane node `talos-cp`, Talos v1.13.7, Kubernetes
v1.31.4, containerd 2.2.6. Registry: `registry.pve.heri.life` (pull secret
`registry-pve-heri-life` in the honryu namespace, pre-existing).

## Prerequisites (as-found)

| Component | Version (live) | Namespace | Notes |
|---|---|---|---|
| MetalLB | controller `quay.io/metallb/controller:v0.14.9` (speaker same tag) | `metallb-system` | IPAddressPool `homelab-pool` = `10.10.10.90-10.10.10.95`; the chart's Ingress address is `10.10.10.90` |
| ingress-nginx | `registry.k8s.io/ingress-nginx/controller:v1.12.1@sha256:d2fbc4…eed5b` (DaemonSet) | `ingress-nginx` | ingressClass `nginx` (default); L2 service via MetalLB |
| Karpenter + Proxmox provider | `ghcr.io/sergelogvinov/karpenter-provider-proxmox:v0.12.0` | `kube-system` | CRDs `karpenter.proxmox.sinextra.dev`; NodePool `mi666-1` (NodeClass `mi666-1`, limits cpu 6 / mem 12Gi, requirements arch=amd64, zone=mi666-1); also present: `proxmox-cloud-controller-manager` (Proxmox CCM, same ecosystem) |
| local-path-provisioner | `rancher/local-path-provisioner:v0.0.31` | `local-path-storage` | **installed by phase 16 task 159** — this repo's pinned manifest below; StorageClass `local-path` is the cluster's **default** |

Reinstall commands, pinned:

```sh
# MetalLB (v0.14.9)
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.9/config/manifests/metallb-native.yaml
# then apply the IPAddressPool/L2Advertisement (pool homelab-pool,
# 10.10.10.90-10.10.10.95) -- kept out of this repo: it encodes the LAN's
# address plan, document yours next to your MetalLB install.

# ingress-nginx (v1.12.1, DaemonSet flavor as installed)
helm upgrade --install ingress-nginx ingress-nginx \
  --repo https://kubernetes.github.io/ingress-nginx ingress-nginx \
  --version 4.14.1 --namespace ingress-nginx --create-namespace \
  --set controller.kind=DaemonSet

# Karpenter Proxmox provider (v0.12.0) -- follow the upstream chart; the
# NodePool/NodeClass (mi666-1) and Proxmox credentials are environment
# specifics, created out of band:
#   https://karpenter-proxmox.sinextra.dev/
# NodePool as-found: limits cpu=6,memory=12Gi; requirements
# kubernetes.io/arch In [amd64], topology.kubernetes.io/zone In [mi666-1].

# local-path-provisioner (v0.0.31) -- pinned manifest in this repo,
# StorageClass annotated as the cluster default:
kubectl apply -f deploy/cluster/local-path-provisioner.yaml
```

## Why local-path (and its limits)

Single control-plane node today; Karpenter adds *nodes* (Proxmox VMs) for
engine pods, which come and go. local-path storage is therefore **node-local
and safe only for platform components pinned to the control plane** (MySQL,
see the chart's PVCs). Anything that must survive node churn belongs on
storage provisioned per the chart's explicit StorageClass choices — the chart
must not rely on the default silently.
