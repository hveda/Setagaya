all: | cluster permissions db prometheus grafana setagaya jmeter local_storage ingress-controller

# Container runtime - can be docker or podman
CONTAINER_RUNTIME ?= $(shell which podman >/dev/null 2>&1 && echo podman || echo docker)

setagaya-controller-ns = setagaya-executors
setagaya-executor-ns = setagaya-executors

.PHONY: cluster
cluster:
	-kind create cluster --name setagaya --wait 180s
	-kubectl create namespace $(setagaya-controller-ns)
	kubectl apply -f kubernetes/metricServer.yaml
	kubectl config set-context --current --namespace=$(setagaya-controller-ns)
	touch setagaya/setagaya-gcp.json

.PHONY: clean
clean:
	kind delete cluster --name setagaya
	-killall kubectl

.PHONY: prometheus
prometheus:
	kubectl -n $(setagaya-controller-ns) replace -f kubernetes/prometheus.yaml --force

.PHONY: db
db: setagaya/db kubernetes/db.yaml
	-kubectl -n $(setagaya-controller-ns) delete configmap database
	kubectl -n $(setagaya-controller-ns) create configmap database --from-file=setagaya/db/
	kubectl -n $(setagaya-controller-ns) replace -f kubernetes/db.yaml --force

.PHONY: grafana
grafana: grafana/
	helm uninstall metrics-dashboard || true
	$(CONTAINER_RUNTIME) build grafana/ -t grafana:local
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/grafana:local -o /tmp/grafana-local.tar
	kind load image-archive /tmp/grafana-local.tar --name setagaya
	rm -f /tmp/grafana-local.tar
else
	kind load docker-image grafana:local --name setagaya
endif
	helm upgrade --install metrics-dashboard grafana/metrics-dashboard

.PHONY: local_api
local_api:
	$(CONTAINER_RUNTIME) build -f setagaya/Dockerfile.api -t api:local setagaya
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/api:local -o /tmp/api-local.tar
	kind load image-archive /tmp/api-local.tar --name setagaya
	rm -f /tmp/api-local.tar
else
	kind load docker-image api:local --name setagaya
endif

.PHONY: local_controller
local_controller:
	$(CONTAINER_RUNTIME) build -f setagaya/Dockerfile.controller -t controller:local setagaya
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/controller:local -o /tmp/controller-local.tar
	kind load image-archive /tmp/controller-local.tar --name setagaya
	rm -f /tmp/controller-local.tar
else
	kind load docker-image controller:local --name setagaya
endif

.PHONY: setagaya
setagaya: local_api local_controller grafana
	helm uninstall setagaya || true
	cd setagaya && helm upgrade --install setagaya install/setagaya

.PHONY: jmeter
jmeter: setagaya/engines/jmeter
	cd setagaya && sh build.sh jmeter
	$(CONTAINER_RUNTIME) build -t setagaya:jmeter -f setagaya/Dockerfile.engines.jmeter setagaya
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/setagaya:jmeter -o /tmp/setagaya-jmeter.tar
	kind load image-archive /tmp/setagaya-jmeter.tar --name setagaya
	rm -f /tmp/setagaya-jmeter.tar
else
	kind load docker-image setagaya:jmeter --name setagaya
endif

.PHONY: expose
expose:
	-killall kubectl
	-kubectl -n $(setagaya-controller-ns) port-forward service/setagaya-metrics-dashboard 3000:3000 > /dev/null 2>&1 &
	-kubectl -n $(setagaya-controller-ns) port-forward service/setagaya-api-local 8080:8080 > /dev/null 2>&1 &

# TODO!
# After k8s 1.22, service account token is no longer auto generated. We need to manually create the secret
# for the service account. ref: "https://kubernetes.io/docs/reference/access-authn-authz/service-accounts-admin/#manual-secret-management-for-serviceaccounts"
# So we should fetch the token details from the manually created secret instead of the automatically created ones
.PHONY: kubeconfig
kubeconfig:
	./kubernetes/generate_kubeconfig.sh $(setagaya-controller-ns)

.PHONY: permissions
permissions:
	kubectl -n $(setagaya-executor-ns) apply -f kubernetes/roles.yaml
	kubectl -n $(setagaya-controller-ns) apply -f kubernetes/serviceaccount.yaml
	kubectl -n $(setagaya-controller-ns) apply -f kubernetes/service-account-secret.yaml
	-kubectl -n $(setagaya-executor-ns) create rolebinding setagaya --role=setagaya --serviceaccount $(setagaya-controller-ns):setagaya
	kubectl -n $(setagaya-executor-ns) replace -f kubernetes/ingress.yaml --force

.PHONY: permissions-gcp
permissions-gcp: node-permissions permissions

.PHONY: node-permissions
node-permissions:
	kubectl apply -f kubernetes/clusterrole.yaml
	-kubectl create clusterrolebinding setagaya --clusterrole=setagaya --serviceaccount $(setagaya-controller-ns):setagaya
	kubectl apply -f kubernetes/pdb.yaml

.PHONY: local_storage
local_storage:
	$(CONTAINER_RUNTIME) build -t setagaya:storage local_storage
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/setagaya:storage -o /tmp/setagaya-storage.tar
	kind load image-archive /tmp/setagaya-storage.tar --name setagaya
	rm -f /tmp/setagaya-storage.tar
else
	kind load docker-image setagaya:storage --name setagaya
endif
	kubectl -n $(setagaya-controller-ns) apply -f kubernetes/storage.yaml

.PHONY: ingress-controller
ingress-controller:
	# if you need to debug the controller, please use the makefile in the ingress controller folder
	# And update the image in the config.json
	$(CONTAINER_RUNTIME) build -t setagaya:ingress-controller -f ingress-controller/Dockerfile ingress-controller
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/setagaya:ingress-controller -o /tmp/setagaya-ingress-controller.tar
	kind load image-archive /tmp/setagaya-ingress-controller.tar --name setagaya
	rm -f /tmp/setagaya-ingress-controller.tar
else
	kind load docker-image setagaya:ingress-controller --name setagaya
endif
