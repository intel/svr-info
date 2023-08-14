# Intel&reg; System Health Inspector [![Build](https://github.com/intel/svr-info/actions/workflows/build-test.yml/badge.svg)](https://github.com/intel/svr-info/actions/workflows/build-test.yml)[![License](https://img.shields.io/badge/License-MIT-blue)](https://github.com/intel/svr-info/blob/master/LICENSE)
System Health Inspector aka "svr-info" is a Linux command line tool used to assess the health of Intel® Xeon® processor-based servers.
## Quick Start
```
wget -qO- https://github.com/intel/svr-info/releases/latest/download/svr-info.tgz | tar xvz
cd svr-info
./svr-info
```
![sample-reports](/docs/images/sample-reports.jpg)
## Example
[HTML Report](https://intel.github.io/svr-info/)
# Options
## Remote Target
Data can be collected from a single remote target by providing the login credentials of the target on the svr-info command line.
```
./svr-info -ip 10.100.222.123 -user fred -key ~/.ssh/id_rsa
```
## Multiple Targets
Data can be collected from multiple remote targets by placing login credentials of the targets in a 'targets' file and then referencing that targets file on the svr-info command line. See the included [targets.example](src/orchestrator/targets.example) file for the required file format.
```
./svr-info -targets <targets file>
```
## Benchmarks
Micro-benchmarks can be executed by svr-info to assess the health of the target system(s). See the help (-h) for the complete list of available benchmarks. To run all benchmarks:
```
./svr-info -benchmark all
```
Notes:
- **Benchmarks should not be run on live/production systems.** Production workload performance may be impacted.
- Running all benchmarks, i.e., `--benchmark all`, will take 4+ minutes to run. The frequency benchmark execution time increases with core count (approx. (# of cores + 10)s). If not all benchmarks are required, use the `--help` option to see how to choose specific benchmarks, e.g., `--benchmark cpu,disk`.
## System Profiling
Subsystems on live/production system(s) can be profiled by svr-info. See the help (-h) for the complete list of subsystems. To profile all subsystems:
```
./svr-info -profile all
```
## Workload Analysis
Workloads on live/production system(s) can be analyzed by svr-info. One or more perf flamegraphs will be produced. See the help (-h) for options. To analyze system and Java apps:
```
./svr-info -analyze all
```
## Report Types
By default svr-info produces HTML, JSON, and Microsoft Excel formatted reports. There is an optional txt report that includes the commands that were executed on the target to collect data and their output. See the help (-h) for report format options. To generate only HTML reports:
```
./svr-info -format html
```
## Additional Data Collection Tools
Additional data collection tools can be used by svr-info by placing them in a directory named "extras".
For example, Intel® Memory Latency Checker can be downloaded from here: [MLC](https://www.intel.com/content/www/us/en/download/736633/intel-memory-latency-checker-intel-mlc.html). Once downloaded, extract the Linux executable and place in the svr-info/extras directory.
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

### Including Additional Collection Tools In The Build
Additional data collection tools can be built into the svr-info distribution by placing binaries in the bin directory before starting the build.
