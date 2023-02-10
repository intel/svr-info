# Intel&reg; System Health Inspector (aka svr-info)

## Getting svr-info

Download the latest release of svr-info from the repository's Releases page.

## Running svr-info

### Extract files from the archive and step into the directory

`tar -xf svr-info.tgz`

`cd svr-info`

### View options

`./svr-info -h`

### Collect configuration data on local host

`./svr-info`

## Contributing

We welcome bug reports, questions and feature requests. Please submit via Github Issues.

## Building svr-info

Due to the large number of build dependencies required, a Docker container-based build environment is provided. Assuming your system has Docker installed (instructions not provided here), the following steps are required to build svr-info:

- `builder/build_docker_image` creates the docker image
- `builder/build` runs `make dist` in the container

After a successful build, you will find the build output in the `dist` folder.

Other builder commands available:

- `builder/test` runs the automated tests in the container via `make test`
- `builder/shell` starts the container and provides a bash prompt useful for debugging build problems

### Incremental Builds
After a complete build using the build container, you can perform incremental builds directly on your host assuming dependencies are installed there. This can make the code/build/test cycle much quicker than rebuilding everything using the Docker container. You can look at the Dockerfile in the builder directory to get the build dependencies for everything or, more likely, you only need go(lang) so install the latest and get to work.

From the project's root directory, you can use the makefile. There are quite a few targets. Most useful may be `make apps`.  This will build all the go-based apps.

If you are working on a single go-based app. You can run `go build` in the app's source directory to build it.

### Additional Collection Tools
Additional data collection tools can be built into svr-info by placing binaries in the bin directory before starting the build. For example, Intel® Memory Latency Checker is a useful tool for identifying the health and performance of a server's memory subsystem. It can be downloaded from here: https://www.intel.com/content/www/us/en/download/736633/intel-memory-latency-checker-intel-mlc.html. Once downloaded, extract the Linux executable and place in the bin directory before starting the build.

## Architecture
There are three primary applications that make up svr-info. They are written in go and can all be run/tested independently.
1. orchestrator - runs on local host, communicates with remote targets via SSH, configures and runs the collector component on selected targets, then runs the reporter component to generate reports. Svr-info is a symbolic link to the orchestrator application.
2. collector - runs on local and/or remote targets to collect information to be fed into the reporter
3. reporter - generates reports from collector output
