#!make
#
# Copyright (C) 2023 Intel Corporation
# SPDX-License-Identifier: MIT
#
TARBALL := svr-info.tgz

default: dist
.PHONY: clean default dist dist-amd64 test tools

tools:
	cd src && make

dist-amd64: tools
	rm -rf dist/svr-info
	mkdir -p dist/svr-info
	cp LICENSE dist/svr-info
	cp README dist/svr-info
	cp RELEASE_NOTES dist/svr-info
	cp THIRD_PARTY_PROGRAMS dist/svr-info
	cp docs/guide/SvrInfoUserGuide.pdf dist/svr-info/USER_GUIDE.pdf
	cp src/orchestrator/orchestrator dist/svr-info/svr-info
	mkdir -p dist/svr-info/tools
	cp src/orchestrator/resources/* dist/svr-info/tools
	cp src/pmu2metrics/pmu2metrics-with-perf dist/svr-info/tools
	cd dist/svr-info/tools && tar -xf collector_deps_amd64.tgz && rm collector_deps_*.tgz
	cd dist && tar -czf $(TARBALL) svr-info
	cd dist && md5sum $(TARBALL) > $(TARBALL).md5
	rm -rf dist/svr-info

dist: dist-amd64

oss:
	cd src && make oss-source
	mv src/oss_source* dist/

clean:
	cd src && make clean
	rm -rf dist

test:
	cd src && make test
	rm -rf test/svr-info
	cd test && tar -xf ../dist/$(TARBALL)
	cd test && ./functional
	cd test && ./fuzz
	rm -rf test/svr-info
