/*
Package cpu provides a reference of CPU architectures and identification keys for known CPUS.
*/
/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package cpu

import (
	"fmt"
	"log"
	"os"
	"regexp"

	"gopkg.in/yaml.v2"
)

type CPUInfo struct {
	Architecture string `yaml:"architecture"`
	Family       string `yaml:"family"`
	Model        string `yaml:"model"`
	Stepping     string `yaml:"stepping"`
	Channels     int    `yaml:"channels"`
}

type CPU struct {
	configFilenames []string
	cpusInfo        []CPUInfo
}

func NewCPU(configFilenames []string) (cpu *CPU, err error) {
	cpu = &CPU{
		configFilenames: configFilenames,
		cpusInfo:        []CPUInfo{},
	}
	err = cpu.init()
	return
}

func (c *CPU) init() (err error) {
	for _, filename := range c.configFilenames {
		yamlBytes, err := os.ReadFile(filename)
		if err != nil {
			log.Printf("failed to read CPU info file: %s, %v", filename, err)
			continue
		}
		cpusInfo := []CPUInfo{}
		err = yaml.UnmarshalStrict(yamlBytes, &cpusInfo)
		if err != nil {
			log.Printf("failed to parse CPU info file: %s, %v", filename, err)
			continue
		}
		c.cpusInfo = append(c.cpusInfo, cpusInfo...)
	}
	return
}

func (c *CPU) getCPU(family, model, stepping string) (cpu CPUInfo, err error) {
	for _, info := range c.cpusInfo {
		// if family matches
		if info.Family == family {
			var reModel *regexp.Regexp
			reModel, err = regexp.Compile(info.Model)
			if err != nil {
				return
			}
			// if model matches
			if reModel.FindString(model) != "" {
				// if there is a stepping
				if info.Stepping != "" {
					var reStepping *regexp.Regexp
					reStepping, err = regexp.Compile(info.Stepping)
					if err != nil {
						return
					}
					// if stepping does NOT match
					if reStepping.FindString(stepping) == "" {
						// no match
						continue
					}
				}
				cpu = info
				return
			}
		}
	}
	err = fmt.Errorf("CPU match not found for family %s, model %s, stepping %s", family, model, stepping)
	return
}

func (c *CPU) GetMicroArchitecture(family, model, stepping string) (uarch string, err error) {
	cpu, err := c.getCPU(family, model, stepping)
	if err != nil {
		return
	}
	uarch = cpu.Architecture
	return
}

func (c *CPU) GetMemoryChannels(family, model, stepping string) (channels int, err error) {
	cpu, err := c.getCPU(family, model, stepping)
	if err != nil {
		return
	}
	channels = cpu.Channels
	return
}
