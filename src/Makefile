#!make
#
# Copyright (C) 2023 Intel Corporation
# SPDX-License-Identifier: MIT
#
COMMIT_ID := $(shell git rev-parse --short=8 HEAD)
COMMIT_DATE := $(shell git show -s --format=%cd --date=short HEAD)
VERSION_FILE := ../version.txt
VERSION_NUMBER := $(shell cat ${VERSION_FILE})
VERSION := $(VERSION_NUMBER)_$(COMMIT_DATE)_$(COMMIT_ID)

default: check orchestrator
.PHONY: default test check format_check format linux-source collector reporter orchestrator pmu2metrics rdmsr wrmsr msrbusy reset clean tools libs async-profiler calcfreq cpuid dmidecode ethtool fio flamegraph ipmitool lshw lspci mlc perf spectre-meltdown-checker sshpass stress-ng sysstat turbostat oss-source

prebuilt_tools: async-profiler cpuid dmidecode ethtool fio flamegraph ipmitool lshw lspci perf spectre-meltdown-checker sshpass stress-ng sysstat turbostat
other_tools: calcfreq mlc msrbusy pmu2metrics pmu2metrics-with-perf rdmsr wrmsr

test:
	cd pkg/commandfile && go test -v -vet=all .
	cd pkg/core && go test -v -vet=all .
	cd pkg/cpu && go test -v -vet=all .
	# these tests require access to MSRs which we don't have on WSL2 and may not have on build machine -- cd pkg/msr && go test -v -vet=all .
	cd pkg/progress && go test -v -vet=all .
	cd pkg/target && go test -v -vet=all .
	cd orchestrator && go test -v -vet=all .
	cd collector && go test -v -vet=all .
	cd reporter && go test -v -vet=all .
	cd rdmsr && go test -v -vet=all .
	cd wrmsr && go test -v -vet=all .
	cd msrbusy && go test -v -vet=all .
	cd pmu2metrics && go test -v -vet=all .

format:
	gofmt -l -w -s pkg/commandfile/ pkg/core/ pkg/cpu/ pkg/progress/ pkg/target/ orchestrator/ collector/ reporter/ pmu2metrics/ rdmsr/ wrmsr/

lint:
	golint -set_exit_status pkg/commandfile/ pkg/core/ pkg/cpu/ pkg/progress/ pkg/target/ orchestrator/ collector/ reporter/ pmu2metrics/ rdmsr/ wrmsr/

format_check:
	@echo "Running gofmt -l to check for code formatting issues..."
	@test -z $(shell gofmt -l -s pkg/commandfile/ pkg/core/ pkg/cpu/ pkg/progress/ pkg/target/ orchestrator/ collector/ reporter/ pmu2metrics/ rdmsr/ wrmsr/) || { echo "[WARN] Formatting issues detected. Resolve with 'make format'"; exit 1; }
	@echo "gofmt detected no issues"

check: format_check

orchestrator: reporter collector collector-deps
	cp /prebuilt/sshpass/sshpass orchestrator/resources/
	cp reporter/reporter orchestrator/resources/
	cp collector/collector orchestrator/resources/
	cp collector/collector_arm64 orchestrator/resources/
	cd orchestrator && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o orchestrator

collector:
	cd collector && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o collector
	cd collector && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o collector_arm64

reporter:
	cd reporter && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o reporter

rdmsr:
	cd rdmsr && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o rdmsrx

wrmsr:
	cd wrmsr && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o wrmsrx

msrbusy:
	cd msrbusy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o msrbusy

pmu2metrics:
	rm -f pmu2metrics/resources/perf
	cd pmu2metrics && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o pmu2metrics

pmu2metrics-with-perf:
	cp /prebuilt/linux/tools/perf/perf pmu2metrics/resources
	cd pmu2metrics && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '-s -w -X main.gVersion=$(VERSION)' -o pmu2metrics-with-perf
	rm pmu2metrics/resources/perf

