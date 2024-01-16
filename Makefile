#!make
#
# Copyright (C) 2023 Intel Corporation
# SPDX-License-Identifier: MIT
#
COMMIT_ID := $(shell git rev-parse --short=8 HEAD)
COMMIT_DATE := $(shell git show -s --format=%cd --date=short HEAD)
VERSION_FILE := ./version.txt
VERSION_NUMBER := $(shell cat ${VERSION_FILE})
VERSION := $(VERSION_NUMBER)_$(COMMIT_DATE)_$(COMMIT_ID)

TARBALL := svr-info.tgz

default: dist
.PHONY: clean default dist dist-amd64 test tools

bin:
	mkdir -p bin

orchestrator: bin reporter collector collector-deps
	cp /prebuilt/bin/sshpass cmd/orchestrator/resources/
	cp bin/reporter cmd/orchestrator/resources/
	cp bin/collector cmd/orchestrator/resources/
	cp bin/collector_arm64 cmd/orchestrator/resources/
	cd bin && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o orchestrator ../cmd/orchestrator

collector: bin
	cd bin && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o collector ../cmd/collector
	cd bin && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o collector_arm64 ../cmd/collector

reporter: bin
	cd bin && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o reporter ../cmd/reporter

msrread: bin
	cd bin && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o msrread ../cmd/msrread

msrwrite: bin
	cd bin && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o msrwrite ../cmd/msrread

msrbusy: bin
	cd bin && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o msrbusy ../cmd/msrbusy

pmu2metrics: bin
	-cp /prebuilt/bin/perf cmd/pmu2metrics/resources
	cd bin && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o pmu2metrics ../cmd/pmu2metrics

calcfreq: bin
	cd bin && gcc -D VERSION=\"$(VERSION)\" ../tools/calcfreq/calcfreq.c -lpthread -o calcfreq -static

collector-deps-amd64: third_party calcfreq msrbusy msrread msrwrite pmu2metrics
	$(eval TMPDIR := $(shell mktemp -d build.XXXXXX))
	cp -R /prebuilt/bin/* $(TMPDIR)
	cp bin/calcfreq $(TMPDIR)
	cp bin/msrbusy $(TMPDIR)
	cp bin/msrread $(TMPDIR)
	cp bin/msrwrite $(TMPDIR)
	cp bin/pmu2metrics $(TMPDIR)
	cd $(TMPDIR) && tar -czf ../cmd/orchestrator/resources/collector_deps_amd64.tgz .
	rm -rf $(TMPDIR)

collector-deps-arm64: third_party
	$(eval TMPDIR := $(shell mktemp -d build.XXXXXX))
	cp /prebuilt/bin/spectre-meltdown-checker.sh $(TMPDIR)
	cd $(TMPDIR) && tar -czf ../cmd/orchestrator/resources/collector_deps_arm64.tgz .
	rm -rf $(TMPDIR)

collector-deps: collector-deps-amd64 collector-deps-arm64

third_party:
	cd third_party && make

dist-amd64: orchestrator
	rm -rf dist/svr-info
	mkdir -p dist/svr-info
	cp LICENSE dist/svr-info
	cp README dist/svr-info
	cp RELEASE_NOTES dist/svr-info
	cp THIRD_PARTY_PROGRAMS dist/svr-info
	cp docs/guide/SvrInfoUserGuide.pdf dist/svr-info/USER_GUIDE.pdf
	cp bin/orchestrator dist/svr-info/svr-info
	mkdir -p dist/svr-info/tools
	cp -R /prebuilt/bin/* dist/svr-info/tools
	cp bin/* dist/svr-info/tools
	rm dist/svr-info/tools/orchestrator
	cd dist && tar -czf $(TARBALL) svr-info
	cd dist && md5sum $(TARBALL) > $(TARBALL).md5
	rm -rf dist/svr-info

dist: dist-amd64 third_party
	cp /prebuilt/oss_source.* dist

clean:
	rm -rf dist
	rm -rf bin

test:
	# test packages
	cd internal/commandfile && go test -v -vet=all .
	cd internal/core && go test -v -vet=all .
	cd internal/cpu && go test -v -vet=all .
	# these tests require access to MSRs which we don't have on WSL2 and may not have on build machine 
	# cd internal/msr && go test -v -vet=all .
	cd internal/progress && go test -v -vet=all .
	cd internal/target && go test -v -vet=all .
	
	# test apps
	go test -v -vet=all ./cmd/orchestrator
	go test -v -vet=all ./cmd/collector
	go test -v -vet=all ./cmd/reporter
	go test -v -vet=all ./cmd/msrread
	go test -v -vet=all ./cmd/msrwrite
	go test -v -vet=all ./cmd/msrbusy
	go test -v -vet=all ./cmd/pmu2metrics
	
	# test svr-info
	rm -rf test/svr-info
	cd test && tar -xf ../dist/$(TARBALL)
	cd test && ./functional
	cd test && ./fuzz
	rm -rf test/svr-info

format_check:
	@echo "Running gofmt -l to check for code formatting issues..."
	@test -z $(shell gofmt -l -s internal/commandfile/ internal/core/ internal/cpu/ internal/progress/ internal/target/ cmd/orchestrator/ cmd/collector/ cmd/reporter/ cmd/pmu2metrics/ cmd/msrread/ cmd/msrwrite/) || { echo "[WARN] Formatting issues detected. Resolve with 'make format'"; exit 1; }
	@echo "gofmt detected no issues"

check: format_check

format:
	gofmt -l -w -s internal/commandfile/ internal/core/ internal/cpu/ internal/progress/ internal/target/ orchestrator/ collector/ reporter/ pmu2metrics/ rdmsr/ wrmsr/

