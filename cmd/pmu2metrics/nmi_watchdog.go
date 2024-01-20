/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
//
// nmi_watchdog provides helper functions for enabling and disabling the NMI (non-maskable interrupt) watchdog
//
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetNMIWatchdog - gets the kernel.nmi_watchdog configuration value (0 or 1)
func GetNMIWatchdog() (setting string, err error) {
	// sysctl kernel.nmi_watchdog
	// kernel.nmi_watchdog = [0|1]
	var sysctl string
	if sysctl, err = findSysctl(); err != nil {
		return
	}
	cmd := exec.Command(sysctl, "kernel.nmi_watchdog")
	var stdout []byte
	if stdout, err = cmd.Output(); err != nil {
		return
	}
	out := string(stdout)
	setting = out[len(out)-2 : len(out)-1]
	return
}

// SetNMIWatchdog -sets the kernel.nmi_watchdog configuration value
func SetNMIWatchdog(setting string) (err error) {
	// sysctl kernel.nmi_watchdog=[0|1]
	var sysctl string
	if sysctl, err = findSysctl(); err != nil {
		return
	}
	cmd := exec.Command(sysctl, fmt.Sprintf("kernel.nmi_watchdog=%s", setting))
	var stdout []byte
	if stdout, err = cmd.Output(); err != nil {
		return
	}
	out := string(stdout)
	outSetting := out[len(out)-2 : len(out)-1]
	if outSetting != setting {
		err = fmt.Errorf("failed to set NMI watchdog to %s", setting)
	}
	return
}

// findSysctl - gets a useable path to sysctl or error
func findSysctl() (path string, err error) {
	if path, err = findInPath("sysctl"); err == nil {
		// found it
		return
	}
	// didn't find it on the path, try being specific
	var exists bool
	sbinPath := "/usr/sbin/sysctl"
	if exists, err = fileExists(sbinPath); err != nil {
		return
	}
	if exists {
		path = sbinPath
	} else {
		err = fmt.Errorf("sysctl not found on path or at %s", sbinPath)
	}
	return
}

// findInPath returns the full path to a program or error if not found
func findInPath(program string) (fullPath string, err error) {
	path := os.Getenv("PATH")
	dirs := strings.Split(path, string(os.PathListSeparator))
	for _, dir := range dirs {
		fullPath = filepath.Join(dir, program)
		if exists, _ := fileExists(fullPath); exists {
			return
		}
	}
	err = fmt.Errorf("%s not found in path: %s", program, path)
	return
}

// fileExists returns whether the given file or directory exists
func fileExists(path string) (exists bool, err error) {
	if _, err = os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			exists = false
			err = nil
			return
		}
		return
	}
	exists = true
	return
}
