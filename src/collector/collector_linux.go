/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"intel.com/svr-info/pkg/target"
)

func getUserPath() string {
	// get user's PATH environment variable, verify that it only contains paths (mitigate risk raised by Checkmarx)
	var verifiedPaths []string
	pathEnv := os.Getenv("PATH")
	pathEnvPaths := strings.Split(pathEnv, ":")
	for _, p := range pathEnvPaths {
		files, err := filepath.Glob(p)
		// Goal is to filter out any non path strings
		// Glob will throw an error on pattern mismatch and return no files if no files
		if err == nil && len(files) > 0 {
			verifiedPaths = append(verifiedPaths, p)
		}
	}
	return strings.Join(verifiedPaths, ":")
}

func runCommand(label string, command string, superuser bool, superuserPassword string, binPath string, timeout int) (stdout string, stderr string, exitCode int, err error) {
	// explicitly set PATH by pre-pending to command
	cmdWithPath := command
	if binPath != "" {
		path := getUserPath()
		newPath := fmt.Sprintf("%s%c%s", binPath, os.PathListSeparator, path)
		cmdWithPath = fmt.Sprintf("PATH=\"%s\" %s", newPath, cmdWithPath)
	}
	if superuser {
		return runSuperUserCommand(cmdWithPath, superuserPassword, timeout)
	}
	return runRegularUserCommand(cmdWithPath, timeout)
}

func runRegularUserCommand(command string, timeout int) (stdout string, stderr string, exitCode int, err error) {
	log.Printf("runRegularUserCommand Start: %s", command)
	defer log.Printf("runRegularUserCommand Finish: %s", command)
	return target.RunLocalCommandWithTimeout(exec.Command("bash", "-c", command), timeout)
}

func runSuperUserCommand(command string, sudoPassword string, timeout int) (stdout string, stderr string, exitCode int, err error) {
	// if running as root/super-user, run the command as is
	if os.Geteuid() == 0 {
		return runRegularUserCommand(command, timeout)
	}
	log.Printf("runSuperUserCommand Start: %s", command)
	defer log.Printf("runSuperUserCommand Finish: %s", command)
	// if sudo password was provided, send it to sudo via stdin
	if sudoPassword != "" {
		cmd := exec.Command("sudo", "-kSE", "bash", "-c", command)
		pwdNewline := fmt.Sprintf("%s\n", sudoPassword)
		return target.RunLocalCommandWithInputWithTimeout(cmd, pwdNewline, timeout)
	}
	// if password is not required for sudo, simply prepend 'sudo'
	cmd := exec.Command("sudo", "-kn", "ls")
	_, _, _, err = target.RunLocalCommandWithTimeout(cmd, timeout)
	if err == nil {
		cmd := exec.Command("sudo", "-E", "bash", "-c", command)
		return target.RunLocalCommandWithTimeout(cmd, timeout)
	}
	// no other options, fail
	err = fmt.Errorf("no option available to run command as super-user using sudo")
	return
}

func installMods(mods string, sudoPassword string) (installedMods []string) {
	if len(mods) > 0 {
		modList := strings.Split(mods, ",")
		for _, mod := range modList {
			log.Printf("Attempting to install kernel module: %s", mod)
			_, _, _, err := runSuperUserCommand(fmt.Sprintf("modprobe --first-time %s > /dev/null 2>&1", mod), sudoPassword, 0)
			if err == nil {
				log.Printf("Installed kernel module %s", mod)
				installedMods = append(installedMods, mod)
			}
		}
	}
	return installedMods
}

func uninstallMods(modList []string, sudoPassword string) (err error) {
	for _, mod := range modList {
		log.Printf("Uninstalling kernel module %s", mod)
		_, _, _, err = runSuperUserCommand(fmt.Sprintf("modprobe -r %s", mod), sudoPassword, 0)
		if err != nil {
			log.Printf("Error: %v", err)
			return
		}
	}
	return
}
