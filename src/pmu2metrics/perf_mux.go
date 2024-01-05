/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

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

func getMuxIntervals() (intervals map[string]string, err error) {
	var paths []string
	if paths, err = getMuxIntervalFiles(); err != nil {
		return
	}
	intervals = make(map[string]string)
	for _, path := range paths {
		var contents []byte
		if contents, err = os.ReadFile(path); err != nil {
			err = nil
			continue
		}
		intervals[path] = string(contents)
	}
	return
}

func setMuxInterval(device string, interval string) (err error) {
	err = os.WriteFile(device, []byte(interval), 0644)
	return
}

func setMuxIntervals(intervals map[string]string) (err error) {
	for device := range intervals {
		if err = setMuxInterval(device, intervals[device]); err != nil {
			return
		}
	}
	return
}

func setAllMuxIntervals(interval int) (err error) {
	var paths []string
	if paths, err = getMuxIntervalFiles(); err != nil {
		return
	}
	for _, path := range paths {
		if err = setMuxInterval(path, fmt.Sprintf("%d\n", interval)); err != nil {
			return
		}
	}
	return
}
