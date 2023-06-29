/*
Package core includes internal shared code.
*/
/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package core

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// ExpandUser expands '~' to user's home directory, if found, otherwise returns original path
func ExpandUser(path string) string {
	usr, _ := user.Current()
	if path == "~" {
		return usr.HomeDir
	} else if strings.HasPrefix(path, "~"+string(os.PathSeparator)) {
		return filepath.Join(usr.HomeDir, path[2:])
	} else {
		return path
	}
}

// AbsPath returns absolute path after expanding '~' to user's home dir
// Useful when application is started by a process that isn't a shell, e.g. PKB
// Use everywhere in place of filepath.Abs()
func AbsPath(path string) (string, error) {
	return filepath.Abs(ExpandUser(path))
}

// FileExists returns error if file does not exist or does exist but
// is not a file, i.e., is a directory
func FileExists(path string) (err error) {
	var fileInfo fs.FileInfo
	fileInfo, err = os.Stat(path)
	if err != nil {
		return
	} else {
		if !fileInfo.Mode().IsRegular() {
			err = fmt.Errorf("%s not a file", path)
			return
		}
	}
	return
}

// DirectoryExists returns error if directory does not exist or does exist but
// is not a directory, i.e., is a file
func DirectoryExists(path string) (err error) {
	var fileInfo fs.FileInfo
	fileInfo, err = os.Stat(path)
	if err != nil {
		return
	} else {
		if !fileInfo.Mode().IsDir() {
			err = fmt.Errorf("%s not a directory", path)
			return
		}
	}
	return
}
