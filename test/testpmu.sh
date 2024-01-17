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

# just a safety net, in case kill fails for some reason, * 4 to account for startup and processing
SNG_TIMEOUT=$((DURATION * 4))

# system scope, system granularity test
TESTNAME=system-system
$STRESSNG --cpu 0 --cpu-load 50 --timeout "$SNG_TIMEOUT" >/dev/null 2>&1 &
SNGPID=$!
sleep 1
sudo "$PMU2METRICS" --output csv --timeout "$DURATION" 2>"$TESTNAME".log 1>"$TESTNAME".csv
if [[ -v PERFSPECT ]]; then
    sudo "$PERFSPECT"/perf-collect --timeout "$DURATION" -o ./"$TESTNAME"-ps.out 1>/dev/null 2>&1
fi
kill "$SNGPID"
"$PMU2METRICS" --post-process "$TESTNAME".csv --format html 1>"$TESTNAME"-stats.html 2>>"$TESTNAME".log
"$PMU2METRICS" --post-process "$TESTNAME".csv --format csv 1>"$TESTNAME"-stats.csv 2>>"$TESTNAME".log
if [[ -v PERFSPECT ]]; then
    "$PERFSPECT"/perf-postprocess -r ./"$TESTNAME"-ps.out -o ./"$TESTNAME"-ps.csv >/dev/null 2>&1
fi

# system scope, socket granularity test
TESTNAME=system-socket
$STRESSNG --cpu 0 --cpu-load 50 --timeout "$SNG_TIMEOUT" >/dev/null 2>&1 &
SNGPID=$!
sleep 1
sudo "$PMU2METRICS" --granularity socket --output csv --timeout "$DURATION" 2>"$TESTNAME".log 1>"$TESTNAME".csv
if [[ -v PERFSPECT ]]; then
    sudo "$PERFSPECT"/perf-collect --socket --timeout "$DURATION" -o ./"$TESTNAME"-ps.out 1>/dev/null 2>&1
fi
kill "$SNGPID"
"$PMU2METRICS" --post-process "$TESTNAME".csv --format csv 1>"$TESTNAME"-stats.csv 2>>"$TESTNAME".log
if [[ -v PERFSPECT ]]; then
    "$PERFSPECT"/perf-postprocess -r ./"$TESTNAME"-ps.out -o ./"$TESTNAME"-ps.csv > /dev/null 2>&1
fi

# system scope, cpu granularity test
TESTNAME=system-cpu
$STRESSNG --cpu 0 --cpu-load 50 --timeout "$SNG_TIMEOUT" >/dev/null 2>&1 &
SNGPID=$!
sleep 1
sudo "$PMU2METRICS" --granularity cpu --output csv --timeout "$DURATION" 2>"$TESTNAME".log 1>"$TESTNAME".csv
if [[ -v PERFSPECT ]]; then
    sudo "$PERFSPECT"/perf-collect --cpu --timeout "$DURATION" -o ./"$TESTNAME"-ps.out 1>/dev/null 2>&1
fi
kill "$SNGPID"
"$PMU2METRICS" --post-process "$TESTNAME".csv --format csv 1>"$TESTNAME"-stats.csv 2>>"$TESTNAME".log
if [[ -v PERFSPECT ]]; then
    "$PERFSPECT"/perf-postprocess -r ./"$TESTNAME"-ps.out -o ./"$TESTNAME"-ps.csv > /dev/null 2>&1
fi

# process scope test (hot pids)
TESTNAME=process-hot
$STRESSNG --cpu 5 --cpu-load 50 --timeout "$SNG_TIMEOUT" >/dev/null 2>&1 &
SNGPID=$!
sleep 1
sudo "$PMU2METRICS" --scope process --output csv --timeout "$DURATION" 2>"$TESTNAME".log 1>"$TESTNAME".csv
if [[ -v PERFSPECT ]]; then
    sudo "$PERFSPECT"/perf-collect --pid --timeout "$DURATION" -o ./"$TESTNAME"-ps.out 1>/dev/null 2>&1
