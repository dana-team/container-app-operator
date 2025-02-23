# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General
# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-e2e
test-e2e: ginkgo
	@test -n "${KUBECONFIG}" -o -r ${HOME}/.kube/config || (echo "Failed to find kubeconfig in ~/.kube/config or no KUBECONFIG set"; exit 1)
	echo "Running e2e tests"
	go clean -testcache
	$(LOCALBIN)/ginkgo -p --vv ./test/e2e_tests/... -coverprofile cover.out -timeout

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go --ecs-logging=false

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply --server-side -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply --server-side -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Capp prerequisites
KNATIVE_VERSION ?= v1.17.0
KNATIVE_URL ?= https://github.com/knative-extensions/kn-plugin-quickstart/releases/download/knative-$(KNATIVE_VERSION)/kn-quickstart-linux-amd64
KNATIVE_HPA_URL ?= https://github.com/knative/serving/releases/download/knative-$(KNATIVE_VERSION)/serving-hpa.yaml
CROSSPLANE_SCC_CRB ?= hack/crossplane-scc-clusterrolebinding.yaml
PREREQ_HELMFILE ?= charts/capp-prereq-helmfile.yaml

.PHONY: prereq ## Install every prerequisite needed to develop and run the operator.
prereq: install install-knative enable-nfs-knative install-prereq-helmfile

.PHONY: prereq-openshift ## Install every prerequisite needed to develop and run the operator on OpenShift with Serverless already installed.
prereq-openshift: enable-nfs-knative install-crossplane-scc install-prereq-helmfile

.PHONY: uninstall-prereq ## Uninstall every prerequisite needed to develop and run the operator.
uninstall-prereq: uninstall-prereq-helmfile

.PHONY: uninstall-prereq-openshift  ## Uninstall every prerequisite needed to develop and run the operator.
uninstall-prereq-openshift: uninstall-prereq-helmfile uninstall-crossplane-scc

.PHONY: enable-nfs-knative
enable-nfs-knative: ## Enable NFS for Knative
	kubectl patch configmap config-features -n knative-serving -p '{"data":{"kubernetes.podspec-persistent-volume-claim":"enabled", "kubernetes.podspec-persistent-volume-write":"enabled"}}'

.PHONY: install-knative
install-knative: ## Install knative controller on the kind cluster
	wget -O $(LOCALBIN)/kn-quickstart $(KNATIVE_URL)
	chmod +x $(LOCALBIN)/kn-quickstart
	@CLUSTER_NAME=$$(kubectl config current-context | awk -F '-' '{ print $$2}'); \
	(yes no || true) | $(LOCALBIN)/kn-quickstart kind -n $$CLUSTER_NAME --install-serving
	$(KUBECTL) apply -f $(KNATIVE_HPA_URL)

.PHONY: install-crossplane-scc
install-crossplane-scc: ## Install crossplane rbac on the cluster
	kubectl apply -f $(CROSSPLANE_SCC_CRB)

CROSSPLANE_SCC_CRB ?= hack/crossplane-scc-clusterrolebinding.yaml
.PHONY: uninstall-crossplane-scc
uninstall-crossplane-scc: ## Uninstall crossplane rbac from the kind cluster
	kubectl delete -f $(CROSSPLANE_SCC_CRB)

.PHONY: install-prereq-helmfile
install-prereq-helmfile: helmfile helm helm-plugins
	${HELMFILE} apply -f $(PREREQ_HELMFILE) \
	--state-values-set providerDNSRealmName=${PROVIDER_DNS_REALM} \
	--state-values-set providerDNSKDCName=${PROVIDER_DNS_KDC} \
	--state-values-set providerDNSPolicy=${PROVIDER_DNS_POLICY} \
	--state-values-set providerDNSNameservers=${PROVIDER_DNS_NAMESERVER} \
	--state-values-set providerDNSUsername=${PROVIDER_DNS_USERNAME} \
	--state-values-set providerDNSPassword=${PROVIDER_DNS_PASSWORD}

.PHONY: uninstall-prereq-helmfile
uninstall-prereq-helmfile: helmfile helm helm-plugins
	${HELMFILE} destroy -f $(PREREQ_HELMFILE)

.PHONY: doc-chart
doc-chart: helm-docs helm
	$(HELM_DOCS) charts/

.PHONY: create-cappConfig
create-cappConfig: ## Run the setup-cappConfig-deployer script
	$(KUBECTL) apply -f $(shell pwd)/hack/manifests/cappconfig.yaml

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GINKGO ?= $(LOCALBIN)/ginkgo
HELMFILE ?= $(LOCALBIN)/helmfile-$(HELMFILE_VERSION)
HELM_DOCS ?= $(LOCALBIN)/helm-docs-$(HELM_DOCS_VERSION)

HELM_URL ?= https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
HELMFILE_URL ?= https://github.com/helmfile/helmfile/releases/download/v${HELMFILE_VERSION}/helmfile_${HELMFILE_VERSION}_linux_amd64.tar.gz

## Tool Versions
KUSTOMIZE_VERSION ?= v5.5.0
CONTROLLER_TOOLS_VERSION ?= v0.16.4
ENVTEST_VERSION ?= release-0.19
GOLANGCI_LINT_VERSION ?= v1.61.0
HELMFILE_VERSION ?= 0.167.1
HELM_DOCS_VERSION ?= v1.14.2

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download ginkgo locally if necessary.
$(GINKGO): $(LOCALBIN)
	test -s $(LOCALBIN)/ginkgo || GOBIN=$(LOCALBIN) go install github.com/onsi/ginkgo/v2/ginkgo@latest

.PHONY: helm
helm: ## Install helm on the local machine
	wget -O $(LOCALBIN)/get-helm.sh $(HELM_URL)
	chmod 700 $(LOCALBIN)/get-helm.sh
	$(LOCALBIN)/get-helm.sh

.PHONY: helm-plugins
helm-plugins: ## Install helm plugins on the local machine
	@if ! helm plugin list | grep -q 'diff'; then \
		helm plugin install https://github.com/databus23/helm-diff; \
	fi
	@if ! helm plugin list | grep -q 'git'; then \
		helm plugin install https://github.com/aslafy-z/helm-git; \
	fi
	@if ! helm plugin list | grep -q 's3'; then \
		helm plugin install https://github.com/hypnoglow/helm-s3; \
	fi
	@if ! helm plugin list | grep -q 'secrets'; then \
		helm plugin install https://github.com/jkroepke/helm-secrets; \
	fi

.PHONY: helmfile
helmfile: $(HELMFILE) ## Install helmfile on the local machine
$(HELMFILE): $(LOCALBIN)
	wget -O $(LOCALBIN)/helmfile.tar.gz $(HELMFILE_URL)
	tar -xzvf $(LOCALBIN)/helmfile.tar.gz -C $(LOCALBIN)
	rm $(LOCALBIN)/helmfile.tar.gz $(LOCALBIN)/*.md $(LOCALBIN)/LICENSE
	mv $(LOCALBIN)/helmfile $(LOCALBIN)/helmfile-$(HELMFILE_VERSION)

.PHONY: helm-docs
helm-docs: $(HELM_DOCS)
$(HELM_DOCS): $(LOCALBIN)
	$(call go-install-tool,$(HELM_DOCS),github.com/norwoodj/helm-docs/cmd/helm-docs,$(HELM_DOCS_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef