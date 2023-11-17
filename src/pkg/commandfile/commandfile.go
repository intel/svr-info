/*
Package commandfile provides common interface to collector input file
*/
/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package commandfile

import "github.com/creasty/defaults"

type Command struct {
	Label     string `yaml:"label"`
	Command   string `yaml:"command"`
	Modprobe  string `yaml:"modprobe"`
	Superuser bool   `default:"false" yaml:"superuser"`
	Run       bool   `default:"false" yaml:"run"`
	Parallel  bool   `default:"false" yaml:"parallel"`
}

type Arguments struct {
	Name    string `default:"test" yaml:"name"`
	Binpath string `default:"." yaml:"bin_path"`
	Timeout int    `default:"300" yaml:"command_timeout"`
}

type CommandFile struct {
	Args     Arguments `yaml:"arguments"`
	Commands []Command `yaml:"commands"`
}

func (s *Arguments) UnmarshalYAML(unmarshal func(interface{}) error) error {
	defaults.Set(s)
	type plain Arguments
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	return nil
}

func (s *Command) UnmarshalYAML(unmarshal func(interface{}) error) error {
	defaults.Set(s)
	type plain Command
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	return nil
}
