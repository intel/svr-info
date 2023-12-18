/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"fmt"
	"testing"
)

// helper
func isValid(arguments []string) bool {
	args := newCmdLineArgs()
	err := args.parse("tester", arguments)
	if err != nil {
		fmt.Printf("%v\n", err)
		return false
	}
	err = args.validate()
	if err != nil {
		fmt.Printf("%v\n", err)
	}
	return err == nil
}

func TestParseInvalidArgs(t *testing.T) {
	if isValid([]string{"-foo"}) {
		t.Fail()
	}
	if isValid([]string{"foo"}) {
		t.Fail()
	}
}

func TestParseNoArgs(t *testing.T) {
	if !isValid([]string{}) {
		t.Fail()
	}
}

func TestTooMuchAnalysis(t *testing.T) {
	if isValid([]string{"-analyze", "all", "-analyze_duration", "301"}) {
		t.Fail()
	}
	if isValid([]string{"-analyze", "all", "-analyze_frequency", "66"}) {
		t.Fail()
	}
}

func TestTooMuchProfiling(t *testing.T) {
	if isValid([]string{"-profile", "all", "-profile_duration", "302"}) {
		t.Fail()
	}
	if isValid([]string{"-profile", "all", "-profile_interval", "1", "-profile_duration", "200"}) {
		t.Fail()
	}
}

func TestFormat(t *testing.T) {
	if !isValid([]string{"-format", "all"}) {
		t.Fail()
	}
	if isValid([]string{"-format", "foo"}) {
		t.Fail()
	}
	if !isValid([]string{"-format", "txt,xlsx,html,json"}) {
		t.Fail()
	}
}

func TestAllExceptTargetsFile(t *testing.T) {
	args := []string{
		"-format", "all",
		"-benchmark", "all",
		"-storage_dir", "/tmp", // any dir
		"-analyze", "all",
		"-analyze_duration", "20",
		"-analyze_frequency", "22",
		"-profile", "all",
		"-profile_duration", "30",
		"-profile_interval", "3",
		"-temp", "/tmp", // any dir
		"-targettemp", "/tmp", // any dir
		"-output", "/tmp", // any dir
		"-megadata",
		"-debug",
		"-cmd_timeout", "150",
		"-printconfig",
		"-ip", "192.168.1.1",
		"-port", "20",
		"-user", "foo",
		"-key", "go.mod", // any file
	}
	if !isValid(args) {
		t.Fail()
	}
}

func TestHelp(t *testing.T) {
	if !isValid([]string{"-h"}) {
		t.Fail()
	}
}

func TestVersion(t *testing.T) {
	if !isValid([]string{"-v"}) {
		t.Fail()
	}
}

func TestIPAddressTooLong(t *testing.T) {
	b := make([]byte, 256)
	for i := range b {
		b[i] = 'x'
	}
	if isValid([]string{"-ip", string(b), "-user", "foo"}) {
		t.Fail()
	}
}

func TestIPNoUser(t *testing.T) {
	if isValid(([]string{"-ip", "192.168.1.1"})) {
		t.Fail()
	}
}

func TestUserNoIp(t *testing.T) {
	if isValid(([]string{"-user", "foo"})) {
		t.Fail()
	}
}

func TestTargetsFile(t *testing.T) {
	if !isValid(([]string{"-targets", "targets.example"})) {
		t.Fail()
	}
}

func TestKeyFile(t *testing.T) {
	if !isValid(([]string{"-key", "targets.example", "-ip", "192.168.1.1", "-user", "foo"})) { // any file will do
		t.Fail()
	}
}

func TestKeyNoIpUser(t *testing.T) {
	if isValid(([]string{"-key", "targets.example"})) {
		t.Fail()
	}
}

func TestPortNoIpUser(t *testing.T) {
	if isValid(([]string{"-port", "2022"})) {
		t.Fail()
	}
}