ASYNCPROFILER_VERSION := 2.9
async-profiler:
ifeq ("$(wildcard async-profiler)","")
ifeq ("$(wildcard async-profiler-$(ASYNCPROFILER_VERSION)-linux-x64.tar.gz)","")
	wget https://github.com/jvm-profiling-tools/async-profiler/releases/download/v$(ASYNCPROFILER_VERSION)/async-profiler-$(ASYNCPROFILER_VERSION)-linux-x64.tar.gz
endif
	tar -xf async-profiler-$(ASYNCPROFILER_VERSION)-linux-x64.tar.gz && mv async-profiler-$(ASYNCPROFILER_VERSION)-linux-x64 async-profiler
endif

calcfreq:
	cd calcfreq && VERSION=$(VERSION) make

CPUID_VERSION := 20230614
cpuid:
ifeq ("$(wildcard cpuid)","")
ifeq ("$(wildcard cpuid-$(CPUID_VERSION).src.tar.gz)","")
	wget http://www.etallen.com/cpuid/cpuid-$(CPUID_VERSION).src.tar.gz
endif	
	tar -xf cpuid-$(CPUID_VERSION).src.tar.gz && mv cpuid-$(CPUID_VERSION)/ cpuid/
endif	
	# gcc 4.8 doesn't support -Wimplicit-fallthrough option
	cd cpuid && sed -i s/"-Wimplicit-fallthrough"/""/ Makefile
	cd cpuid && make

dmidecode:
ifeq ("$(wildcard dmidecode)","")
	git clone https://github.com/mirror/dmidecode.git
else
	cd dmidecode && git checkout master && git pull
endif
	cd dmidecode && git checkout dmidecode-3-5
	cd dmidecode && make

ethtool:
ifeq ("$(wildcard ethtool)","")
	git clone https://git.kernel.org/pub/scm/network/ethtool/ethtool.git
else
	cd ethtool && git checkout master && git pull
endif
	cd ethtool && git checkout v5.15
ifeq ("$(wildcard ethtool/Makefile)","")
	cd ethtool && ./autogen.sh && ./configure enable_netlink=no
endif
	cd ethtool && make

fio:
ifeq ("$(wildcard fio)","")
	git clone https://github.com/axboe/fio.git
else
	cd fio && git checkout master && git pull
endif
	cd fio && git checkout fio-3.28
ifeq ("$(wildcard fio/config.log)","")
	cd fio && ./configure --build-static --disable-native
endif
	cd fio && make

flamegraph:
ifeq ("$(wildcard flamegraph)","")
	git clone https://github.com/brendangregg/FlameGraph.git flamegraph
	# small modification to script to include module name in output
	cd flamegraph && sed -i '382 a \\t\t\t\t$$func = \$$func."'" "'".\$$mod;\t# add module name' stackcollapse-perf.pl
endif

ipmitool:
ifeq ("$(wildcard ipmitool)","")
	git clone https://github.com/ipmitool/ipmitool.git
endif
	cd ipmitool && git checkout IPMITOOL_1_8_19
ifeq ("$(wildcard ipmitool/Makefile)","")
	# hack to get around static build problem - don't check for libreadline
	sed -i "s#x\$$xenable_ipmishell#xno#" ipmitool/configure.ac
	cd ipmitool && ./bootstrap && LDFLAGS=-static ./configure
endif
	cd ipmitool && make
	cd ipmitool/src && ../libtool --silent --tag=CC --mode=link gcc -fno-strict-aliasing -Wreturn-type -all-static -o ipmitool.static ipmitool.o ipmishell.o ../lib/libipmitool.la plugins/libintf.la

lshw:
ifeq ("$(wildcard lshw)","")
	git clone https://github.com/lyonel/lshw.git
else
	cd lshw && git checkout master && git pull
endif
	cd lshw && git checkout B.02.19
	cd lshw/src && make static

lspci:
ifeq ("$(wildcard lspci)","")
	git clone https://github.com/pciutils/pciutils.git lspci
else
	cd lspci && git checkout master && git pull
endif
	cd lspci && make
	cd lspci && ./update-pciids.sh
	cd lspci && gzip -c pci.ids > pci.ids.gz

