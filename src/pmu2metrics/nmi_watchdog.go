/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"fmt"
	"os/exec"
)

func getNmiWatchdog() (setting string, err error) {
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

func setNmiWatchdog(setting string) (err error) {
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
