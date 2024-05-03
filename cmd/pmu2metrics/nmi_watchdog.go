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
	"log"
	"os/exec"

	"github.com/intel/svr-info/internal/util"
)

// EnableNMIWatchdog - sets the kernel.nmi_watchdog value to "1"
func EnableNMIWatchdog() (err error) {
	if gCmdLineArgs.verbose {
		log.Print("enabling NMI watchdog")
	}
	err = setNMIWatchdog("1")
	return
}

// DisableNMIWatchdog - sets the kernel.nmi_watchdog value to "0"
func DisableNMIWatchdog() (err error) {
	if gCmdLineArgs.verbose {
		log.Print("disabling NMI watchdog")
	}
	err = setNMIWatchdog("0")
	return
}

// NMIWatchdogEnabled - reads the kernel.nmi_watchdog value. If it is "1", returns true
func NMIWatchdogEnabled() (enabled bool, err error) {
	var setting string
	if setting, err = getNMIWatchdog(); err != nil {
		return
	}
	enabled = setting == "1"
	return
}

// getNMIWatchdog - gets the kernel.nmi_watchdog configuration value (0 or 1)
func getNMIWatchdog() (setting string, err error) {
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

// setNMIWatchdog -sets the kernel.nmi_watchdog configuration value
func setNMIWatchdog(setting string) (err error) {
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
	if path, err = exec.LookPath("sysctl"); err == nil {
		// found it
		return
	}
	// didn't find it on the path, try being specific
	var exists bool
	sbinPath := "/usr/sbin/sysctl"
	if exists, err = util.FileExists(sbinPath); err != nil {
		return
	}
	if exists {
		path = sbinPath
	} else {
		err = fmt.Errorf("sysctl not found on path or at %s", sbinPath)
	}
	return
}
