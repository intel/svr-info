#!make
#
# Copyright (C) 2023 Intel Corporation
# SPDX-License-Identifier: MIT
#

default: tools
.PHONY: default tools async-profiler avx-turbo cpuid dmidecode ethtool fio flamegraph ipmitool lshw lspci mlc pcm perf spectre-meltdown-checker sshpass stress-ng sysstat turbostat

tools: async-profiler avx-turbo cpuid dmidecode ethtool fio flamegraph ipmitool lshw lspci mlc pcm perf spectre-meltdown-checker sshpass stress-ng sysstat turbostat
	mkdir -p bin
	cp -R async-profiler bin/
	cp avx-turbo/avx-turbo bin/
	cp cpuid/cpuid bin/
	cp dmidecode/dmidecode bin/
	cp ethtool/ethtool bin/
	cp fio/fio bin/
	cp flamegraph/stackcollapse-perf.pl bin/
	cp ipmitool/src/ipmitool.static bin/ipmitool
	cp lshw/src/lshw-static bin/lshw
	cp lspci/lspci bin/
	cp lspci/pci.ids.gz bin/
	-cp mlc/mlc bin/
	cp pcm/build/bin/pcm-tpmi bin/
	cp linux/tools/perf/perf bin/
	cp spectre-meltdown-checker/spectre-meltdown-checker.sh bin/
	cp sshpass/sshpass bin/
	cp stress-ng/stress-ng bin/
	cp sysstat/mpstat bin/
	cp sysstat/iostat bin/
	cp sysstat/sar bin/
	cp sysstat/sadc bin/
	cp linux/tools/power/x86/turbostat/turbostat bin/

ASYNCPROFILER_VERSION := 2.9
async-profiler:
ifeq ("$(wildcard async-profiler)","")
ifeq ("$(wildcard async-profiler-$(ASYNCPROFILER_VERSION)-linux-x64.tar.gz)","")
	wget https://github.com/jvm-profiling-tools/async-profiler/releases/download/v$(ASYNCPROFILER_VERSION)/async-profiler-$(ASYNCPROFILER_VERSION)-linux-x64.tar.gz
endif
	tar -xf async-profiler-$(ASYNCPROFILER_VERSION)-linux-x64.tar.gz && mv async-profiler-$(ASYNCPROFILER_VERSION)-linux-x64 async-profiler
endif

# if you change the commit, check the sed hacks below
AVXTURBO_COMMIT := 4a04a45
avx-turbo:
ifeq ("$(wildcard avx-turbo)","")
	git clone https://github.com/travisdowns/avx-turbo.git
else
	cd avx-turbo && git checkout master && git pull
endif
	cd avx-turbo && git checkout $(AVXTURBO_COMMIT)
	cd avx-turbo && sed -i 281s/"read_msr_cur_cpu(MSR_IA32_MPERF/read_msr(1, MSR_IA32_MPERF"/ avx-turbo.cpp
	cd avx-turbo && sed -i 282s/"read_msr_cur_cpu(MSR_IA32_APERF/read_msr(1, MSR_IA32_APERF"/ avx-turbo.cpp
	cd avx-turbo && make

# if you change the version, check the sed hacks below
CPUID_VERSION := 20240409
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
	cd ethtool && git checkout v6.5
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
	cd fio && git checkout fio-3.36
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
ifndef GITHUB_ACCESS_TOKEN
	-git clone git@github.com:intel-innersource/applications.benchmarking.cpu-micros.mlc.git mlc
else
	-git clone https://${GITHUB_ACCESS_TOKEN}@github.com/intel-innersource/applications.benchmarking.cpu-micros.mlc.git mlc
endif
else
	cd mlc && git checkout master && git pull
endif
	-cd mlc && git checkout v3.11
	-cd mlc && make version && STATIC="-static" make

pcm:
ifeq ("$(wildcard pcm)","")
	git clone --recursive https://github.com/intel/pcm.git
else
	cd pcm && git checkout master && git pull
endif
	mkdir -p pcm/build
	cd pcm/build &&	cmake ..
	cd pcm/build && cmake --build .

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

reset:
	cd async-profiler
	cd cpuid && make clean
	cd dmidecode && git clean -fdx && git reset --hard
	cd ethtool && git clean -fdx && git reset --hard
	cd fio && git clean -fdx && git reset --hard
	cd flamegraph && git clean -fdx && git reset --hard
	cd ipmitool && git clean -fdx && git reset --hard
	cd lshw && git clean -fdx && git reset --hard
	cd lspci && git clean -fdx && git reset --hard
	cd pcm && git clean -fdx && git reset --hard
	-cd mlc && git clean -fdx && git reset --hard
	cd linux/tools/perf && make clean
	cd spectre-meltdown-checker
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
	tar --exclude-vcs -czf oss_source.tgz async-profiler/ cpuid/ dmidecode/ ethtool/ fio/ flamegraph/ ipmitool/ lshw/ lspci/ pcm/ linux/tools/perf spectre-meltdown-checker/ sshpass/ stress-ng/ sysstat/ linux/tools/power/x86/turbostat glibc-2.19.tar.bz2 zlib.tar.gz libcrypt.tar.gz
	md5sum oss_source.tgz > oss_source.tgz.md5