mlc:
ifeq ("$(wildcard mlc)","")
	-if [ ${CI} = true ]; then\
		git clone https://${CELINE_ACCESS_TOKEN}@github.com/intel-innersource/applications.benchmarking.cpu-micros.mlc.git mlc; \
	else\
		git clone git@github.com:intel-innersource/applications.benchmarking.cpu-micros.mlc.git mlc; \
	fi
else
	cd mlc && git checkout master && git pull
endif
	-cd mlc && git checkout v3.10
	-cd mlc && make version && STATIC="-static" make

perf: linux-source
	cd linux/tools/perf && make LDFLAGS=-static

spectre-meltdown-checker:
ifeq ("$(wildcard spectre-meltdown-checker)","")
	git clone https://github.com/speed47/spectre-meltdown-checker.git
else
	cd spectre-meltdown-checker && git checkout master && git pull
endif

SSHPASS_VERSION := 1.10
sshpass:
ifeq ("$(wildcard sshpass)","")
	wget https://cytranet.dl.sourceforge.net/project/sshpass/sshpass/$(SSHPASS_VERSION)/sshpass-$(SSHPASS_VERSION).tar.gz
	tar -xf sshpass-$(SSHPASS_VERSION).tar.gz
	mv sshpass-$(SSHPASS_VERSION) sshpass
	rm sshpass-$(SSHPASS_VERSION).tar.gz
	cd sshpass && ./configure
endif
	cd sshpass && make

stress-ng:
ifeq ("$(wildcard stress-ng)","")
	git clone https://github.com/ColinIanKing/stress-ng.git
else
	cd stress-ng && git checkout master && git pull
endif
	cd stress-ng && git checkout V0.13.08
	cd stress-ng && STATIC=1 make

sysstat:
ifeq ("$(wildcard sysstat)","")
	git clone https://github.com/sysstat/sysstat.git
else
	cd sysstat && git checkout master && git pull
endif
ifeq ("$(wildcard sysstat/Makefile)","")
	cd sysstat && ./configure
endif
	cd sysstat && make

LINUX_VERSION := 6.1.52
linux-source:
ifeq ("$(wildcard linux)","")
ifeq ("$(wildcard linux-$(LINUX_VERSION).tar.xz)","")
	wget https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-$(LINUX_VERSION).tar.xz
endif
	tar -xf linux-$(LINUX_VERSION).tar.xz && mv linux-$(LINUX_VERSION)/ linux/
endif

turbostat: linux-source
	cd linux/tools/power/x86/turbostat && make

