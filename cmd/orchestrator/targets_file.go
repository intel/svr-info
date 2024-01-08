/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/intel/svr-info/internal/core"
)

type targetFromFile struct {
	label  string
	ip     string
	port   string
	user   string
	key    string
	pwd    string
	sudo   string
	lineNo int
}

type TargetsFile struct {
	path string
}

func newTargetsFile(path string) *TargetsFile {
	return &TargetsFile{path: path}
}

func (tf *TargetsFile) parse() (targets []targetFromFile, err error) {
	content, err := os.ReadFile(tf.path)
	if err != nil {
		return
	}
	return tf.parseContent(content)
}

func (tf *TargetsFile) parseContent(content []byte) (targets []targetFromFile, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	var fileErrors []string
	for scanner.Scan() {
		lineNo += 1
		line := scanner.Text()
		line = strings.Split(line, "#")[0] // strip trailing comment
		line = strings.TrimSpace(line)
		// skip blank and commented lines
		if line == "" || line[0] == '#' {
			continue
		}
		tokens := strings.Split(line, ":")
		var t targetFromFile
		if len(tokens) != 6 && len(tokens) != 7 {
			fileErrors = append(fileErrors, fmt.Sprintf("-targets %s : format error, line %d\n", tf.path, lineNo))
		} else {
			i := 0
			t.lineNo = lineNo
			t.label = tokens[0]
			if len(tokens) == 7 {
				i++
			}
			t.ip = tokens[i]
			// ip is required
			if t.ip == "" {
				fileErrors = append(fileErrors, fmt.Sprintf("-targets %s : IP Address (or hostname) is required, line %d\n", tf.path, lineNo))
			}
			// port is optional, but must be an integer if provided
			t.port = tokens[i+1]
			if t.port != "" {
				_, err := strconv.Atoi(t.port)
				if err != nil {
					fileErrors = append(fileErrors, fmt.Sprintf("-targets %s : invalid port %s, line %d\n", tf.path, t.port, lineNo))
				}
			}
			// user is required
			t.user = tokens[i+2]
			if t.user == "" {
				fileErrors = append(fileErrors, fmt.Sprintf("-targets %s : user name is required, line %d\n", tf.path, lineNo))
			}
			// key, pwd, and sudo are all optional
			t.key = tokens[i+3]
			if t.key != "" {
				err = core.FileExists(t.key)
				if err != nil {
					fileErrors = append(fileErrors, fmt.Sprintf("-targets %s : key file (%s) not a file, line %d\n", tf.path, t.key, lineNo))
					return
				}
			}
			t.pwd = tokens[i+4]
			t.sudo = tokens[i+5]
			t.sudo = strings.ReplaceAll(t.sudo, "$", "\\$") // escape $ in sudo password
			targets = append(targets, t)
		}
	}
	if len(fileErrors) > 0 {
		err = fmt.Errorf("%s", strings.Join(fileErrors, "\n"))
		return
	}
	return
}
