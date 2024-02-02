/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"strings"
	"testing"
)

func TestParseAllFields(t *testing.T) {
	content := `
	label:ip:22:user:targets.example:sshpassword:sudopassword # ignored comment
	`
	tf := newTargetsFile("testing")
	targets, err := tf.parseContent([]byte(content))
	if err != nil {
		t.Fail()
	}
	if len(targets) != 1 {
		t.Fail()
	}
	if targets[0].label != "label" {
		t.Fail()
	}
	if targets[0].ip != "ip" {
		t.Fail()
	}
	if targets[0].port != "22" {
		t.Fail()
	}
	if targets[0].user != "user" {
		t.Fail()
	}
	if targets[0].key != "targets.example" {
		t.Fail()
	}
	if targets[0].pwd != "sshpassword" {
		t.Fail()
	}
	if targets[0].sudo != "sudopassword" {
		t.Fail()
	}
}

func TestParseNoLabel(t *testing.T) {
	content := `
	ip:22:user:targets.example:sshpassword:sudopassword
	`
	tf := newTargetsFile("testing")
	targets, err := tf.parseContent([]byte(content))
	if err != nil {
		t.Fail()
	}
	if len(targets) != 1 {
		t.Fail()
	}
	if targets[0].label != "ip" {
		t.Fail()
	}
	if targets[0].ip != "ip" {
		t.Fail()
	}
	if targets[0].port != "22" {
		t.Fail()
	}
	if targets[0].user != "user" {
		t.Fail()
	}
	if targets[0].key != "targets.example" {
		t.Fail()
	}
	if targets[0].pwd != "sshpassword" {
		t.Fail()
	}
	if targets[0].sudo != "sudopassword" {
		t.Fail()
	}
}

func TestParseMultiLine(t *testing.T) {
	content := `

	# this is a commented line
	label:ip::user:targets.example:sshpassword:sudopassword
	#label:ip:port:user::sshpassword:sudopassword # trailing comment
	label:ip:22:user::sshpassword:sudopassword
	# another commented line
	
	`
	tf := newTargetsFile("testing")
	targets, err := tf.parseContent([]byte(content))
	if err != nil {
		t.Fail()
	}
	if len(targets) != 2 {
		t.Fail()
	}
	if targets[0].label != "label" {
		t.Fail()
	}
	if targets[0].ip != "ip" {
		t.Fail()
	}
	if targets[0].port != "" {
		t.Fail()
	}
	if targets[0].user != "user" {
		t.Fail()
	}
	if targets[0].key != "targets.example" {
		t.Fail()
	}
	if targets[0].pwd != "sshpassword" {
		t.Fail()
	}
	if targets[0].sudo != "sudopassword" {
		t.Fail()
	}
}

func TestParseEmpty(t *testing.T) {
	content := ""
	tf := newTargetsFile("testing")
	targets, err := tf.parseContent([]byte(content))
	if err != nil {
		t.Fail()
	}
	if len(targets) != 0 {
		t.Fail()
	}
}

func TestParseAllComments(t *testing.T) {
	content := `
	# ip:22:user::sshpassword:sudopassword # comment
	# foo
	
	`
	tf := newTargetsFile("testing")
	targets, err := tf.parseContent([]byte(content))
	if err != nil {
		t.Fail()
	}
	if len(targets) != 0 {
		t.Fail()
	}
}

func TestParseMissingFields(t *testing.T) {
	content := `
	# valid line
	label:ip:22:user::sshpassword:sudopassword # comment
	# invalid line
	ip:22:user:key:sshpassword
	`
	tf := newTargetsFile("testing")
	_, err := tf.parseContent([]byte(content))
	if err == nil {
		t.Fail()
	}
	if !strings.Contains(err.Error(), "format error, line 5") {
		t.Fail()
	}
}

func TestParseInvalidPort(t *testing.T) {
	content := "ip:invalid_port:user::sshpassword:sudopassword"
	tf := newTargetsFile("testing")
	_, err := tf.parseContent([]byte(content))
	if err == nil {
		t.Fail()
	}
	if !strings.Contains(err.Error(), "invalid port invalid_port, line 1") {
		t.Fail()
	}
}

func TestMissingIpAndUser(t *testing.T) {
	content := ":22::targets.example:sshpassword:sudopassword"
	tf := newTargetsFile("testing")
	_, err := tf.parseContent([]byte(content))
	if err == nil {
		t.Fail()
	}
	if !strings.Contains(err.Error(), "user name is required, line 1") {
		t.Fail()
	}
	if !strings.Contains(err.Error(), "IP Address (or hostname) is required, line 1") {
		t.Fail()
	}
}

func TestEscapeSudo(t *testing.T) {
	content := "ip::user:::$foo$bar"
	tf := newTargetsFile("testing")
	targets, err := tf.parseContent([]byte(content))
	if err != nil {
		t.Fail()
	}
	if targets[0].sudo != "\\$foo\\$bar" {
		t.Fail()
	}
}
