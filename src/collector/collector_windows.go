/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"log"
	"os/exec"
	"strings"

	"intel.com/svr-info/pkg/target"
)

func runCommand(label string, command string, superuser bool, sudoPassword string, binPath string, timeout int) (stdout string, stderr string, exitCode int, err error) {
	if superuser {
		return runSuperUserCommand(command, sudoPassword, timeout)
	}
	return runRegularUserCommand(command, timeout)
}

func runRegularUserCommand(command string, timeout int) (stdout string, stderr string, exitCode int, err error) {
	log.Printf("runRegularUserCommand Start: %s", command)
	defer log.Printf("runRegularUserCommand Finish: %s", command)
	cmdList := strings.Split(command, " ")
	var cmd *exec.Cmd
	if len(cmdList) > 1 {
		cmd = exec.Command(cmdList[0], cmdList[1:]...)
	} else {
		cmd = exec.Command(command)
	}
	return target.RunLocalCommand(cmd)
}

func runSuperUserCommand(command string, sudoPassword string, timeout int) (stdout string, stderr string, exitCode int, err error) {
	return runRegularUserCommand(command, timeout)
}

func installMods(mods string, sudoPassword string) (installedMods []string) {
	return
}

func uninstallMods(modList []string, sudoPassword string) (err error) {
	return
}
