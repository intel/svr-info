#!/bin/bash

# exit on error
#set -e

# show commands
set -x

# defaults
DURATION=30
PMU2METRICS=../bin/pmu2metrics
STRESSNG=/usr/bin/stress-ng

while getopts "m:p:s:t:" opt; do
    case $opt in
        m)
            PMU2METRICS=$OPTARG
            ;;
        p)
            PERFSPECT=$OPTARG
            ;;
        s)
            STRESSNG=$OPTARG
            ;;
        t)
            DURATION=$OPTARG
            ;;
        *)
            echo "Invalid option: $OPTARG"
            exit 1
    esac
done

if [[ -z "$PERFSPECT" ]]; then
    echo "path to perfspect not set, skipping perfspect"
else
    if [ ! -f "$PERFSPECT"/perf-collect ]; then
        echo "perfspect not found at $PERFSPECT"
        exit 1
    fi
fi

if [ ! -f "$PMU2METRICS" ]; then
    echo "pmu2metrics not found at $PMU2METRICS"
    exit 1
fi

if [ ! -f "$STRESSNG" ]; then
    echo "stress-ng not found at $STRESSNG"
    exit 1
fi


# system wide test
$STRESSNG --cpu 0 --cpu-load 50 >/dev/null 2>&1 &
SNGPID=$!
sudo "$PMU2METRICS" -csv -t "$DURATION" 2>system-wide.log 1>system-wide.csv
if [[ -v PERFSPECT ]]; then
    sudo "$PERFSPECT"/perf-collect --timeout "$DURATION" -o ./system-wide-ps.out 1>system-wide-psc.log 2>&1
fi
kill "$SNGPID"
"$PMU2METRICS" --post-process system-wide.csv --format html 1>system-wide-stats.html 2>>system-wide.log
"$PMU2METRICS" --post-process system-wide.csv --format csv 1>system-wide-stats.csv 2>>system-wide.log
if [[ -v PERFSPECT ]]; then
    "$PERFSPECT"/perf-postprocess -r ./system-wide-ps.out -o ./system-wide-ps.csv > system-wide-psp.log 2>&1
fi

# per-process test
$STRESSNG --cpu 0 --cpu-load 50 >/dev/null 2>&1 &
SNGPID=$!
sudo "$PMU2METRICS" -per-process -pid "$SNGPID" -csv -t "$DURATION" 2>per-process.log 1>per-process.csv
sudo "$PERFSPECT"/perf-collect --pid "$SNGPID" --timeout "$DURATION" -o ./per-process-ps.out 1>per-process-psc.log 2>&1
kill "$SNGPID"
"$PMU2METRICS" --post-process per-process.csv --format html -pid "$SNGPID" 1>per-process-stats.html 2>>per-process.log
"$PMU2METRICS" --post-process per-process.csv --format csv -pid "$SNGPID" 1>per-process-stats.csv 2>>per-process.log
"$PERFSPECT"/perf-postprocess -r ./per-process-ps.out -o ./per-process-ps.csv >per-process-psp.log 2>&1

# per-cgroup test
docker pull colinianking/stress-ng || true
docker image prune -f || true
SNGCID=$(docker run --rm --detach colinianking/stress-ng --cpu 0 --cpu-load 50)
sudo "$PMU2METRICS" -per-cgroup -cid "$SNGCID" -csv -t "$DURATION" 2>per-cgroup.log 1>per-cgroup.csv
sudo "$PERFSPECT"/perf-collect --cid "$SNGCID" --timeout "$DURATION" -o ./per-cgroup-ps.out 1>per-cgroup-psc.log 2>&1
docker kill "$SNGCID"
"$PMU2METRICS" --post-process per-cgroup.csv --format html -cid "$SNGCID" 1>per-cgroup-stats.html 2>>per-cgroup.log
"$PMU2METRICS" --post-process per-cgroup.csv --format csv -cid "$SNGCID" 1>per-cgroup-stats.csv 2>>per-cgroup.log
"$PERFSPECT"/perf-postprocess -r ./per-cgroup-ps.out -o ./per-cgroup-ps.csv >per-cgroup-psp.log 2>&1