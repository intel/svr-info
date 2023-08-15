FROM ubuntu:16.04
VOLUME /scripts
VOLUME /workdir
ENV LANG en_US.UTF-8
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y apt-utils locales wget curl git netcat-openbsd
RUN locale-gen en_US.UTF-8 &&  echo "LANG=en_US.UTF-8" > /etc/default/locale

# Needed for Celine CI/CD
RUN apt-get update && apt-get install -y software-properties-common
RUN add-apt-repository ppa:git-core/ppa -y
RUN apt-get update && apt-get install -y jq zip unzip git

# Install build dependencies for third party tools
RUN apt-get update && apt-get install -y build-essential autotools-dev automake \
    gawk zlib1g-dev libtool libaio-dev libaio1 pandoc pkgconf libcap-dev docbook-utils \
    libreadline-dev default-jre default-jdk cmake flex bison

ENV JAVA_HOME=/usr/lib/jvm/java-1.11.0-openjdk-amd64

#    need up-to-date zlib (used by fio and stress-ng static builds) to fix security vulnerabilities
RUN git clone https://github.com/madler/zlib.git && cd zlib && ./configure && make install
RUN cp /usr/local/lib/libz.a /usr/lib/x86_64-linux-gnu/libz.a

# Radamsa is used for fuzz testing
RUN curl -s https://gitlab.com/akihe/radamsa/uploads/a2228910d0d3c68d19c09cee3943d7e5/radamsa-0.6.c.gz | gzip -d | cc -O2 -x c -o /usr/local/bin/radamsa -

# Install Go
ARG GO_VERSION="1.21.0"
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz
RUN tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
RUN rm go${GO_VERSION}.linux-amd64.tar.gz
ENV PATH="$PATH:/usr/local/go/bin"

# so that build output files have the correct owner
# add non-root user
ARG USERNAME
ARG USERID
ARG LOCALBUILD
RUN if [ ! -z "${LOCALBUILD}" ] ; then \
    adduser --disabled-password --uid ${USERID} --gecos '' ${USERNAME} \
    && adduser ${USERNAME} sudo \
    && echo "${USERNAME} ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers; \
    fi

# Run container as non-root user from here onwards
USER ${USERNAME}

# run bash script and process the input command
ENTRYPOINT [ "/bin/bash", "/scripts/entrypoint"]
