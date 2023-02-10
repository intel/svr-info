#!make
#
# Copyright (C) 2023 Intel Corporation
# SPDX-License-Identifier: MIT
#
COMMIT_ID := $(shell git rev-parse --short=8 HEAD)
COMMIT_DATE := $(shell git show -s --format=%cd --date=short HEAD)
VERSION_FILE := version.txt
VERSION_NUMBER := $(shell cat ${VERSION_FILE})
VERSION := $(VERSION_NUMBER)_$(COMMIT_DATE)_$(COMMIT_ID)
TARBALL := svr-info-$(VERSION_NUMBER).tgz

default: apps collector-deps dist-amd64
.PHONY: default clean test apps tools dist dist-amd64 collector-deps collector-deps-amd64 collector-deps-arm64

apps:
	cd src && VERSION=$(VERSION) make apps

tools:
	cd src && VERSION=$(VERSION) make tools

collector-deps-amd64:
	$(eval TMPDIR := $(shell mktemp -d build.XXXXXX))
	cp src/calcfreq/calcfreq $(TMPDIR)
	cp src/cpuid/cpuid $(TMPDIR)
	cp src/dmidecode/dmidecode $(TMPDIR)
	cp src/ethtool/ethtool $(TMPDIR)
	cp src/fio/fio $(TMPDIR)
	cp src/ipmitool/src/ipmitool.static $(TMPDIR)/ipmitool
	cp src/lshw/src/lshw-static $(TMPDIR)/lshw
	-cp src/mlc/mlc $(TMPDIR)
	cp src/msrbusy/msrbusy $(TMPDIR)
	cp src/linux/tools/perf/perf $(TMPDIR)
	cp -R src/async-profiler $(TMPDIR)
	cp src/flamegraph/stackcollapse-perf.pl $(TMPDIR)
	cp src/rdmsr/rdmsr $(TMPDIR)
	cp src/spectre-meltdown-checker/spectre-meltdown-checker.sh $(TMPDIR)
	cp src/stress-ng/stress-ng $(TMPDIR)
	cp src/sysstat/mpstat $(TMPDIR)
	cp src/sysstat/iostat $(TMPDIR)
	cp src/sysstat/sar $(TMPDIR)
	cp src/sysstat/sadc $(TMPDIR)
	cp src/linux/tools/power/x86/turbostat/turbostat $(TMPDIR)
	-cp -r bin/* $(TMPDIR)
	for f in $(TMPDIR)/*; do strip -s -p --strip-unneeded $$f; done
	cd $(TMPDIR) && tar -czf ../config/collector_deps_amd64.tgz .
	rm -rf $(TMPDIR)

collector-deps-arm64:
	$(eval TMPDIR := $(shell mktemp -d build.XXXXXX))
	cp src/spectre-meltdown-checker/spectre-meltdown-checker.sh $(TMPDIR)
	cd $(TMPDIR) && tar -czf ../config/collector_deps_arm64.tgz .
	rm -rf $(TMPDIR)

collector-deps: collector-deps-amd64 collector-deps-arm64

dist-amd64:
	rm -rf dist/svr-info
	mkdir -p dist/svr-info/tools
	cp src/orchestrator/orchestrator dist/svr-info/tools
	cp src/collector/collector dist/svr-info/tools
	cp src/collector/collector_arm64 dist/svr-info/tools
	cp src/reporter/reporter dist/svr-info/tools
	cp src/sshpass/sshpass dist/svr-info/tools
	cp src/burn/burn dist/svr-info/tools
	mkdir -p dist/svr-info/config/extras
	rsync config/* dist/svr-info/config
	cp LICENSE dist/svr-info
	cp README dist/svr-info
	cp RELEASE_NOTES dist/svr-info
	cp third-party-programs.txt dist/svr-info
	cp src/orchestrator/targets.example dist/svr-info
	cp documentation/ServerInfoUserGuide.pdf dist/svr-info/UserGuide.pdf
	cd dist/svr-info && ln -s tools/orchestrator ./svr-info
	cd dist/svr-info && find . -type f -exec md5sum {} + > config/sums.md5
	cd dist && tar -czf $(TARBALL) svr-info
	cd dist && md5sum $(TARBALL) > $(TARBALL).md5
	cp dist/svr-info/config/sums.md5 config/sums.md5
	rm -rf dist/svr-info/

dist: apps tools collector-deps dist-amd64 oss

oss:
	cd src && make oss-source
	mv src/oss_source* dist/

clean:
	cd src && make clean
	rm -rf dist

test:
	rm -rf test/svr-info
	cd test && tar -xf ../dist/$(TARBALL)
	cd test && ./functional
	rm -rf test/svr-info
