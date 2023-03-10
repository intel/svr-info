#!make
#
# Copyright (C) 2023 Intel Corporation
# SPDX-License-Identifier: MIT
#

VERSION ?= dev

default: apps
.PHONY: default all linux-source collector reporter orchestrator rdmsr msrbusy reset clean tools libs async-profiler burn calcfreq cpuid dmidecode ethtool fio flamegraph ipmitool lshw mlc perf spectre-meltdown-checker sshpass stress-ng sysstat turbostat oss-source

package_tests:
	cd pkg/commandfile && go test -v -vet=all .
	cd pkg/core && go test -v -vet=all .
	cd pkg/cpu && go test -v -vet=all .
	# these tests require access to MSRs which we don't have on WSL2 and may not have on build machine -- cd pkg/msr && go test -v -vet=all .
	cd pkg/progress && go test -v -vet=all .
	cd pkg/target && go test -v -vet=all .

orchestrator: package_tests
	cd orchestrator && go test -v -vet=all .
	cd orchestrator && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '-s -w -X main.gVersion=$(VERSION)' -o orchestrator

collector:
	cd collector && go test -v -vet=all .
	cd collector && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '-s -w -X main.gVersion=$(VERSION)' -o collector
	cd collector && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags '-s -w -X main.gVersion=$(VERSION)' -o collector_arm64

reporter:
	cd reporter && go test -v -vet=all .
	cd reporter && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '-s -w -X main.gVersion=$(VERSION)' -o reporter

rdmsr:
	cd rdmsr && go test -v -vet=all .
	cd rdmsr && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '-s -w -X main.gVersion=$(VERSION)' -o rdmsr

msrbusy:
	cd msrbusy && go test -v -vet=all .
	cd msrbusy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '-s -w -X main.gVersion=$(VERSION)' -o msrbusy

async-profiler:
ifeq ("$(wildcard async-profiler)","")
ifeq ("$(wildcard async-profiler-2.9-linux-x64.tar.gz)","")
	wget https://github.com/jvm-profiling-tools/async-profiler/releases/download/v2.9/async-profiler-2.9-linux-x64.tar.gz
endif
	tar -xf async-profiler-2.9-linux-x64.tar.gz && mv async-profiler-2.9-linux-x64 async-profiler
endif

burn:
ifeq ("$(wildcard burn)","")
	git clone https://github.com/Granulate/burn.git
	cd burn && git checkout v1.0.1g2
endif
	cd burn && GOWORK=off CGO_ENABLED=0 go build

calcfreq:
	cd calcfreq && VERSION=$(VERSION) make

cpuid:
ifeq ("$(wildcard cpuid)","")
ifeq ("$(wildcard cpuid-20221003.src.tar.gz)","")
	wget http://www.etallen.com/cpuid/cpuid-20221003.src.tar.gz
endif	
	tar -xf cpuid-20221003.src.tar.gz && mv cpuid-20221003/ cpuid/
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
	cd dmidecode && git checkout dmidecode-3-4
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
	mkdir -p spectre-meltdown-checker
	rm -f spectre-meltdown-checker/spectre-meltdown-checker.sh
	# get script from this PR https://github.com/speed47/spectre-meltdown-checker/pull/418
	cd spectre-meltdown-checker && wget https://raw.githubusercontent.com/speed47/spectre-meltdown-checker/3c4f5e4d8e0fc6fc828c5bc164f20372eb2537ac/spectre-meltdown-checker.sh
	chmod +x spectre-meltdown-checker/spectre-meltdown-checker.sh

sshpass:
ifeq ("$(wildcard sshpass)","")
	wget https://cytranet.dl.sourceforge.net/project/sshpass/sshpass/1.09/sshpass-1.09.tar.gz
	tar -xf sshpass-1.09.tar.gz
	mv sshpass-1.09 sshpass
	rm sshpass-1.09.tar.gz
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

linux-source:
ifeq ("$(wildcard linux)","")
ifeq ("$(wildcard linux-5.19.16.tar.xz)","")
	wget https://cdn.kernel.org/pub/linux/kernel/v5.x/linux-5.19.16.tar.xz
endif
	tar -xf linux-5.19.16.tar.xz && mv linux-5.19.16/ linux/
endif

turbostat: linux-source
	cd linux/tools/power/x86/turbostat && make

apps: collector reporter orchestrator
tools: rdmsr msrbusy async-profiler burn calcfreq cpuid dmidecode ethtool fio flamegraph ipmitool lshw mlc perf spectre-meltdown-checker sshpass stress-ng sysstat turbostat

clean:
	rm -rf async-profiler burn cpuid dmidecode ethtool fio flamegraph ipmitool linux lshw mlc spectre-meltdown-checker sshpass stress-ng sysstat oss_source.* linux-*.xz cpuid-*.gz glibc-*.bz2 libcrypt*.gz zlib.*.gz
	rm -f calcfreq/calcfreq
	rm -f collector/collector
	rm -f collector/collector_arm64
	rm -f collector/collector.exe
	rm -f reporter/reporter
	rm -f reporter/reporter_arm64
	rm -f reporter/reporter.exe
	rm -f orchestrator/orchestrator
	rm -f orchestrator/orchestrator_arm64
	rm -f orchestrator/orchestrator.exe
	rm -f rdmsr/rdmsr
	rm -f msrbusy/msrbusy

reset:
	cd burn && git clean -fdx && git reset --hard
	cd cpuid && make clean
	cd dmidecode && git clean -fdx && git reset --hard
	cd ethtool && git clean -fdx && git reset --hard
	cd fio && git clean -fdx && git reset --hard
	cd flamegraph && git clean -fdx && git reset --hard
	cd ipmitool && git clean -fdx && git reset --hard
	cd lshw && git clean -fdx && git reset --hard
	-cd mlc && git clean -fdx && git reset --hard
	cd linux/tools/perf && make clean
	cd sshpass && make clean
	cd stress-ng && git clean -fdx && git reset --hard
	cd sysstat && git clean -fdx && git reset --hard
	cd linux/tools/power/x86/turbostat && make clean

# not used in build but required in oss archive file because some of the tools are statically linked
glibc-2.19.tar.bz2:
	wget http://ftp.gnu.org/gnu/glibc/glibc-2.19.tar.bz2
zlib.tar.gz:
	wget https://github.com/madler/zlib/archive/refs/heads/master.tar.gz -O zlib.tar.gz
libcrypt.tar.gz:
	wget https://github.com/gpg/libgcrypt/archive/refs/heads/master.tar.gz -O libcrypt.tar.gz
libs: glibc-2.19.tar.bz2 zlib.tar.gz libcrypt.tar.gz

oss-source: reset libs
	tar --exclude-vcs -czf oss_source.tgz async-profiler/ dmidecode/ ethtool/ fio/ flamegraph/ ipmitool/ linux/ lshw/ spectre-meltdown-checker/ sshpass/ sysstat/ stress-ng/ glibc-2.19.tar.bz2 zlib.tar.gz libcrypt.tar.gz
	md5sum oss_source.tgz > oss_source.tgz.md5
