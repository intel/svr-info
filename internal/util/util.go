/*
Package util includes utility/helper functions that may be useful to other modules.
*/
/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package util

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

// FileExists returns whether the given file (not a directory) exists
func FileExists(path string) (exists bool, err error) {
	var fileInfo fs.FileInfo
	fileInfo, err = os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			exists = false
			err = nil
			return
		}
		return
	}
	if !fileInfo.Mode().IsRegular() {
		err = fmt.Errorf("%s not a file", path)
		return
	}
	exists = true
	return
}

// DirectoryExists returns whether the given directory (not a file) exists
func DirectoryExists(path string) (exists bool, err error) {
	var fileInfo fs.FileInfo
	fileInfo, err = os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			exists = false
			err = nil
			return
		}
		return
	}
	if !fileInfo.Mode().IsDir() {
		err = fmt.Errorf("%s not a directory", path)
		return
	}
	exists = true
	return
}

// StringIndexInList returns the index of the given string in the given list of
// strings and error if not found
func StringIndexInList(s string, l []string) (idx int, err error) {
	var item string
	for idx, item = range l {
		if item == s {
			return
		}
	}
	err = fmt.Errorf("%s not found in %s", s, strings.Join(l, ", "))
	return
}

// StringInList confirms if string is in list of strings
func StringInList(s string, l []string) bool {
	for _, item := range l {
		if item == s {
			return true
		}
	}
	return false
}