collector-deps-amd64: other_tools
	$(eval TMPDIR := $(shell mktemp -d build.XXXXXX))
	cp calcfreq/calcfreq $(TMPDIR)
	cp /prebuilt/cpuid/cpuid $(TMPDIR)
	cp /prebuilt/dmidecode/dmidecode $(TMPDIR)
	cp /prebuilt/ethtool/ethtool $(TMPDIR)
	cp /prebuilt/fio/fio $(TMPDIR)
	cp /prebuilt/ipmitool/src/ipmitool.static $(TMPDIR)/ipmitool
	cp /prebuilt/lshw/src/lshw-static $(TMPDIR)/lshw
	cp /prebuilt/lspci/lspci $(TMPDIR)
	cp /prebuilt/lspci/pci.ids.gz $(TMPDIR)
	-cp mlc/mlc $(TMPDIR)
	cp msrbusy/msrbusy $(TMPDIR)
	cp /prebuilt/linux/tools/perf/perf $(TMPDIR)
	cp -R /prebuilt/async-profiler $(TMPDIR)
	cp /prebuilt/flamegraph/stackcollapse-perf.pl $(TMPDIR)
	cp pmu2metrics/pmu2metrics $(TMPDIR)
	cp rdmsr/rdmsrx $(TMPDIR)
	cp wrmsr/wrmsrx $(TMPDIR)
	cp /prebuilt/spectre-meltdown-checker/spectre-meltdown-checker.sh $(TMPDIR)
	cp /prebuilt/stress-ng/stress-ng $(TMPDIR)
	cp /prebuilt/sysstat/mpstat $(TMPDIR)
	cp /prebuilt/sysstat/iostat $(TMPDIR)
	cp /prebuilt/sysstat/sar $(TMPDIR)
	cp /prebuilt/sysstat/sadc $(TMPDIR)
	cp /prebuilt/linux/tools/power/x86/turbostat/turbostat $(TMPDIR)
	-cp -r ../bin/* $(TMPDIR)
	for f in $(TMPDIR)/*; do strip -s -p --strip-unneeded $$f; done
	cd $(TMPDIR) && tar -czf ../orchestrator/resources/collector_deps_amd64.tgz .
	rm -rf $(TMPDIR)

collector-deps-arm64:
	$(eval TMPDIR := $(shell mktemp -d build.XXXXXX))
	cp /prebuilt/spectre-meltdown-checker/spectre-meltdown-checker.sh $(TMPDIR)
	cd $(TMPDIR) && tar -czf ../orchestrator/resources/collector_deps_arm64.tgz .
	rm -rf $(TMPDIR)

collector-deps: collector-deps-amd64 collector-deps-arm64

clean:
	rm -rf mlc *.tar.*
	rm -f calcfreq/calcfreq
	rm -f collector/collector
	rm -f collector/collector_arm64
	rm -f reporter/reporter
	rm -f orchestrator/orchestrator
	rm -f orchestrator/resources/collector
	rm -f orchestrator/resources/collector_arm64
	rm -f orchestrator/resources/collector_deps_amd64.tgz
	rm -f orchestrator/resources/collector_deps_arm64.tgz
	rm -f orchestrator/resources/reporter
	rm -f orchestrator/resources/sshpass
	rm -f rdmsr/rdmsrx
	rm -f wrmsr/wrmsrx
	rm -f msrbusy/msrbusy
	rm -f pmu2metrics/pmu2metrics

reset:
	cd /prebuilt/cpuid && make clean
	cd /prebuilt/dmidecode && git clean -fdx && git reset --hard
	cd /prebuilt/ethtool && git clean -fdx && git reset --hard
	cd /prebuilt/fio && git clean -fdx && git reset --hard
	cd /prebuilt/flamegraph && git clean -fdx && git reset --hard
	cd /prebuilt/ipmitool && git clean -fdx && git reset --hard
	cd /prebuilt/lshw && git clean -fdx && git reset --hard
	cd /prebuilt/lspci && git clean -fdx && git reset --hard
	-cd mlc && git clean -fdx && git reset --hard
	cd /prebuilt/linux/tools/perf && make clean
	cd /prebuilt/sshpass && make clean
	cd /prebuilt/stress-ng && git clean -fdx && git reset --hard
	cd /prebuilt/sysstat && git clean -fdx && git reset --hard
	cd /prebuilt/linux/tools/power/x86/turbostat && make clean

# not used in build but required in oss archive file because some of the tools are statically linked
glibc-2.19.tar.bz2:
	wget http://ftp.gnu.org/gnu/glibc/glibc-2.19.tar.bz2
zlib.tar.gz:
	wget https://github.com/madler/zlib/archive/refs/heads/master.tar.gz -O zlib.tar.gz
libcrypt.tar.gz:
	wget https://github.com/gpg/libgcrypt/archive/refs/heads/master.tar.gz -O libcrypt.tar.gz
libs: glibc-2.19.tar.bz2 zlib.tar.gz libcrypt.tar.gz

oss-source: reset libs
	tar --exclude-vcs -czf oss_source.tgz /prebuilt/async-profiler/ /prebuilt/dmidecode/ /prebuilt/ethtool/ /prebuilt/fio/ /prebuilt/flamegraph/ /prebuilt/ipmitool/ /prebuilt/linux/ /prebuilt/lshw/ /prebuilt/lspci/ /prebuilt/spectre-meltdown-checker/ /prebuilt/sshpass/ /prebuilt/sysstat/ /prebuilt/stress-ng/ glibc-2.19.tar.bz2 zlib.tar.gz libcrypt.tar.gz
	md5sum oss_source.tgz > oss_source.tgz.md5
