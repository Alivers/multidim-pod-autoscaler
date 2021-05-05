all: build

TAG?=dev
REGISTRY?=aliverjon
FLAGS=
TEST_ENVVAR=LD_FLAGS=-s GO111MODULE=on
ENVVAR=CGO_ENABLED=0 $(TEST_ENVVAR)
GOOS?=linux
COMPONENT=admission
FULL_COMPONENT=mpa-${COMPONENT}

ALL_ARCHITECTURES?=amd64
export DOCKER_CLI_EXPERIMENTAL=enabled

build: clean
	$(ENVVAR) GOOS=$(GOOS) go build ./...
	$(ENVVAR) GOOS=$(GOOS) go build -o ${COMPONENT}

build-binary: clean
	$(ENVVAR) GOOS=$(GOOS) go build -o ${COMPONENT}

.PHONY: build-binary-with-vendor
build-binary-with-vendor: $(addprefix build-binary-with-vendor-,$(ALL_ARCHITECTURES)) clean

.PHONY: build-binary-with-vendor-*
build-binary-with-vendor-%:
	$(ENVVAR) GOARCH=$* GOOS=$(GOOS) go build -mod vendor -o ${COMPONENT}-$*

.PHONY: docker-build
docker-build: $(addprefix docker-build-,$(ALL_ARCHITECTURES))

.PHONY: docker-build-*
docker-build-%:
ifndef REGISTRY
	ERR = $(error REGISTRY is undefined)
	$(ERR)
endif
ifndef TAG
	ERR = $(error TAG is undefined)
	$(ERR)
endif
	docker build --pull -t ${REGISTRY}/${FULL_COMPONENT}-$*:${TAG} --build-arg ARCH=$* .

.PHONY: docker-push
docker-push: $(addprefix sub-push-,$(ALL_ARCHITECTURES)) ;

.PHONY: sub-push-*
sub-push-%: docker-build-% do-push-% ;

.PHONY: do-push-*
do-push-%:
ifndef REGISTRY
	ERR = $(error REGISTRY is undefined)
	$(ERR)
endif
ifndef TAG
	ERR = $(error TAG is undefined)
	$(ERR)
endif
	docker push ${REGISTRY}/${FULL_COMPONENT}-$*:${TAG}

clean: $(addprefix clean-,$(ALL_ARCHITECTURES))

clean-%:
	rm -f ${COMPONENT}-$*

format:
	test -z "$$(find . -path ./vendor -prune -type f -o -name '*.go' -exec gofmt -s -d {} + | tee /dev/stderr)" || \
	test -z "$$(find . -path ./vendor -prune -type f -o -name '*.go' -exec gofmt -s -w {} + | tee /dev/stderr)"

.PHONY: all build clean format release
