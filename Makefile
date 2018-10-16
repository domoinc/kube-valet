VERSION ?= $(shell echo $${BRANCH_NAME:-local} | sed s/[^a-zA-Z0-9_-]/_/)_$(shell git describe --always --dirty)
IMAGE ?= domoinc/kube-valet

.PHONY: all kube-valet valetctl test release

all: install-deps customresources build

build: kube-valet valetctl

install-deps:
	glide i

kube-valet:
	mkdir build || true
	CGO_ENABLED=0 GOOS=linux go build -v -i -pkgdir $(PWD)/build/gopkgs --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo -o build/kube-valet

valetctl:
	mkdir build || true
	CGO_ENABLED=0 GOOS=linux go build -v -i -pkgdir $(PWD)/build/gopkgs --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo -o build/valetctl bin/valetctl.go

clean:
	rm build/* || true

test: test-customresources test-pkgs

test-pkgs:
	# client-go is huge, install deps so future tests are faster
	go test -i ./pkg/...

	# run tests
	go test -v ./pkg/...

docker-image:
	docker build -t $(IMAGE):$(VERSION) .

release: docker-image
	# Push the versioned tag
	docker push $(IMAGE):$(VERSION)

	# Push the latest tag
	docker tag $(IMAGE):$(VERSION) $(IMAGE):latest
	docker push $(IMAGE):latest

# Targets to build custom resources and clients

customresources: clean-customresources gen-customresources test-customresources

gen-customresources: clean-customresources
	grep "openapi-gen" ./vendor/k8s.io/code-generator/generate-groups.sh || cat ./_openapi/openapi-gen.sh >> ./vendor/k8s.io/code-generator/generate-groups.sh \

	mkdir -p ./pkg/client/openapi

	echo "package openapi" > ./pkg/client/openapi/doc.go
	cp ./_openapi/path_template.tmpl ./pkg/client/openapi
	cp ./_openapi/print_test.go ./pkg/client/openapi

	./vendor/k8s.io/code-generator/generate-groups.sh all \
	github.com/domoinc/kube-valet/pkg/client \
	github.com/domoinc/kube-valet/pkg/apis \
	"assignments:v1alpha1"

	go test ./pkg/client/openapi/*.go -test.run=TestWriteOpenAPISpec

	rm ./pkg/client/openapi/path_template.tmpl
	rm ./pkg/client/openapi/print_test.go

	# workaround https://github.com/openshift/origin/issues/10357
	find pkg/client -name "clientset_generated.go" -exec sed -i'' 's/return \\&Clientset{fakePtr/return \\&Clientset{\\&fakePtr/g' '{}' \;

clean-customresources:
	# Delete all generated code.
	rm -rf pkg/client
	rm -f pkg/apis/*/*/zz_generated.deepcopy.go

# This is a basic smoke-test to make sure the types compile
test-customresources:
	go build -o build/crud -i _examples/clients/crud.go
	go build -o build/list -i _examples/clients/list.go

	@echo "All custom resource client test binaries compiled!"

