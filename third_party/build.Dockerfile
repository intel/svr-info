# builds third_party components used by svr-info
# build output binaries will be in workdir/bin
# build output oss_source* will be in workdir
# build image (third_party directory):
#   $ GITHUB_ACCESS_TOKEN=<your token>
#   $ docker image build -f build.Dockerfile --tag svr-info-third-party:v1 .
FROM ubuntu:18.04 as builder
ENV LANG en_US.UTF-8
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y apt-utils locales wget curl git netcat-openbsd software-properties-common jq zip unzip
RUN locale-gen en_US.UTF-8 &&  echo "LANG=en_US.UTF-8" > /etc/default/locale
RUN add-apt-repository ppa:git-core/ppa -y
RUN apt-get update && apt-get install -y git build-essential autotools-dev automake \
    gawk zlib1g-dev libtool libaio-dev libaio1 pandoc pkgconf libcap-dev docbook-utils \
    libreadline-dev default-jre default-jdk cmake flex bison

ENV JAVA_HOME=/usr/lib/jvm/java-1.11.0-openjdk-amd64

# need up-to-date zlib (used by fio and stress-ng static builds) to fix security vulnerabilities
RUN git clone https://github.com/madler/zlib.git && cd zlib && ./configure && make install
RUN cp /usr/local/lib/libz.a /usr/lib/x86_64-linux-gnu/libz.a

# Build third-party components
RUN mkdir workdir
COPY Makefile workdir/
ARG GITHUB_ACCESS_TOKEN
ENV GITHUB_ACCESS_TOKEN ${GITHUB_ACCESS_TOKEN}
RUN cd workdir && make -j4 tools && make oss-source

FROM scratch as output
COPY --from=builder workdir/bin /bin
COPY --from=builder workdir/oss_source* /
