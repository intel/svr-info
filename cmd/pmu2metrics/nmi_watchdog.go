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
	"os/exec"
)

// GetNMIWatchdog - gets the kernel.nmi_watchdog configuration value (0 or 1)
func GetNMIWatchdog() (setting string, err error) {
	// sysctl kernel.nmi_watchdog
	// kernel.nmi_watchdog = [0|1]
	cmd := exec.Command("sysctl", "kernel.nmi_watchdog")
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
	cmd := exec.Command("sysctl", fmt.Sprintf("kernel.nmi_watchdog=%s", setting))
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