fi
kill "$SNGPID"
"$PMU2METRICS" --post-process "$TESTNAME".csv --format csv 1>"$TESTNAME"-stats.csv 2>>"$TESTNAME".log
if [[ -v PERFSPECT ]]; then
    "$PERFSPECT"/perf-postprocess -r ./"$TESTNAME"-ps.out -o ./"$TESTNAME"-ps.csv >/dev/null 2>&1
fi

# process scope test (specify pids)
TESTNAME=process-pids
$STRESSNG --cpu 5 --cpu-load 50 --timeout "$SNG_TIMEOUT" >/dev/null 2>&1 &
SNGPID=$!
sleep 1
CHILDPIDS=$(pgrep -P $SNGPID | paste -sd ",")
sudo "$PMU2METRICS" --scope process --pid "$CHILDPIDS" --output csv --timeout "$DURATION" 2>"$TESTNAME".log 1>"$TESTNAME".csv
if [[ -v PERFSPECT ]]; then
    sudo "$PERFSPECT"/perf-collect --pid "$CHILDPIDS" --timeout "$DURATION" -o ./"$TESTNAME"-ps.out 1>/dev/null 2>&1
fi
kill "$SNGPID"
"$PMU2METRICS" --post-process "$TESTNAME".csv --format csv 1>"$TESTNAME"-stats.csv 2>>"$TESTNAME".log
if [[ -v PERFSPECT ]]; then
    "$PERFSPECT"/perf-postprocess -r ./"$TESTNAME"-ps.out -o ./"$TESTNAME"-ps.csv >/dev/null 2>&1
fi

# cgroup scope test (hot cids)
TESTNAME=cgroup-hot
docker pull colinianking/stress-ng || true
docker image prune -f || true
SNGCID1=$(docker run --rm --detach colinianking/stress-ng --cpu 5 --cpu-load 50 --timeout "$SNG_TIMEOUT")
SNGCID2=$(docker run --rm --detach colinianking/stress-ng --cpu 5 --cpu-load 50 --timeout "$SNG_TIMEOUT")
sleep 1
sudo "$PMU2METRICS" --scope cgroup --count 2 --output csv --timeout "$DURATION" 2>"$TESTNAME".log 1>"$TESTNAME".csv
if [[ -v PERFSPECT ]]; then
    sudo "$PERFSPECT"/perf-collect --cid "$SNGCID1","$SNGCID2" --timeout "$DURATION" -o ./"$TESTNAME"-ps.out 1>/dev/null 2>&1
fi
docker kill "$SNGCID1"
docker kill "$SNGCID2"
"$PMU2METRICS" --post-process "$TESTNAME".csv --format csv 1>"$TESTNAME"-stats.csv 2>>"$TESTNAME".log
if [[ -v PERFSPECT ]]; then
    "$PERFSPECT"/perf-postprocess -r ./"$TESTNAME"-ps.out -o ./"$TESTNAME"-ps.csv >/dev/null 2>&1
fi

# cgroup scope test (specify cids)
TESTNAME=cgroup-cids
docker pull colinianking/stress-ng || true
docker image prune -f || true
SNGCID1=$(docker run --rm --detach colinianking/stress-ng --cpu 5 --cpu-load 50 --timeout "$SNG_TIMEOUT")
SNGCID2=$(docker run --rm --detach colinianking/stress-ng --cpu 5 --cpu-load 50 --timeout "$SNG_TIMEOUT")
sleep 1
sudo "$PMU2METRICS" --scope cgroup --cid "$SNGCID1","$SNGCID2" --output csv --timeout "$DURATION" 2>"$TESTNAME".log 1>"$TESTNAME".csv
if [[ -v PERFSPECT ]]; then
    sudo "$PERFSPECT"/perf-collect --cid "$SNGCID1","$SNGCID2" --timeout "$DURATION" -o ./"$TESTNAME"-ps.out 1>/dev/null 2>&1
fi
docker kill "$SNGCID1"
docker kill "$SNGCID2"
"$PMU2METRICS" --post-process "$TESTNAME".csv --format csv 1>"$TESTNAME"-stats.csv 2>>"$TESTNAME".log
if [[ -v PERFSPECT ]]; then
    "$PERFSPECT"/perf-postprocess -r ./"$TESTNAME"-ps.out -o ./"$TESTNAME"-ps.csv >/dev/null 2>&1
fi
