# image contains svr-info release package build environment
# build image:
#   $ docker build --build-arg TAG=v1 -f builder/build.Dockerfile --tag svr-info-builder:v1 .
# build svr-info:
#   $ docker run --rm -v "$PWD":/localrepo -w /localrepo svr-info-builder:v1 make dist

ARG REGISTRY=
ARG PREFIX=
ARG TAG=
# STAGE 1 - image contains pre-built third-party components, rebuild the image to rebuild the third-party components
FROM ${REGISTRY}${PREFIX}svr-info-third-party:${TAG} AS third-party

# STAGE 2- image contains svr-info's Go components build environment
FROM ${REGISTRY}${PREFIX}svr-info-cmd-builder:${TAG} AS svr-info
RUN mkdir /prebuilt
RUN mkdir /prebuilt/third-party
RUN mkdir /prebuilt/bin
COPY --from=third-party /bin/ /prebuilt/third-party
COPY --from=third-party /oss_source* /prebuilt
RUN git config --global --add safe.directory /localrepo