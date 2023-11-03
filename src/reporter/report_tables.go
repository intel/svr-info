/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
/* functions for creating tables used in reports */

package main

import (
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hyperjumptech/grule-rule-engine/ast"
	"github.com/hyperjumptech/grule-rule-engine/builder"
	"github.com/hyperjumptech/grule-rule-engine/engine"
	"github.com/hyperjumptech/grule-rule-engine/pkg"
	"gopkg.in/yaml.v2"
	"intel.com/svr-info/pkg/cpu"
)

/* a note about functions that define tables...
 * - "Brief" and "Summary" in the function name have meaning...see examples below
 * - Avoid duplicating any of the parsing logic present in the full table when creating Brief or Summary tables
 * - Avoid duplicating ...............................when creating tables, in general.
 *  Examples:
 *   memoryTable() - the full table
 *   memoryBriefTable() - a table used in the "Brief" report that has a reduced number of fields compared to the full table
 *   nicTable() - the full table
 *   nicSummaryTable() - has info derived from the full table, but is presented in summary format
 */

func newMarketingClaimTable(fullReport *Report, tableNicSummary *Table, tableDiskSummary *Table, tableAcceleratorSummary *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Marketing Claim",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	// BASELINE: 1-node, 2x Intel® Xeon® <SKU, processor>, xx cores, HT On/Off?, Turbo On/Off?, NUMA xxx,  Integrated Accelerators Available [used]: xxx, Total Memory xxx GB (xx slots/ xx GB/ xxxx MHz [run @ xxxx MHz] ), <BIOS version>, <ucode version>, <OS Version>, <kernel version>, WORKLOAD+VERSION, COMPILER, LIBRARIES, OTHER_SW, score=?UNITS.\nTest by COMPANY as of <mm/dd/yy>.
	template := "1-node, %sx %s, %s cores, HT %s, Turbo %s, NUMA %s, Integrated Accelerators Available [used]: %s, Total Memory %s, BIOS %s, microcode %s, %s, %s, %s, %s, WORKLOAD+VERSION, COMPILER, LIBRARIES, OTHER_SW, score=?UNITS.\nTest by COMPANY as of %s."
	var date, socketCount, cpuModel, coreCount, htOnOff, turboOnOff, numaNodes, installedMem, biosVersion, uCodeVersion, nics, disks, operatingSystem, kernelVersion string

	for sourceIdx, source := range fullReport.Sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"System Summary",
			},
			Values: [][]string{},
		}
		date = strings.TrimSpace(source.getCommandOutput("date"))
		socketCount, _ = fullReport.findTable("CPU").getValue(sourceIdx, "Sockets")
		cpuModel, _ = fullReport.findTable("CPU").getValue(sourceIdx, "CPU Model")
		coreCount, _ = fullReport.findTable("CPU").getValue(sourceIdx, "Cores per Socket")
		hyperthreading, _ := fullReport.findTable("CPU").getValue(sourceIdx, "Hyperthreading")
		if hyperthreading == "Enabled" {
			htOnOff = "On"
		} else if hyperthreading == "Disabled" {
			htOnOff = "Off"
		} else {
			htOnOff = "?"
		}
		turboEnabledDisabled, _ := fullReport.findTable("CPU").getValue(sourceIdx, "Intel Turbo Boost")
		if strings.Contains(strings.ToLower(turboEnabledDisabled), "enabled") {
			turboOnOff = "On"
		} else if strings.Contains(strings.ToLower(turboEnabledDisabled), "disabled") {
			turboOnOff = "Off"
		} else {
			turboOnOff = "?"
		}
		numaNodes, _ = fullReport.findTable("CPU").getValue(sourceIdx, "NUMA Nodes")
		accelerators, _ := tableAcceleratorSummary.getValue(sourceIdx, "Accelerators Available [used]")
		installedMem, _ = fullReport.findTable("Memory").getValue(sourceIdx, "Installed Memory")
		biosVersion, _ = fullReport.findTable("BIOS").getValue(sourceIdx, "Version")
		uCodeVersion, _ = fullReport.findTable("Operating System").getValue(sourceIdx, "Microcode")
		nics, _ = tableNicSummary.getValue(sourceIdx, "NIC")
		disks, _ = tableDiskSummary.getValue(sourceIdx, "Disk")
		operatingSystem, _ = fullReport.findTable("Operating System").getValue(sourceIdx, "OS")
		kernelVersion, _ = fullReport.findTable("Operating System").getValue(sourceIdx, "Kernel")
		claim := fmt.Sprintf(template, socketCount, cpuModel, coreCount, htOnOff, turboOnOff, numaNodes, accelerators, installedMem, biosVersion, uCodeVersion, nics, disks, operatingSystem, kernelVersion, date)
		hostValues.Values = append(hostValues.Values, []string{claim})
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newMemoryNUMABandwidthTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Memory NUMA Bandwidth",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	/* MLC Output:
			Numa node
	Numa node	     0	     1
	       0	175610.3	55579.7
	       1	55575.2	175656.7
	*/
	/* table :
	Node |   Bandwidths
	0    |  val,val1,val...,valn
	1    |  val,val1,val...,valn
	...  |  val,val1,val...,valn
	N    |  val,val1,val...,valn
	*/
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Node",
				"Bandwidths 0-N",
			},
			Values: [][]string{},
		}
		nodeBandwidthsPairs := source.valsArrayFromRegexSubmatch("Memory MLC Bandwidth", `^(\d)\s+(\d.*)`)
		for _, nodeBandwidthsPair := range nodeBandwidthsPairs {
			bandwidths := strings.Split(strings.TrimSpace(nodeBandwidthsPair[1]), "\t")
			hostValues.Values = append(hostValues.Values, []string{nodeBandwidthsPair[0], strings.Join(bandwidths, ",")})
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newMemoryBandwidthLatencyTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Memory Bandwidth and Latency",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	/* MLC Output:
	Inject	Latency	Bandwidth
	Delay	(ns)	MB/sec
	==========================
	 00000	261.65	 225060.9
	 00002	261.63	 225040.5
	 00008	261.54	 225073.3
	 ...
	*/
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Latency (ns)",
				"Bandwidth (GB/s)",
			},
			Values: [][]string{},
		}
		latencyBandwidthPairs := source.valsArrayFromRegexSubmatch("Memory MLC Loaded Latency Test", `^[0-9]*\s*([0-9]*\.[0-9]+)\s*([0-9]*\.[0-9]+)`)
		for _, latencyBandwidth := range latencyBandwidthPairs {
			latency := latencyBandwidth[0]
			bandwidth, err := strconv.ParseFloat(latencyBandwidth[1], 32)
			if err != nil {
				log.Printf("Unable to convert bandwidth to float: %s", latencyBandwidth[1])
				continue
			}
			bandwidth = bandwidth / 1000
			// insert into beginning of array (reverse order)
			vals := []string{latency, fmt.Sprintf("%.1f", bandwidth)}
			hostValues.Values = append([][]string{vals}, hostValues.Values...)
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newNetworkIRQTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Network IRQ Mapping",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name:       source.getHostname(),
			ValueNames: []string{"Interface", "CPU:IRQs CPU:IRQs ..."},
			Values:     [][]string{},
		}
		nics := source.valsFromRegexSubmatch("lshw", `^pci.*? (\S+)\s+network\s+\S.*?\s+\[\w+:\w+]$`)
		nics = append(nics, source.valsFromRegexSubmatch("lshw", `^usb.*? (\S+)\s+network\s+\S.*?$`)...)
		for _, nic := range nics {
			cmdout := source.valFromOutputRegexSubmatch("nic info", fmt.Sprintf(`CPU AFFINITY %s: (.*)\n`, nic))
			// command output is formatted like this: 200:1;201:1-17,36-53;202:44
			// which is <irq>:<cpu(s)>
			// we need to reverse it to <cpu>:<irq(s)>
			cpuIRQMappings := make(map[int][]int)
			irqCPUPairs := strings.Split(cmdout, ";")
			for _, pair := range irqCPUPairs {
				if pair == "" {
					continue
				}
				tokens := strings.Split(pair, ":")
				irq, err := strconv.Atoi(tokens[0])
				if err != nil {
					continue
				}
				cpuList := tokens[1]
				cpus := expandCPUList(cpuList)
				for _, cpu := range cpus {
					cpuIRQMappings[cpu] = append(cpuIRQMappings[cpu], irq)
				}
			}
			var val string
			var cpuIRQs []string
			var cpus []int
			for k := range cpuIRQMappings {
				cpus = append(cpus, k)
			}
			sort.Ints(cpus)
			for _, cpu := range cpus {
				irqs := cpuIRQMappings[cpu]
				cpuIRQ := fmt.Sprintf("%d:", cpu)
				var irqStrings []string
				for _, irq := range irqs {
					irqStrings = append(irqStrings, fmt.Sprintf("%d", irq))
				}
				cpuIRQ += strings.Join(irqStrings, ",")
				cpuIRQs = append(cpuIRQs, cpuIRQ)
			}
			val = strings.Join(cpuIRQs, " ")
			hostValues.Values = append(hostValues.Values, []string{nic, val})
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newFrequencyTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Core Frequency",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Core Count",
				"Spec Frequency (GHz)",
				"Measured Frequency (GHz)",
			},
			Values: [][]string{},
		}
		type freq struct {
			spec     float64
			measured float64
		}
		vals := make(map[int]freq) // map core count to spec/measured frequencies

		// get measured frequencies (these are optionally collected)
		matches := source.valsArrayFromRegexSubmatch("Measure Turbo Frequencies", `^(\d+)-core turbo\s+(\d+) MHz`)
		for _, countFreq := range matches {
			mhz, err := strconv.Atoi(countFreq[1])
			if err != nil {
				log.Print(err)
				return
			}
			ghz := math.Round(float64(mhz)/100.0) / 10
			count, err := strconv.Atoi(countFreq[0])
			if err != nil {
				log.Print(err)
				return
			}
			vals[count] = freq{}
			x := vals[count]
			x.measured = ghz
			vals[count] = x
		}
		// get spec frequencies (these also may not be present)
		countFreqs, err := source.getSpecCountFrequencies()
		if err != nil {
			log.Print(err)
		} else {
			// fill in gaps in sparse list...
			// go through list in reverse order so we can fill previous slots with same frequency
			for i := len(countFreqs) - 1; i >= 0; i-- {
				countFreq := countFreqs[i]
				count, _ := strconv.Atoi(countFreq[0])
				ghz, _ := strconv.ParseFloat(countFreq[1], 64)
				for j := count; j > 0; j-- {
					if _, ok := vals[j]; !ok {
						vals[j] = freq{}
					}
					x := vals[j]
					x.spec = ghz
					vals[j] = x
				}
			}
		}
		// need the vals in order (by core count), so get and sort the keys
		var valKeys []int
		for k := range vals {
			valKeys = append(valKeys, k)
		}
		sort.Ints(valKeys)
		// now go through the vals in sorted order
		for _, k := range valKeys {
			var count, spec, measured string
			count = fmt.Sprintf("%d", k)
			if vals[k].spec != 0 {
				spec = fmt.Sprintf("%.1f", vals[k].spec)
			}
			if vals[k].measured != 0 {
				measured = fmt.Sprintf("%.1f", vals[k].measured)
			}
			hostValues.Values = append(hostValues.Values, []string{count, spec, measured})
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newHostTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Host",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Name",
				"Time",
			},
			Values: [][]string{
				{
					source.valFromRegexSubmatch("uname -a", `^Linux (\S+) \S+`),
					source.valFromRegexSubmatch("date -u", `^(.*UTC\s*[0-9]*)$`),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newOperatingSystemTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Operating System",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"OS",
				"Kernel",
				"Boot Parameters",
				"Microcode",
			},
			Values: [][]string{
				{
					source.getOperatingSystem(),
					source.valFromRegexSubmatch("uname -a", `^Linux \S+ (\S+)`),
					source.getCommandOutputLine("/proc/cmdline"),
					source.valFromRegexSubmatch("/proc/cpuinfo", `^microcode.*:\s*(.+?)$`),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newOperatingSystemBriefTable(tableOS *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "OS",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	copyValues(tableOS, table, []string{
		"Microcode",
		"OS",
		"Kernel",
	})
	for i := range table.AllHostValues {
		table.AllHostValues[i].Name = tableOS.AllHostValues[i].Name
	}
	return
}

func newSystemTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "System",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Manufacturer",
				"Product Name",
				"Version",
				"Serial #",
				"UUID",
			},
			Values: [][]string{
				{
					source.valFromDmiDecodeRegexSubmatch("1", `^Manufacturer:\s*(.+?)$`),
					source.valFromDmiDecodeRegexSubmatch("1", `^Product Name:\s*(.+?)$`),
					source.valFromDmiDecodeRegexSubmatch("1", `^Version:\s*(.+?)$`),
					source.valFromDmiDecodeRegexSubmatch("1", `^Serial Number:\s*(.+?)$`),
					source.valFromDmiDecodeRegexSubmatch("1", `^UUID:\s*(.+?)$`),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newSystemSummaryTable(tableSystem *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "System",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, srcHv := range tableSystem.AllHostValues {
		mfgIndex, err := findValueIndex(&srcHv, "Manufacturer")
		if err != nil {
			log.Panicf("Did not find Manufacturer field in table.")
		}
		nameIndex, err := findValueIndex(&srcHv, "Product Name")
		if err != nil {
			log.Panicf("Did not find Product Name field in table.")
		}
		var hostValues = HostValues{
			Name: srcHv.Name,
			ValueNames: []string{
				"System",
			},
			Values: [][]string{
				{
					strings.Join([]string{
						srcHv.Values[0][mfgIndex],
						srcHv.Values[0][nameIndex],
					}, " "),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newChassisTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Chassis",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Manufacturer",
				"Type",
				"Version",
				"Serial #",
			},
			Values: [][]string{
				{
					source.valFromDmiDecodeRegexSubmatch("3", `^Manufacturer:\s*(.+?)$`),
					source.valFromDmiDecodeRegexSubmatch("3", `^Type:\s*(.+?)$`),
					source.valFromDmiDecodeRegexSubmatch("3", `^Version:\s*(.+?)$`),
					source.valFromDmiDecodeRegexSubmatch("3", `^Serial Number:\s*(.+?)$`),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newChassisSummaryTable(tableChassis *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Chassis",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, srcHv := range tableChassis.AllHostValues {
		mfgIndex, err := findValueIndex(&srcHv, "Manufacturer")
		if err != nil {
			log.Panicf("Did not find Manufacturer field in table.")
		}
		typeIndex, err := findValueIndex(&srcHv, "Type")
		if err != nil {
			log.Panicf("Did not find Type field in table.")
		}
		var hostValues = HostValues{
			Name: srcHv.Name,
			ValueNames: []string{
				"Chassis",
			},
			Values: [][]string{
				{
					strings.Join([]string{
						srcHv.Values[0][mfgIndex],
						srcHv.Values[0][typeIndex],
					}, " "),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newBIOSTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "BIOS",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Vendor",
				"Version",
				"Release Date",
			},
			Values: [][]string{
				{
					source.valFromDmiDecodeRegexSubmatch("0", `^Vendor:\s*(.+?)$`),       // BIOS
					source.valFromDmiDecodeRegexSubmatch("0", `^Version:\s*(.+?)$`),      // BIOS
					source.valFromDmiDecodeRegexSubmatch("0", `^Release Date:\s*(.+?)$`), // BIOS
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newBIOSSummaryTable(tableBIOS *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "BIOS",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, srcHv := range tableBIOS.AllHostValues {
		versionIndex, err := findValueIndex(&srcHv, "Version")
		if err != nil {
			log.Panicf("Did not find Version field.")
		}
		var hostValues = HostValues{
			Name: srcHv.Name,
			ValueNames: []string{
				"BIOS",
			},
			Values: [][]string{
				{srcHv.Values[0][versionIndex]},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newBaseboardTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Baseboard",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Manufacturer",
				"Product Name",
				"Version",
				"Serial #",
			},
			Values: [][]string{
				{
					source.valFromDmiDecodeRegexSubmatch("2", `^Manufacturer:\s*(.+?)$`),  // Baseboard
					source.valFromDmiDecodeRegexSubmatch("2", `^Product Name:\s*(.+?)$`),  // Baseboard
					source.valFromDmiDecodeRegexSubmatch("2", `^Version:\s*(.+?)$`),       // Baseboard
					source.valFromDmiDecodeRegexSubmatch("2", `^Serial Number:\s*(.+?)$`), // Baseboard
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}
func newBaseboardSummaryTable(tableBaseboard *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Baseboard",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, srcHv := range tableBaseboard.AllHostValues {
		mfgIndex, err := findValueIndex(&srcHv, "Manufacturer")
		if err != nil {
			log.Panicf("Did not find Manufacturer field in Baseboard table.")
		}
		nameIndex, err := findValueIndex(&srcHv, "Product Name")
		if err != nil {
			log.Panicf("Did not find Product Name field in Baseboard table.")
		}
		var hostValues = HostValues{
			Name: srcHv.Name,
			ValueNames: []string{
				"Baseboard",
			},
			Values: [][]string{
				{
					strings.Join([]string{
						srcHv.Values[0][mfgIndex],
						srcHv.Values[0][nameIndex],
					}, " "),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newSoftwareTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Software Version",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"GCC",
				"GLIBC",
				"Binutils",
				"Python",
				"Python3",
				"Java",
				"OpenSSL",
			},
			Values: [][]string{
				{
					source.valFromRegexSubmatch("gcc version", `^(gcc .*)$`),
					source.valFromRegexSubmatch("glibc version", `^(ldd .*)`),
					source.valFromRegexSubmatch("binutils version", `^(GNU ld .*)$`),
					source.valFromRegexSubmatch("python version", `^(Python .*)$`),
					source.valFromRegexSubmatch("python3 version", `^(Python 3.*)$`),
					source.valFromRegexSubmatch("java version", `^(openjdk .*)$`),
					source.valFromRegexSubmatch("openssl version", `^(OpenSSL .*)$`),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newUncoreTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Uncore",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"CHA Count",
				"Minimum Frequency",
				"Maximum Frequency",
				"Active Idle Frequency",
				"Active Idle Utilization Point",
			},
			Values: [][]string{
				{
					source.getCHACount(),
					source.getUncoreMinFrequency(),
					source.getUncoreMaxFrequency(),
					source.getActiveIdleFrequency(),
					source.getActiveIdleUtilizationPoint(),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newCPUTable(sources []*Source, cpusInfo *cpu.CPU, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "CPU",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		family := source.valFromRegexSubmatch("lscpu", `^CPU family.*:\s*([0-9]+)$`)
		model := source.valFromRegexSubmatch("lscpu", `^Model.*:\s*([0-9]+)$`)
		stepping := source.valFromRegexSubmatch("lscpu", `^Stepping.*:\s*(.+)$`)
		sockets := source.valFromRegexSubmatch("lscpu", `^Socket\(.*:\s*(.+?)$`)
		capid4 := source.valFromRegexSubmatch("lspci bits", `^([0-9a-fA-F]+)`)
		devices := source.valFromRegexSubmatch("lspci devices", `^([0-9]+)`)
		coresPerSocket := source.valFromRegexSubmatch("lscpu", `^Core\(s\) per socket.*:\s*(.+?)$`)
		microarchitecture := getMicroArchitecture(cpusInfo, family, model, stepping, capid4, devices, sockets)
		channelCount, err := cpusInfo.GetMemoryChannels(family, model, stepping)
		channels := fmt.Sprintf("%d", channelCount)
		if err != nil {
			channels = "Unknown"
		}
		virtualization := source.valFromRegexSubmatch("lscpu", `^Virtualization.*:\s*(.+?)$`)
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"CPU Model",
				"Architecture",
				"Microarchitecture",
				"Family",
				"Model",
				"Stepping",
				"Base Frequency",
				"Maximum Frequency",
				"All-core Maximum Frequency",
				"CPUs",
				"On-line CPU List",
				"Hyperthreading",
				"Cores per Socket",
				"Sockets",
				"NUMA Nodes",
				"NUMA CPU List",
				"L1d Cache",
				"L1i Cache",
				"L2 Cache",
				"L3 Cache",
				"L3 per Core",
				"Memory Channels",
				"Prefetchers",
				"Intel Turbo Boost",
				"Virtualization",
				"PPINs",
			},
			Values: [][]string{
				{
					source.valFromRegexSubmatch("lscpu", `^[Mm]odel name.*:\s*(.+?)$`),
					source.valFromRegexSubmatch("lscpu", `^Architecture.*:\s*(.+)$`),
					microarchitecture,
					family,
					model,
					stepping,
					source.getBaseFrequency(),
					source.getMaxFrequency(),
					source.getAllCoreMaxFrequency(),
					source.valFromRegexSubmatch("lscpu", `^CPU\(.*:\s*(.+?)$`),
					source.valFromRegexSubmatch("lscpu", `^On-line CPU.*:\s*(.+?)$`),
					source.getHyperthreading(),
					coresPerSocket,
					sockets,
					source.valFromRegexSubmatch("lscpu", `^NUMA node\(.*:\s*(.+?)$`),
					source.getNUMACPUList(),
					source.valFromRegexSubmatch("lscpu", `^L1d cache.*:\s*(.+?)$`),
					source.valFromRegexSubmatch("lscpu", `^L1i cache.*:\s*(.+?)$`),
					source.valFromRegexSubmatch("lscpu", `^L2 cache.*:\s*(.+?)$`),
					source.getL3(microarchitecture),
					source.getL3PerCore(microarchitecture, coresPerSocket, sockets, virtualization),
					channels,
					source.getPrefetchers(),
					source.getTurboEnabled(family),
					virtualization,
					source.getPPINs(),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newCPUBriefTable(tableCPU *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "CPU",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	copyValues(tableCPU, table, []string{
		"CPU Model",
		"Microarchitecture",
		"Sockets",
		"Cores per Socket",
		"Hyperthreading",
		"CPUs",
		"Intel Turbo Boost",
		"Base Frequency",
		"All-core Maximum Frequency",
		"Maximum Frequency",
		"NUMA Nodes",
		"Prefetchers",
		"PPINs",
	})
	for i := range table.AllHostValues {
		table.AllHostValues[i].Name = tableCPU.AllHostValues[i].Name
	}
	return
}

func newISATable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "ISA",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	type ISA struct {
		Name     string
		FullName string
		CPUID    string
		lscpu    string
	}
	isas := []ISA{
		{"AES", "Advanced Encryption Standard New Instructions (AES-NI)", "AES instruction", "aes"},
		{"AMX", "Advanced Matrix Extensions", "AMX-BF16: tile bfloat16 support", "amx_bf16"},
		{"AVX512F", "AVX-512 Foundation", "AVX512F: AVX-512 foundation instructions", "avx512f"},
		{"AVX512_BF16", "Vector Neural Network Instructions - BF16", "AVX512_BF16: bfloat16 instructions", "avx512_bf16"},
		{"AVX512_FP16", "Advanced Vector Extensions 512 - FP16", "AVX512_FP16: fp16 support", "avx512_fp16"},
		{"AVX512_VNNI", "Vector Neural Network Instructions", "AVX512_VNNI: neural network instructions", "avx512_vnni"},
		{"CLDEMOTE", "Cache Line Demote", "CLDEMOTE supports cache line demote", "cldemote"},
		{"ENQCMD", "Enqueue Command Instruction", "ENQCMD instruction", "enqcmd"},
		{"SERIALIZE", "SERIALIZE Instruction", "SERIALIZE instruction", "serialize"},
		{"TSXLDTRK", "Transactional Synchronization Extensions", "TSXLDTRK: TSX suspend load addr tracking", "tsxldtrk"},
		{"VAES", "Vector AES", "VAES instructions", "vaes"},
		{"WAITPKG", "UMONITOR, UMWAIT, TPAUSE Instructions", "WAITPKG instructions", "waitpkg"},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Name",
				"Full Name",
				"CPU Support",
				"Kernel Support",
			},
		}
		flags := source.valFromRegexSubmatch("lscpu", `^Flags.*:\s*(.*)$`)
		for _, isa := range isas {
			cpuSupport := yesIfTrue(source.valFromRegexSubmatch("cpuid -1", isa.CPUID+`\s*= (.+?)$`))
			kernelSupport := "Yes"
			match, err := regexp.MatchString(" "+isa.lscpu+" ", flags)
			if err != nil {
				log.Printf("regex match failed: %v", err)
				return
			}
			if !match {
				kernelSupport = "No"
			}
			hostValues.Values = append(hostValues.Values, []string{isa.Name, isa.FullName, cpuSupport, kernelSupport})
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newAcceleratorTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Accelerator",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	type Accelerator struct {
		MfgID       string `yaml:"mfgid"`
		DevID       string `yaml:"devid"`
		Name        string `yaml:"name"`
		FullName    string `yaml:"full_name"`
		Description string `yaml:"description"`
	}
	var accelDefs []Accelerator
	// load accelerator info from YAML
	yamlBytes, err := resources.ReadFile("resources/accelerators.yaml")
	if err != nil {
		log.Printf("failed to read accelerators.yaml: %v", err)
		return
	}
	err = yaml.UnmarshalStrict(yamlBytes, &accelDefs)
	if err != nil {
		log.Printf("failed to parse accelerators.yaml: %v", err)
		return
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Name",
				"Count",
				"Work Queues",
				"Full Name",
				"Description",
			},
			Values: [][]string{},
		}
		for _, accelDef := range accelDefs {
			hostValues.Values = append(hostValues.Values, []string{accelDef.Name, source.getAcceleratorCount(accelDef.MfgID, accelDef.DevID), source.getAcceleratorQueues(accelDef.Name), accelDef.FullName, accelDef.Description})
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newAcceleratorSummaryTable(tableAccelerator *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Accelerator",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, hv := range tableAccelerator.AllHostValues {
		var summaryParts []string
		for _, rowValues := range hv.Values {
			accelName := rowValues[0]
			accelCount := rowValues[1]
			if strings.Contains(accelName, "chipset") { // skip "QAT (on chipset)" in this table
				continue
			} else if strings.Contains(accelName, "CPU") { // rename "QAT (on CPU)" to simply "QAT"
				accelName = "QAT"
			}
			summaryParts = append(summaryParts, fmt.Sprintf("%s %s [0]", accelName, accelCount))
		}
		var summaryHv = HostValues{
			Name:       hv.Name,
			ValueNames: []string{"Accelerators Available [used]"},
			Values:     [][]string{{strings.Join(summaryParts, ", ")}},
		}
		table.AllHostValues = append(table.AllHostValues, summaryHv)
	}
	return
}

func newPowerTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Power",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"TDP",
				"Power & Perf Policy",
				"Frequency Governor",
				"Frequency Driver",
				"Max C-State",
			},
			Values: [][]string{
				{
					source.getTDP(),
					source.getPowerPerfPolicy(),
					source.getCommandOutputLine("cpu_freq_governor"),
					source.getCommandOutputLine("cpu_freq_driver"),
					source.getCommandOutputLine("max_cstate"),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newGPUTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "GPU",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	type GPU struct {
		Model string `yaml:"model"`
		MfgID string `yaml:"mfgid"`
		DevID string `yaml:"devid"`
	}
	var gpuDefs []GPU
	// load GPU info from YAML
	yamlBytes, err := resources.ReadFile("resources/gpus.yaml")
	if err != nil {
		log.Printf("failed to read gpus.yaml: %v", err)
		return
	}
	err = yaml.UnmarshalStrict(yamlBytes, &gpuDefs)
	if err != nil {
		log.Printf("failed to parse gpus.yaml: %v", err)
		return
	}
	for _, source := range sources {
		// get all GPUs from lshw
		var gpus [][]string
		gpusLshw := source.valsArrayFromRegexSubmatch("lshw", `^pci.*?\s+display\s+(\w+).*?\s+\[(\w+):(\w+)]$`)
		idxMfgName := 0
		idxMfgID := 1
		idxDevID := 2
		for _, gpu := range gpusLshw {
			// Find GPU in GPU defs, note the model
			var model string
			for _, gpuDef := range gpuDefs {
				if gpu[idxMfgID] == gpuDef.MfgID {
					re, err := regexp.Compile(gpuDef.DevID)
					if err != nil {
						log.Printf("failed to compile regex from GPU definition: %s", gpuDef.DevID)
						return
					}
					if re.FindString(gpu[idxDevID]) != "" {
						// found it
						model = gpuDef.Model
						break
					}
				}
			}
			if model == "" {
				if gpu[idxMfgID] == "8086" {
					model = "Unknown Intel"
				} else {
					model = "Unknown"
				}
			}
			gpus = append(gpus, []string{gpu[idxMfgName], model, gpu[idxMfgID] + ":" + gpu[idxDevID]})
		}
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Manufacturer",
				"Model",
				"PCI ID",
			},
			Values: gpus,
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newNICTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "NIC",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	idxNicName := 0
	idxNicModel := 1
	for _, source := range sources {
		nicsInfo := source.valsArrayFromRegexSubmatch("lshw", `^pci.*? (\S+)\s+network\s+(\S.*?)\s+\[\w+:\w+]$`)
		nicsInfo = append(nicsInfo, source.valsArrayFromRegexSubmatch("lshw", `^usb.*? (\S+)\s+network\s+(\S.*?)$`)...)
		var nics [][]string
		for _, nic := range nicsInfo {
			nics = append(nics, []string{
				nic[idxNicName],
				nic[idxNicModel],
				source.valFromOutputRegexSubmatch("nic info", fmt.Sprintf(`Settings for %s:(?:.|\n)*?Speed:\s*(.+)(?:.|\n)*?MAC ADDRESS`, nic[0])),
				source.valFromOutputRegexSubmatch("nic info", fmt.Sprintf(`Settings for %s:(?:.|\n)*?Link detected:\s*(.+)(?:.|\n)*?MAC ADDRESS`, nic[0])),
				source.valFromOutputRegexSubmatch("nic info", fmt.Sprintf(`Settings for %s:(?:.|\n)*?bus-info:\s*(.+)(?:.|\n)*?MAC ADDRESS`, nic[0])),
				source.valFromOutputRegexSubmatch("nic info", fmt.Sprintf(`Settings for %s:(?:.|\n)*?driver:\s*(.+)(?:.|\n)*?MAC ADDRESS`, nic[0])),
				source.valFromOutputRegexSubmatch("nic info", fmt.Sprintf(`Settings for %s:(?:.|\n)*?version:\s*(.+)(?:.|\n)*?MAC ADDRESS`, nic[0])),
				source.valFromOutputRegexSubmatch("nic info", fmt.Sprintf(`Settings for %s:(?:.|\n)*?firmware-version:\s*(.+)(?:.|\n)*?MAC ADDRESS`, nic[0])),
				source.valFromOutputRegexSubmatch("nic info", fmt.Sprintf(`MAC ADDRESS %s: (.*)\n`, nic[0])),
				source.valFromOutputRegexSubmatch("nic info", fmt.Sprintf(`NUMA NODE %s: (.*)\n`, nic[0])),
				enabledIfVal(source.getCommandOutputLine("irqbalance")),
			})
		}
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Name",
				"Model",
				"Speed",
				"Link",
				"Bus",
				"Driver",
				"Driver Version",
				"Firmware Version",
				"MAC Address",
				"NUMA Node",
				"IRQBalance",
			},
			Values: nics,
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newNICSummaryTable(tableNic *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "NIC",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, hv := range tableNic.AllHostValues {
		modelValIdx := 1
		var modelCount = make(map[string]int)
		for _, nic := range hv.Values {
			model := nic[modelValIdx]
			if _, ok := modelCount[model]; !ok {
				modelCount[model] = 0
			}
			modelCount[model] += 1
		}
		var summaryParts []string
		for model, count := range modelCount {
			summaryParts = append(summaryParts, fmt.Sprintf("%dx %s", count, model))
		}
		var summaryHv = HostValues{
			Name:       hv.Name,
			ValueNames: []string{"NIC"},
			Values:     [][]string{{strings.Join(summaryParts, ", ")}},
		}
		table.AllHostValues = append(table.AllHostValues, summaryHv)
	}
	return
}

func newMemoryTable(sources []*Source, tableDIMM *Table, tableDIMMPopulation *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Memory",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for sourceIdx, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Installed Memory",
				"MemTotal",
				"MemFree",
				"MemAvailable",
				"Buffers",
				"Cached",
				"HugePages_Total",
				"Hugepagesize",
				"Transparent Huge Pages",
				"Automatic NUMA Balancing",
				"Populated Memory Channels",
			},
			Values: [][]string{
				{
					getDIMMsSummary(tableDIMM, sourceIdx),
					source.valFromRegexSubmatch("/proc/meminfo", `^MemTotal:\s*(.+?)$`),
					source.valFromRegexSubmatch("/proc/meminfo", `^MemFree:\s*(.+?)$`),
					source.valFromRegexSubmatch("/proc/meminfo", `^MemAvailable:\s*(.+?)$`),
					source.valFromRegexSubmatch("/proc/meminfo", `^Buffers:\s*(.+?)$`),
					source.valFromRegexSubmatch("/proc/meminfo", `^Cached:\s*(.+?)$`),
					source.valFromRegexSubmatch("/proc/meminfo", `^HugePages_Total:\s*(.+?)$`),
					source.valFromRegexSubmatch("/proc/meminfo", `^Hugepagesize:\s*(.+?)$`),
					source.valFromRegexSubmatch("transparent huge pages", `.*\[(.*)\].*`),
					source.getMemoryNUMABalancing(),
					getPopulatedMemoryChannels(tableDIMMPopulation, sourceIdx),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newMemoryBriefTable(tableMemory *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Memory",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	copyValues(tableMemory, table, []string{
		"Installed Memory",
		"Hugepagesize",
		"Transparent Huge Pages",
		"Automatic NUMA Balancing",
	})
	for i := range table.AllHostValues {
		table.AllHostValues[i].Name = tableMemory.AllHostValues[i].Name
	}
	return
}

const (
	BankLocatorIdx = iota
	LocatorIdx
	ManufacturerIdx
	PartIdx
	SerialIdx
	SizeIdx
	TypeIdx
	DetailIdx
	SpeedIdx
	RankIdx
	ConfiguredSpeedIdx
	DerivedSocketIdx
	DerivedChannelIdx
	DerivedSlotIdx
)

func newDIMMTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "DIMM",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Bank Locator",
				"Locator",
				"Manufacturer",
				"Part",
				"Serial",
				"Size",
				"Type",
				"Detail",
				"Speed",
				"Rank",
				"Configured Speed",
			},
			Values: source.valsArrayFromDmiDecodeRegexSubmatch(
				"17",
				`^Bank Locator:\s*(.+?)$`,
				`^Locator:\s*(.+?)$`,
				`^Manufacturer:\s*(.+?)$`,
				`^Part Number:\s*(.+?)\s*$`,
				`^Serial Number:\s*(.+?)\s*$`,
				`^Size:\s*(.+?)$`,
				`^Type:\s*(.+?)$`,
				`^Type Detail:\s*(.+?)$`,
				`^Speed:\s*(.+?)$`,
				`^Rank:\s*(.+?)$`,
				`^Configured.*Speed:\s*(.+?)$`,
			),
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

/*
DMI type 9, 24 bytes
System Slot Information

	Designation: RISER_SLOT_1(PCIe x32)
	Type: x8 \u003cOUT OF SPEC\u003e
	Current Usage: In Use
	Length: Long
	Characteristics:
		3.3 V is provided
	Bus Address: 0000:26:01.0
	Data Bus Width: 11
	Peer Devices: 0

	Handle 0x007F
*/
func newPCIeSlotsTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "PCIe Slots",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Designation",
				"Type",
				"Length",
				"Bus Address",
				"Current Usage",
			},
			Values: source.valsArrayFromDmiDecodeRegexSubmatch(
				"9",
				`^Designation:\s*(.+?)$`,
				`^Type:\s*(.+?)$`,
				`^Length:\s*(.+?)\s*$`,
				`^Bus Address:\s*(.+?)$`,
				`^Current Usage:\s*(.+?)$`,
			),
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newDIMMPopulationTable(sources []*Source, dimmTable *Table, cpusInfo *cpu.CPU, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "DIMM Population",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for sourceIdx, source := range sources {
		// deep copy of dimmTable's HostValues
		var hv HostValues
		hv.Name = dimmTable.AllHostValues[sourceIdx].Name
		hv.ValueNames = append(hv.ValueNames, dimmTable.AllHostValues[sourceIdx].ValueNames...)
		hv.Values = append(hv.Values, dimmTable.AllHostValues[sourceIdx].Values...)
		// extend value names
		hv.ValueNames = append(hv.ValueNames, []string{"Derived Socket", "Derived Channel", "Derived Slot"}...)
		// populate with empty values
		for valuesIdx := range hv.Values {
			hv.Values[valuesIdx] = append(hv.Values[valuesIdx], []string{"", "", ""}...)
		}
		success := false
		family := source.valFromRegexSubmatch("lscpu", `^CPU family.*:\s*([0-9]+)$`)
		model := source.valFromRegexSubmatch("lscpu", `^Model.*:\s*([0-9]+)$`)
		stepping := source.valFromRegexSubmatch("lscpu", `^Stepping.*:\s*(.+)$`)
		channels, err := cpusInfo.GetMemoryChannels(family, model, stepping)
		if err != nil {
			log.Printf("Failed to find CPU info: %v", err)
		} else {
			vendor := source.valFromDmiDecodeRegexSubmatch("0", `^\s*Vendor:\s*(.+?)$`)
			sockets, _ := strconv.Atoi(source.valFromRegexSubmatch("lscpu", `^Socket\(.*:\s*(.+?)$`))
			if vendor == "Dell" {
				err := deriveDIMMInfoDell(&hv.Values, sockets, channels)
				if err != nil {
					log.Printf("%v", err)
				}
				success = err == nil
			} else if vendor == "HPE" {
				err := deriveDIMMInfoHPE(&hv.Values, sockets, channels)
				if err != nil {
					log.Printf("%v", err)
				}
				success = err == nil
			} else if vendor == "Amazon EC2" {
				err := deriveDIMMInfoEC2(&hv.Values, sockets, channels)
				if err != nil {
					log.Printf("%v", err)
				}
				success = err == nil
			}
			if !success {
				err := deriveDIMMInfoOther(&hv.Values, sockets, channels)
				if err != nil {
					log.Printf("%v", err)
				}
				success = err == nil
			}
		}
		if !success {
			hv.ValueNames = []string{}
			hv.Values = [][]string{}
		}
		table.AllHostValues = append(table.AllHostValues, hv)
	}
	return
}

func newBenchmarkSummaryTable(sources []*Source, tableMemBandwidthLatency *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Summary",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		singleCoreTurbo, allCoreTurbo, turboTDP := source.getTurbo()
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"CPU Speed",
				"Single-core Turbo",
				"All-core Turbo",
				"Turbo TDP",
				"Idle TDP",
				"Memory Peak Bandwidth",
				"Memory Minimum Latency",
				"Disk Speed",
			},
			Values: [][]string{
				{
					source.getCPUSpeed(), // CPU speed
					singleCoreTurbo,      // single-core turbo
					allCoreTurbo,         // all-core turbo
					turboTDP,             // turbo TDP
					source.getIdleTDP(),  // idle TDP
					source.getPeakBandwidth(tableMemBandwidthLatency), // peak memory bandwidth
					source.getMinLatency(tableMemBandwidthLatency),    // minimum memory latency
					source.getDiskSpeed(),                             // disk speed
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newDiskTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Disk",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"NAME",
				"MODEL",
				"SIZE",
				"MOUNTPOINT",
				"FSTYPE",
				"RQ-SIZE",
				"MIN-IO",
				"FwRev",
			},
			Values: [][]string{},
		}
		for i, line := range source.getCommandOutputLines("lsblk -r -o") {
			fields := strings.Split(line, " ")
			if len(fields) != len(hostValues.ValueNames)-1 {
				log.Printf("lsblk field count mismatch: %s", strings.Join(fields, ","))
				continue
			}
			if i == 0 { // headers are in the first line
				for idx, field := range fields {
					if field != hostValues.ValueNames[idx] {
						log.Printf("lsblk field name mismatch: %s", strings.Join(fields, ","))
						break
					}
				}
				continue
			}
			// clean up the model name
			fields[1] = strings.ReplaceAll(fields[1], `\x20`, " ")
			fields[1] = strings.TrimSpace(fields[1])
			fields = append(fields, source.getDiskFwRev(fields[0]))
			hostValues.Values = append(hostValues.Values, fields)
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newDiskSummaryTable(tableDisk *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Disk",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, hv := range tableDisk.AllHostValues {
		modelValIdx := 1
		sizeValIdx := 2
		var modelSizeCount = make(map[string]int)
		for _, disk := range hv.Values {
			model := disk[modelValIdx]
			if model != "" {
				size := disk[sizeValIdx]
				modelSize := strings.Join([]string{model, size}, ",")
				if _, ok := modelSizeCount[modelSize]; !ok {
					modelSizeCount[modelSize] = 0
				}
				modelSizeCount[modelSize] += 1
			}
		}
		var summaryParts []string
		for modelSize, count := range modelSizeCount {
			tokens := strings.Split(modelSize, ",")
			model := tokens[0]
			size := tokens[1]
			summaryParts = append(summaryParts, fmt.Sprintf("%dx %s %s", count, size, model))
		}
		var summaryHv = HostValues{
			Name:       hv.Name,
			ValueNames: []string{"Disk"},
			Values:     [][]string{{strings.Join(summaryParts, ", ")}},
		}
		table.AllHostValues = append(table.AllHostValues, summaryHv)
	}
	return
}

func newFilesystemTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Filesystem",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name:       source.getHostname(),
			ValueNames: []string{},
			Values:     [][]string{},
		}
		for i, line := range source.getCommandOutputLines("df -h") {
			fields := strings.Fields(line)
			// "Mounted On" gets split into two fields, rejoin
			if fields[len(fields)-2] == "Mounted" && fields[len(fields)-1] == "on" {
				fields[len(fields)-2] = "Mounted on"
				fields = fields[:len(fields)-1]
			}
			if i == 0 { // headers are in the first line
				hostValues.ValueNames = fields
				hostValues.ValueNames = append(hostValues.ValueNames, "Mount Options")
				continue
			}
			if len(fields)+1 != len(hostValues.ValueNames) {
				log.Printf("Warning: filesystem field count does not match header count: %s", strings.Join(fields, ","))
				continue
			}
			fields = append(fields, source.getMountOptions(fields[0] /*Filesystem*/, fields[5] /*Mounted On*/))
			hostValues.Values = append(hostValues.Values, fields)
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newProcessTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Process",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name:       source.getHostname(),
			ValueNames: []string{},
			Values:     [][]string{},
		}
		for i, line := range source.getCommandOutputLines("ps -eo") {
			fields := strings.Fields(line)
			if i == 0 {
				hostValues.ValueNames = fields
				continue
			}
			// combine trailing fields
			if len(fields) > len(hostValues.ValueNames) {
				fields[len(hostValues.ValueNames)-1] = strings.Join(fields[len(hostValues.ValueNames)-1:], " ")
				fields = fields[:len(hostValues.ValueNames)]
			}
			if len(fields) != len(hostValues.ValueNames) {
				log.Printf("Warning: process field count does not match header count: %s", strings.Join(fields, ","))
				continue
			}
			hostValues.Values = append(hostValues.Values, fields)
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newPMUTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "PMU",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"cpu_cycles",
				"instructions",
				"ref_cycles",
				"topdown_slots",
				"gen_programmable_1",
				"gen_programmable_2",
				"gen_programmable_3",
				"gen_programmable_4",
				"gen_programmable_5",
				"gen_programmable_6",
				"gen_programmable_7",
				"gen_programmable_8",
			},
			Values: [][]string{},
		}
		lines := source.getCommandOutputLines("msrbusy")
		var vals []string
		if len(lines) == 2 {
			vals = strings.Split(lines[1], "|")
		} else {
			for range hostValues.ValueNames {
				vals = append(vals, "")
			}
		}
		hostValues.Values = append(hostValues.Values, vals)
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newVulnerabilityTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Vulnerability",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name:       source.getHostname(),
			ValueNames: []string{},
			Values:     [][]string{},
		}
		vulns := source.getVulnerabilities()
		var values []string
		// sort the keys
		var keys []string
		for k := range vulns {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			hostValues.ValueNames = append(hostValues.ValueNames, k)
			values = append(values, vulns[k])
		}
		if len(values) > 0 {
			hostValues.Values = append(hostValues.Values, []string{})
			hostValues.Values[0] = values
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newVulnerabilitySummaryTable(tableVuln *Table, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Vulnerability",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	re := regexp.MustCompile(`([A-Z]+)\s.*`)
	for _, hv := range tableVuln.AllHostValues {
		var vulns []string
		for valIdx, valueName := range hv.ValueNames {
			longValue := hv.Values[0][valIdx]
			match := re.FindStringSubmatch(longValue)
			if match != nil {
				shortValue := match[1]
				vulns = append(vulns, fmt.Sprintf("%s:%s", valueName, shortValue))
			}
		}
		var summaryHv = HostValues{
			Name:       hv.Name,
			ValueNames: []string{"Vulnerability"},
			Values:     [][]string{{strings.Join(vulns, ", ")}},
		}
		table.AllHostValues = append(table.AllHostValues, summaryHv)
	}
	return
}

func newSensorTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Sensor",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name:       source.getHostname(),
			ValueNames: []string{"Sensor", "Reading", "Status"},
			Values:     [][]string{},
		}
		for _, line := range source.getCommandOutputLines("ipmitool sdr list full") {
			vals := strings.Split(line, " | ")
			if len(vals) != len(hostValues.ValueNames) {
				log.Printf("Warning: unexpected number of sensor fields: %s", strings.Join(vals, ","))
				continue
			}
			for i := range vals {
				vals[i] = strings.TrimSpace(vals[i])
			}
			hostValues.Values = append(hostValues.Values, vals)
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newChassisStatusTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Chassis Status",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Last Power Event",
				"Power Overload",
				"Main Power Fault",
				"Power Restore Policy",
				"Drive Fault",
				"Cooling/Fan Fault",
				"System Time",
			},
			Values: [][]string{
				{
					source.valFromRegexSubmatch("ipmitool chassis status", `^Last Power Event\s*: (.+?)$`),
					source.valFromRegexSubmatch("ipmitool chassis status", `^Power Overload\s*: (.+?)$`),
					source.valFromRegexSubmatch("ipmitool chassis status", `^Main Power Fault\s*: (.+?)$`),
					source.valFromRegexSubmatch("ipmitool chassis status", `^Power Restore Policy\s*: (.+?)$`),
					source.valFromRegexSubmatch("ipmitool chassis status", `^Drive Fault\s*: (.+?)$`),
					source.valFromRegexSubmatch("ipmitool chassis status", `^Cooling/Fan Fault\s*: (.+?)$`),
					source.getCommandOutputLine("ipmitool sel time get"),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newSystemEventLogTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "System Event Log",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Date",
				"Time",
				"Sensor",
				"Status",
				"Event",
			},
			Values: [][]string{},
		}
		for _, line := range source.getCommandOutputLines("ipmitool sel elist") {
			fields := strings.Split(line, " | ")
			if len(fields) > len(hostValues.ValueNames) {
				fields[len(hostValues.ValueNames)-1] = strings.Join(fields[len(hostValues.ValueNames)-1:], ", ")
				fields = fields[:len(hostValues.ValueNames)]

			}
			if len(fields) != len(hostValues.ValueNames) {
				log.Printf("Warning: unexpected number of event list fields: %s", strings.Join(fields, ","))
				continue
			}
			hostValues.Values = append(hostValues.Values, fields)
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newKernelLogTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Kernel Log",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Entries",
			},
			Values: [][]string{},
		}
		for _, line := range source.getCommandOutputLines("dmesg") {
			hostValues.Values = append(hostValues.Values, []string{line})
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newCPUUtilizationTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "CPU Utilization",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Time",
				"CPU",
				"CORE",
				"SOCK",
				"NODE",
				"%usr",
				"%nice",
				"%sys",
				"%iowait",
				"%irq",
				"%soft",
				"%steal",
				"%guest",
				"%gnice",
				"%idle",
			},
			Values: [][]string{},
		}
		reStat := regexp.MustCompile(`^(\d\d:\d\d:\d\d)\s+(\d+)\s+(\d+)\s+(\d+)\s+(-*\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)$`)
		for _, line := range source.getProfileLines("mpstat") {
			match := reStat.FindStringSubmatch(line)
			if len(match) == 0 {
				continue
			}
			hostValues.Values = append(hostValues.Values, match[1:])
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newAverageCPUUtilizationTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Average CPU Utilization",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Time",
				"%usr",
				"%nice",
				"%sys",
				"%iowait",
				"%irq",
				"%soft",
				"%steal",
				"%guest",
				"%gnice",
				"%idle",
			},
			Values: [][]string{},
		}
		reStat := regexp.MustCompile(`^(\d\d:\d\d:\d\d)\s+all\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)$`)
		for _, line := range source.getProfileLines("mpstat") {
			match := reStat.FindStringSubmatch(line)
			if len(match) == 0 {
				continue
			}
			hostValues.Values = append(hostValues.Values, match[1:])
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newIRQRateTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "IRQ Rate",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Time",
				"CPU",
				"HI/s",
				"TIMER/s",
				"NET_TX/s",
				"NET_RX/s",
				"BLOCK/s",
				"IRQ_POLL/s",
				"TASKLET/s",
				"SCHED/s",
				"HRTIMER/s",
				"RCU/s",
			},
			Values: [][]string{},
		}
		reStat := regexp.MustCompile(`^(\d\d:\d\d:\d\d)\s+(\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)\s+(\d+\.\d+)$`)
		for _, line := range source.getProfileLines("mpstat") {
			match := reStat.FindStringSubmatch(line)
			if len(match) == 0 {
				continue
			}
			hostValues.Values = append(hostValues.Values, match[1:])
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newDriveStatsTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Drive Stats",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Device",
				"tps",
				"kB_read/s",
				"kB_wrtn/s",
				"kB_dscd/s",
			},
			Values: [][]string{},
		}
		// don't capture the last three vals: "kB_read","kB_wrtn","kB_dscd" -- they aren't the same scale as the others
		reStat := regexp.MustCompile(`^(\w+)\s*(\d+.\d+)\s*(\d+.\d+)\s*(\d+.\d+)\s*(\d+.\d+)\s*\d+\s*\d+\s*\d+$`)
		for _, line := range source.getProfileLines("iostat") {
			match := reStat.FindStringSubmatch(line)
			if len(match) == 0 {
				continue
			}
			hostValues.Values = append(hostValues.Values, match[1:])
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newNetworkStatsTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Network Stats",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Time",
				"IFACE",
				"rxpck/s",
				"txpck/s",
				"rxkB/s",
				"txkB/s",
			},
			Values: [][]string{},
		}
		// don't capture the last four vals: "rxcmp/s","txcmp/s","rxcmt/s","%ifutil" -- obscure more important vals
		reStat := regexp.MustCompile(`^(\d+:\d+:\d+)\s*(\w*)\s*(\d+.\d+)\s*(\d+.\d+)\s*(\d+.\d+)\s*(\d+.\d+)\s*\d+.\d+\s*\d+.\d+\s*\d+.\d+\s*\d+.\d+$`)
		for _, line := range source.getProfileLines("sar-network") {
			match := reStat.FindStringSubmatch(line)
			if len(match) == 0 {
				continue
			}
			hostValues.Values = append(hostValues.Values, match[1:])
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newMemoryStatsTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Memory Stats",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Time",
				"free",
				"avail",
				"used",
				"buffers",
				"cached",
				"commit",
				"active",
				"inactive",
				"dirty",
			},
			Values: [][]string{},
		}
		reStat := regexp.MustCompile(`^(\d+:\d+:\d+)\s*(\d+)\s*(\d+)\s*(\d+)\s*\d+\.\d+\s*(\d+)\s*(\d+)\s*(\d+)\s*\d+\.\d+\s*(\d+)\s*(\d+)\s*(\d+)$`)
		for _, line := range source.getProfileLines("sar-memory") {
			match := reStat.FindStringSubmatch(line)
			if len(match) == 0 {
				continue
			}
			hostValues.Values = append(hostValues.Values, match[1:])
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newPowerStatsTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Power Stats",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Package",
				"DRAM",
			},
			Values: [][]string{},
		}
		reStat := regexp.MustCompile(`^(\d+\.\d+)\s*(\d+\.\d+)$`)
		for _, line := range source.getProfileLines("turbostat") {
			match := reStat.FindStringSubmatch(line)
			if len(match) == 0 {
				continue
			}
			hostValues.Values = append(hostValues.Values, match[1:])
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}
func newProfileSummaryTable(sources []*Source, category TableCategory, averageCPUUtilizationTable, CPUUtilizationTable, IRQRateTable, driveStatsTable, netStatsTable, memStatsTable, PMUMetricsTable, powerStatsTable *Table) (table *Table) {
	table = &Table{
		Name:          "Summary",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for idx, source := range sources {
		utilization := getCPUAveragePercentage(averageCPUUtilizationTable, idx, "%idle", true)
		if utilization == "" {
			utilization = getPMUMetricFromTable(PMUMetricsTable, idx, "CPU utilization %")
		}
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"CPU Utilization (%)",
				"CPU Frequency (GHz)",
				"CPI",
				"Package Power (Watts)",
				"Drive Reads (kB/s)",
				"Drive Writes (kB/s)",
				"Network RX (kB/s)",
				"Network TX (kB/s)",
				"Memory Available (kB)",
			},
			Values: [][]string{
				{
					utilization,
					getPMUMetricFromTable(PMUMetricsTable, idx, "CPU operating frequency (in GHz)"),
					getPMUMetricFromTable(PMUMetricsTable, idx, "CPI"),
					getMetricAverage(powerStatsTable, idx, []string{"Package"}, ""),
					getMetricAverage(driveStatsTable, idx, []string{"kB_read/s"}, "Device"),
					getMetricAverage(driveStatsTable, idx, []string{"kB_wrtn/s"}, "Device"),
					getMetricAverage(netStatsTable, idx, []string{"rxkB/s"}, "Time"),
					getMetricAverage(netStatsTable, idx, []string{"txkB/s"}, "Time"),
					getMetricAverage(memStatsTable, idx, []string{"avail"}, "Time"),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newFeatureTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Feature",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"BI_2IFU_4_F_VICTIMS_EN",
				"EnableDBPForF",
				"NoHmlessPref",
				"FBThreadSlicing",
				"DISABLE_FASTGO",
				"SpecI2MEn",
				"disable_llpref",
				"DPT_DISABLE",
			},
		}
		hostValues.Values = append(hostValues.Values, source.getFeatures())
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newCXLDeviceTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "CXL Device",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Slot",
				"Class",
				"Vendor",
				"Device",
				"Rev",
				"ProgIf",
				"NUMANode",
				"IOMMUGroup",
			},
		}
		hostCxlDevices := source.getPCIDevices("CXL")
		for _, device := range hostCxlDevices {
			var values []string
			for _, key := range hostValues.ValueNames {
				if value, ok := device[key]; ok {
					values = append(values, value)
				} else {
					values = append(values, "")
				}
			}
			hostValues.Values = append(hostValues.Values, values)
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newCodePathTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "Code Path Frequency",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		hv := HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"System Paths",
				"Java Paths",
			},
			Values: [][]string{
				{
					source.getSystemFolded(),
					source.getJavaFolded(),
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hv)
	}
	return
}

func newInsightTable(sources []*Source, configReport, briefReport, profileReport, benchmarkReport *Report, analyzeReport *Report, cpusInfo *cpu.CPU) (table *Table) {
	table = &Table{
		Name:          "Insight",
		Category:      NoCategory,
		AllHostValues: []HostValues{},
	}
	var gruleEngine *engine.GruleEngine
	var knowledgeBase *ast.KnowledgeBase
	var dataContext ast.IDataContext
	rulesEngineContext := &RulesEngineContext{
		insightTable: table,
		reportsData:  []*Report{configReport, briefReport, profileReport, benchmarkReport, analyzeReport},
		sourceIdx:    0, // will be incremented while looping through sources below
	}
	gruleEngine = &engine.GruleEngine{MaxCycle: 500}
	rules, err := getInsightsRules()
	if err != nil {
		log.Printf("Failed to load insights rules: %v", err)
	} else {
		dataContext = ast.NewDataContext()
		err = dataContext.Add("Report", rulesEngineContext) // we call it "Report" because that makes sense when writing/reading rules
		if err != nil {
			log.Panicf("failed to add context: %v", err)
		}
		knowledgeLibrary := ast.NewKnowledgeLibrary()
		ruleBuilder := builder.NewRuleBuilder(knowledgeLibrary)
		err = ruleBuilder.BuildRuleFromResource("Rules", "0.1", pkg.NewBytesResource(rules))
		if err != nil {
			// Ref: https://github.com/hyperjumptech/grule-rule-engine/blob/master/docs/en/GRL_en.md
			// Cast the error into pkg.GruleErrorReporter with typecast checking.
			// Typecast checking is necessary because the err might not only parsing error.
			if reporter, ok := err.(*pkg.GruleErrorReporter); ok {
				// Lets iterate all the error we get during parsing.
				for i, er := range reporter.Errors {
					log.Printf("rules parsing error #%d : %s\n", i, er.Error())
				}
			} else {
				log.Printf("failed to load rules into engine, %v", err)
			}
		} else {
			knowledgeBase, err = knowledgeLibrary.NewKnowledgeBaseInstance("Rules", "0.1")
			if err != nil {
				log.Panicf("failed to create knowledge base instance: %v", err)
			}
		}
	}
	for sourceIdx, source := range configReport.Sources {
		hv := HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Recommendation",
				"Justification",
			},
		}
		table.AllHostValues = append(table.AllHostValues, hv)
		if knowledgeBase != nil {
			rulesEngineContext.sourceIdx = sourceIdx
			err = gruleEngine.Execute(dataContext, knowledgeBase)
			if err != nil {
				log.Printf("failed to execute rules, %v", err)
				continue
			}
		}
	}
	return
}

func newSvrinfoTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "svr-info",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"version",
			},
			Values: [][]string{
				{
					gVersion,
				},
			},
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}

func newPMUMetricsTable(sources []*Source, category TableCategory) (table *Table) {
	table = &Table{
		Name:          "PMU Metrics",
		Category:      category,
		AllHostValues: []HostValues{},
	}
	for _, source := range sources {
		var hostValues = HostValues{
			Name: source.getHostname(),
			ValueNames: []string{
				"Name",
				"Average",
				"Min",
				"Max",
			},
			Values: [][]string{},
		}
		metricNames, timeStamps, metrics := source.getPMUMetrics()
		if len(metrics) > 0 {
			var series []string
			for _, ts := range timeStamps {
				series = append(series, fmt.Sprintf("%ss", strconv.FormatFloat(ts, 'f', 1, 64)))
			}
			hostValues.ValueNames = append(hostValues.ValueNames, series...)
			for i, name := range metricNames {
				hostValues.Values = append(hostValues.Values, []string{})
				var values []string
				values = append(values, name, strconv.FormatFloat(metrics[name].average, 'f', 4, 64), strconv.FormatFloat(metrics[name].min, 'f', 4, 64), strconv.FormatFloat(metrics[name].max, 'f', 4, 64))
				for _, val := range metrics[name].series {
					values = append(values, strconv.FormatFloat(val, 'f', 4, 64))
				}
				hostValues.Values[i] = append(hostValues.Values[i], values...)
			}
		}
		table.AllHostValues = append(table.AllHostValues, hostValues)
	}
	return
}
