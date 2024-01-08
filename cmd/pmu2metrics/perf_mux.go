/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
//
// Linux perf event/group multiplexing interval helper functions
//
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// GetMuxIntervals - get a map of sysfs device file names to current mux value for the associated device
func GetMuxIntervals() (intervals map[string]int, err error) {
	var paths []string
	if paths, err = getMuxIntervalFiles(); err != nil {
		return
	}
	intervals = make(map[string]int)
	for _, path := range paths {
		var contents []byte
		if contents, err = os.ReadFile(path); err != nil {
			err = nil
			continue
		}
		var interval int
		if interval, err = strconv.Atoi(string(contents)); err != nil {
			return
		}
		intervals[path] = interval
	}
	return
}

// SetMuxIntervals - write the given intervals (values in ms) to the given sysfs device file names (key)
func SetMuxIntervals(intervals map[string]int) (err error) {
	for device := range intervals {
		if err = setMuxInterval(device, intervals[device]); err != nil {
			return
		}
	}
	return
}

// SetAllMuxIntervals - writes the given interval (ms) to all perf mux sysfs device files
func SetAllMuxIntervals(interval int) (err error) {
	var paths []string
	if paths, err = getMuxIntervalFiles(); err != nil {
		return
	}
	for _, path := range paths {
		if err = setMuxInterval(path, interval); err != nil {
			return
		}
	}
	return
}

// getMuxIntervalFiles - get list of sysfs device file names used for getting/setting the mux interval
func getMuxIntervalFiles() (paths []string, err error) {
	pattern := filepath.Join("/", "sys", "devices", "*")
	var files []string
	if files, err = filepath.Glob(pattern); err != nil {
		return
	}
	for _, file := range files {
		var fileInfo os.FileInfo
		if fileInfo, err = os.Stat(file); err != nil {
			return
		}
		if fileInfo.IsDir() {
			fullPath := filepath.Join(file, "perf_event_mux_interval_ms")
			var fileInfo os.FileInfo
			if fileInfo, err = os.Stat(fullPath); err != nil {
				err = nil
				continue
			}
			if !fileInfo.IsDir() {
				paths = append(paths, fullPath)
			}
		}
	}
	return
}

// setMuxInterval - write the given interval (ms) to the given sysfs device file name
func setMuxInterval(device string, interval int) (err error) {
	err = os.WriteFile(device, []byte(fmt.Sprintf("%d", interval)), 0644)
	return
}
