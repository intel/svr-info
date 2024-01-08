/*
Package cpu provides a reference of CPU architectures and identification keys for known CPUS.
*/
/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package cpu

import (
	"embed"
	"fmt"
	"log"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v2"
)

//go:embed resources
var resources embed.FS

type CPUInfo struct {
	Architecture string `yaml:"architecture"`
	Family       string `yaml:"family"`
	Model        string `yaml:"model"`
	Stepping     string `yaml:"stepping"`
	Channels     int    `yaml:"channels"`
}

type CPU struct {
	cpusInfo []CPUInfo
}

func NewCPU() (cpu *CPU, err error) {
	yamlBytes, err := resources.ReadFile("resources/cpus.yaml")
	if err != nil {
		log.Printf("failed to read cpus.yaml: %v", err)
		return
	}
	cpu = &CPU{
		cpusInfo: []CPUInfo{},
	}
	err = yaml.UnmarshalStrict(yamlBytes, &cpu.cpusInfo)
	if err != nil {
		log.Printf("failed to parse cpus.yaml: %v", err)
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
			if reModel.FindString(model) == model {
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

func (c *CPU) GetMicroArchitecture(family, model, stepping, sockets, capid4, devices string) (uarch string, err error) {
	if family != "6" || (model != "143" && model != "207" && model != "173") {
		var cpu CPUInfo
		cpu, err = c.getCPU(family, model, stepping)
		if err != nil {
			return
		}
		uarch = cpu.Architecture
	} else { // SPR, EMR, GNR are special
		uarch, err = c.getMicroArchitectureExt(family, model, sockets, capid4, devices)
	}
	return
}

func (c *CPU) getMicroArchitectureExt(family, model, sockets string, capid4 string, devices string) (uarch string, err error) {
	if family != "6" || (model != "143" && model != "207" && model != "173") {
		err = fmt.Errorf("no extended architecture info for %s:%s", family, model)
		return
	}
	var bits int64
	if (model == "143" || model == "207") && capid4 != "" { // SPR and EMR
		var capid4Int int64
		capid4Int, err = strconv.ParseInt(capid4, 16, 64)
		if err != nil {
			return
		}
		bits = (capid4Int >> 6) & 0b11
	}
	if model == "143" { // SPR
		if bits == 3 {
			uarch = "SPR_XCC"
		} else if bits == 1 {
			uarch = "SPR_MCC"
		} else {
			uarch = "SPR"
		}
	} else if model == "207" { // EMR
		if bits == 3 {
			uarch = "EMR_XCC"
		} else if bits == 1 {
			uarch = "EMR_MCC"
		} else {
			uarch = "EMR"
		}
	} else if model == "173" { // GNR
		var devCount int
		devCount, err = strconv.Atoi(devices)
		if err != nil {
			return
		}
		var socketsCount int
		socketsCount, err = strconv.Atoi(sockets)
		if socketsCount == 0 || err != nil {
			return
		}
		ratio := devCount / socketsCount
		if ratio == 3 {
			uarch = "GNR_X1" // 1 die, GNR-SP HCC/LCC
		} else if ratio == 4 {
			uarch = "GNR_X2" // 2 dies, GNR-SP XCC
		} else if ratio == 5 {
			uarch = "GNR_X3" // 3 dies, GNR-AP UCC
		} else {
			uarch = "GNR"
		}
	}
	return
}

func (c *CPU) getCPUByUarch(uarch string) (cpu CPUInfo, err error) {
	if uarch == "" {
		err = fmt.Errorf("microarchitecture not provided")
		return
	}
	for _, info := range c.cpusInfo {
		var re *regexp.Regexp
		re, err = regexp.Compile(info.Architecture)
		if err != nil {
			return
		}
		if re.FindString(uarch) == "" {
			continue
		}
		cpu = info
		return
	}
	return
}

func (c *CPU) GetMemoryChannels(microarchitecture string) (channels int, err error) {
	cpu, err := c.getCPUByUarch(microarchitecture)
	if err != nil {
		return
	}
	channels = cpu.Channels
	return
}
