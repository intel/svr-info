/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
/* Reads, parses, and provides access functions to json-formatted data file produced by the collector */

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type CommandData struct {
	Command    string `json:"command"`
	ExitStatus string `json:"exitstatus"`
	Label      string `json:"label"`
	Stderr     string `json:"stderr"`
	Stdout     string `json:"stdout"`
	SuperUser  string `json:"superuser"`
}

type Source struct {
	inputFilePath string
	Hostname      string
	ParsedData    map[string]CommandData // command label string: command data structure
}

func newSource(inputFilePath string) (source *Source) {
	source = &Source{
		inputFilePath: inputFilePath,
		Hostname:      "",
		ParsedData:    map[string]CommandData{},
	}
	return
}

func (s *Source) parse() (err error) {
	inputBytes, err := os.ReadFile(s.inputFilePath)
	if err != nil {
		return
	}
	var jsonData map[string][]CommandData // hostname: array of command data (this is the format of collector output file)
	err = json.Unmarshal(inputBytes, &jsonData)
	if err != nil {
		return
	}
	// get the hostname
	var hostname string
	for hostname = range jsonData {
		break
	}
	s.Hostname = hostname
	// put the data in a map for faster lookup by command label
	for _, c := range jsonData[hostname] {
		s.ParsedData[c.Label] = c
	}
	return
}

func (s *Source) getHostname() (hostname string) {
	return s.Hostname
}

// return command output or empty string if no match
func (s *Source) getCommandOutput(cmdLabel string) (output string) {
	if c, ok := s.ParsedData[cmdLabel]; ok {
		output = c.Stdout
	}
	return
}

// return array of lines from command output, or empty array if no match or all empty lines
func (s *Source) getCommandOutputLines(cmdLabel string) (lines []string) {
	cmdout := s.getCommandOutput(cmdLabel)
	dirtyLines := strings.Split(cmdout, "\n")
	for _, dirtyLine := range dirtyLines {
		line := strings.TrimSpace(dirtyLine)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return
}

// get the first line from command output, or empty string
func (s *Source) getCommandOutputLine(cmdLabel string) (line string) {
	lines := s.getCommandOutputLines(cmdLabel)
	if len(lines) > 0 {
		line = lines[0]
	}
	return
}

func (s *Source) getCommandOutputSections(cmdLabel string) (sections map[string]string) {
	reHeader := regexp.MustCompile(`^##########\s+(.+)\s+##########$`)
	sections = make(map[string]string, 0)
	var header string
	var sectionLines []string
	lines := s.getCommandOutputLines(cmdLabel)
	lineCount := len(lines)
	for idx, line := range lines {
		match := reHeader.FindStringSubmatch(line)
		if match != nil {
			if header != "" {
				sections[header] = strings.Join(sectionLines, "\n")
				sectionLines = []string{}
			}
			header = match[1]
			if _, ok := sections[header]; ok {
				log.Panic("can't have same header twice")
			}
			continue
		}
		sectionLines = append(sectionLines, line)
		if idx == lineCount-1 {
			sections[header] = strings.Join(sectionLines, "\n")
		}
	}
	return
}

// getCommandOutputLabeled -- some collector commands collect output from more than one
// command. We separate that output data with a header that allows us to more easily
// parse it. This function loads that data into a map where the key is extracted
// from the header and the value is the output data itself
// note: only output from those sections whose header matches the provided labelPattern
func (s *Source) getCommandOutputLabeled(cmdLabel string, labelPattern string) (sections map[string]string) {
	sections = make(map[string]string, 0)
	allSections := s.getCommandOutputSections(cmdLabel)
	reLabel := regexp.MustCompile(labelPattern)
	for header, content := range allSections {
		if reLabel.FindString(header) != "" {
			sections[header] = content
		}
	}
	return
}

// return first match or empty string if no match
func (s *Source) valFromRegexSubmatch(cmdLabel string, regex string) (val string) {
	re := regexp.MustCompile(regex)
	for _, line := range s.getCommandOutputLines(cmdLabel) {
		match := re.FindStringSubmatch(line)
		if len(match) > 1 {
			val = match[1]
			return
		}
	}
	return
}

// return first match or empty string if no match
func (s *Source) valFromOutputRegexSubmatch(cmdLabel string, regex string) (val string) {
	re := regexp.MustCompile(regex)
	cmdout := s.getCommandOutput(cmdLabel)
	match := re.FindStringSubmatch(cmdout)
	if match != nil {
		val = match[1]
		return
	}
	return
}

// return all matches for first capture group in regex
func (s *Source) valsFromRegexSubmatch(cmdLabel string, regex string) (vals []string) {
	re := regexp.MustCompile(regex)
	for _, line := range s.getCommandOutputLines(cmdLabel) {
		match := re.FindStringSubmatch(line)
		if len(match) > 1 {
			vals = append(vals, match[1])
		}
	}
	return
}

// return all matches for all capture groups in regex
func (s *Source) valsArrayFromRegexSubmatch(cmdLabel string, regex string) (vals [][]string) {
	re := regexp.MustCompile(regex)
	for _, line := range s.getCommandOutputLines(cmdLabel) {
		match := re.FindStringSubmatch(line)
		if len(match) > 1 {
			vals = append(vals, match[1:])
		}
	}
	return
}

// return all lines of dmi type specified
func (s *Source) getDmiDecodeLines(dmiType string) (lines []string) {
	start := false
	for _, line := range s.getCommandOutputLines("dmidecode") {
		if start && strings.HasPrefix(line, "Handle ") {
			start = false
		}
		if strings.Contains(line, "DMI type "+dmiType+",") {
			start = true
		}
		if start {
			lines = append(lines, line)
		}
	}
	return
}

// return single value from first regex submatch or empty string
func (s *Source) valFromDmiDecodeRegexSubmatch(dmiType string, regex string) (val string) {
	re := regexp.MustCompile(regex)
	for _, line := range s.getDmiDecodeLines(dmiType) {
		match := re.FindStringSubmatch(line)
		if len(match) > 1 {
			val = match[1]
			break
		}
	}
	return
}

// finds first match in dmiType section of DMI Decode output
// return array of values from regex submatches or zero-length array if no match
func (s *Source) valsFromDmiDecodeRegexSubmatch(dmiType string, regex string) (vals []string) {
	re := regexp.MustCompile(regex)
	for _, line := range s.getDmiDecodeLines(dmiType) {
		match := re.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		for i := 1; i < len(match); i++ {
			vals = append(vals, match[i])
		}
		break
	}
	return
}

func (s *Source) getDmiDecodeEntries(dmiType string) (entries [][]string) {
	output := s.getCommandOutput("dmidecode")
	lines := strings.Split(output, "\n")
	var entry []string
	typeMatch := false
	for _, line := range lines {
		if strings.HasPrefix(line, "Handle ") {
			if strings.Contains(line, "DMI type "+dmiType+",") {
				// type match
				typeMatch = true
				entry = []string{}
			} else {
				// not a type match
				typeMatch = false
			}
		}
		if !typeMatch {
			continue
		}
		if line == "" {
			// end of type match entry
			entries = append(entries, entry)
		} else {
			// a line in the entry
			entry = append(entry, line)
		}
	}
	return
}

// return table of matches
func (s *Source) valsArrayFromDmiDecodeRegexSubmatch(dmiType string, regexes ...string) (vals [][]string) {
	var res []*regexp.Regexp
	for _, r := range regexes {
		re := regexp.MustCompile(r)
		res = append(res, re)
	}
	for _, entry := range s.getDmiDecodeEntries(dmiType) {
		row := make([]string, len(res))
		for _, line := range entry {
			for i, re := range res {
				match := re.FindStringSubmatch(strings.TrimSpace(line))
				if len(match) > 1 {
					row[i] = match[1]
				}
			}
		}
		vals = append(vals, row)
	}
	return
}

// return all PCI Devices of specified class
func (s *Source) getPCIDevices(class string) (devices []map[string]string) {
	device := make(map[string]string)
	cmdout := s.getCommandOutput("lspci -vmm")
	re := regexp.MustCompile(`^(\w+):\s+(.*)$`)
	for _, line := range strings.Split(cmdout, "\n") {
		if line == "" { // end of device
			if devClass, ok := device["Class"]; ok {
				if devClass == class {
					devices = append(devices, device)
				}
			}
			device = make(map[string]string)
			continue
		}
		match := re.FindStringSubmatch(line)
		if len(match) > 0 {
			key := match[1]
			value := match[2]
			device[key] = value
		}
	}
	return
}

// return all lines of profile that matches profileRegex
func (s *Source) getProfileLines(profileRegex string) (lines []string) {
	re, err := regexp.Compile(profileRegex)
	if err != nil {
		log.Panicf("regex %s failed to compile", profileRegex)
	}
	labeledCmdout := s.getCommandOutputSections("profile")
	for label, cmdout := range labeledCmdout {
		if re.FindString(label) != "" {
			lines = strings.Split(cmdout, "\n")
			return
		}
	}
	return
}

func (s *Source) getOperatingSystem() (os string) {
	os = s.valFromRegexSubmatch("/etc/*-release", `^PRETTY_NAME=\"(.+?)\"`)
	centos := s.valFromRegexSubmatch("/etc/*-release", `^(CentOS Linux release .*)`)
	if centos != "" {
		os = centos
	}
	return
}

func (s *Source) getBaseFrequency() (val string) {
	/* add Base Frequency
	   1st option) /sys/devices/system/cpu/cpu0/cpufreq/base_frequency
	   2nd option) from dmidecode "Current Speed"
	   3nd option) parse it from the model name
	*/
	cmdout := s.getCommandOutputLine("base frequency")
	if cmdout != "" {
		freqf, err := strconv.ParseFloat(cmdout, 64)
		if err == nil {
			freqf = freqf / 1000000
			val = fmt.Sprintf("%.1fGHz", freqf)
		}
	}
	if val == "" {
		currentSpeedVals := s.valsFromDmiDecodeRegexSubmatch("4", `Current Speed:\s(\d+)\s(\w+)`)
		if len(currentSpeedVals) > 0 {
			num, err := strconv.ParseFloat(currentSpeedVals[0], 64)
			if err == nil {
				unit := currentSpeedVals[1]
				if unit == "MHz" {
					num = num / 1000
					unit = "GHz"
				}
				val = fmt.Sprintf("%.1f%s", num, unit)
			}
		}
	}
	if val == "" {
		modelName := s.valFromRegexSubmatch("lscpu", `^[Mm]odel name.*:\s*(.+?)$`)
		// the frequency (if included) is at the end of the model name
		tokens := strings.Split(modelName, " ")
		if len(tokens) > 0 {
			lastToken := tokens[len(tokens)-1]
			if len(lastToken) > 0 && lastToken[len(lastToken)-1] == 'z' {
				val = lastToken
			}
		}
	}
	return
}

func (s *Source) getMaxFrequency() (val string) {
	/* get max frequency
	 * 1st option) /sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq
	 * 2nd option) from MSR
	 * 3rd option) from dmidecode "Max Speed"
	 */
	cmdout := s.getCommandOutputLine("maximum frequency")
	if cmdout != "" {
		freqf, err := strconv.ParseFloat(cmdout, 64)
		if err == nil {
			freqf = freqf / 1000000
			val = fmt.Sprintf("%.1fGHz", freqf)
		}
	}
	if val == "" {
		countFreqs, err := s.getSpecCountFrequencies()
		// the first entry is the max single-core frequency
		if err == nil && len(countFreqs) > 0 && len(countFreqs[0]) > 1 {
			val = countFreqs[0][1]
		}
	}
	if val == "" {
		val = s.valFromDmiDecodeRegexSubmatch("4", `Max Speed:\s(.*)`)
	}
	return
}

func (s *Source) getAllCoreMaxFrequency() (val string) {
	countFreqs, err := s.getSpecCountFrequencies()
	// the last entry is the max all-core frequency
	if err == nil && len(countFreqs) > 0 && len(countFreqs[len(countFreqs)-1]) > 1 {
		val = countFreqs[len(countFreqs)-1][1] + "GHz"
	}
	return
}

func (s *Source) getNUMACPUList() (val string) {
	nodeCPUs := s.valsFromRegexSubmatch("lscpu", `^NUMA node[0-9] CPU\(.*:\s*(.+?)$`)
	val = strings.Join(nodeCPUs, " :: ")
	return
}

func (s *Source) getUncoreMaxFrequency() (val string) {
	hex := s.getCommandOutputLine("uncore max frequency")
	if hex != "" && hex != "0" {
		parsed, err := strconv.ParseInt(hex, 16, 64)
		if err == nil {
			val = fmt.Sprintf("%.1fGhz", float64(parsed)/10)
		}
	}
	return
}

func (s *Source) getUncoreMinFrequency() (val string) {
	hex := s.getCommandOutputLine("uncore min frequency")
	if hex != "" && hex != "0" {
		parsed, err := strconv.ParseInt(hex, 16, 64)
		if err == nil {
			val = fmt.Sprintf("%.1fGHz", float64(parsed)/10)
		}
	}
	return
}

func (s *Source) getCHACount() (val string) {
	options := []string{"uncore client cha count", "uncore cha count", "uncore cha count spr"}
	for _, option := range options {
		hexCount := s.getCommandOutputLine(option)
		if hexCount != "" && hexCount != "0" {
			count, err := strconv.ParseInt(hexCount, 16, 64)
			if err == nil {
				val = fmt.Sprintf("%d", count)
				break
			}
		}
	}
	return
}

func (s *Source) getCacheWays(uArch string) (cacheWays []int64) {
	var wayCount int
	if uArch == "BDX" {
		wayCount = 20
	} else if uArch == "SKX" || uArch == "CLX" {
		wayCount = 11
	} else if uArch == "ICX" {
		wayCount = 12
	} else if uArch == "SPR_MCC" || uArch == "SPR_XCC" {
		wayCount = 15
	} else if uArch == "EMR_MCC" || uArch == "EMR_XCC" {
		wayCount = 20
	} else {
		return
	}
	var cacheSize int64 = 0
	// set wayCount bits in cacheSize
	for i := 0; i < wayCount; i++ {
		cacheSize = (cacheSize << 1) | 1
	}
	var shift int64 = -1
	for i := 0; i < wayCount; i++ {
		cacheWays = append([]int64{cacheSize}, cacheWays...)
		shift = shift << 1
		cacheSize = cacheSize & shift
	}
	return
}

// get L3 in MB from lscpu
// known lscpu output formats for L3 cache:
//
//	1.5 MBi    < Ubuntu
//	1536KB     < CentOS
func (s *Source) getL3LscpuMB() (val float64, err error) {
	l3Lscpu := s.valFromRegexSubmatch("lscpu", `^L3 cache.*:\s*(.+?)$`)
	re := regexp.MustCompile(`(\d+\.?\d*)\s*(\w+).*`) // match known formats
	match := re.FindStringSubmatch(l3Lscpu)
	if len(match) == 0 {
		err = fmt.Errorf("Unknown L3 format in lscpu: %s", l3Lscpu)
		return
	}
	l3SizeNoUnit, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		err = fmt.Errorf("Failed to parse L3 size from lscpu: %s, %v", l3Lscpu, err)
		return
	}
	if strings.ToLower(match[2][:1]) == "m" {
		val = l3SizeNoUnit
		return
	}
	if strings.ToLower(match[2][:1]) == "k" {
		val = l3SizeNoUnit / 1024
		return
	}
	err = fmt.Errorf("Unknown L3 units in lscpu: %s", l3Lscpu)
	return
}

// get L3 in MB from MSR
func (s *Source) getL3MSRMB(uArch string) (val float64, err error) {
	l3LscpuMB, err := s.getL3LscpuMB()
	if err != nil {
		return
	}
	l3MSRHex := s.getCommandOutputLine("rdmsr 0xc90")
	l3MSR, err := strconv.ParseInt(l3MSRHex, 16, 64)
	if err != nil {
		err = fmt.Errorf("Failed to parse MSR output: %s", l3MSRHex)
		return
	}
	cacheWays := s.getCacheWays(uArch)
	if len(cacheWays) == 0 {
		err = fmt.Errorf("Failed to get cache ways for uArch: %s", uArch)
		return
	}
	cpul3SizeGB := l3LscpuMB / 1024
	GBperWay := cpul3SizeGB / float64(len(cacheWays))
	for i, way := range cacheWays {
		if way == l3MSR {
			val = float64(i+1) * GBperWay * 1024
			return
		}
	}
	err = fmt.Errorf("Did not find %d in cache ways.", l3MSR)
	return
}

func (s *Source) getL3(uArch string) (val string) {
	l3, err := s.getL3MSRMB(uArch)
	if err != nil {
		log.Printf("Could not get L3 size from MSR, falling back to lscpu.: %v", err)
		l3, err = s.getL3LscpuMB()
		if err != nil {
			log.Printf("Could not get L3 size from lscpu.: %v", err)
			return
		}
	}
	val = fmt.Sprintf("%s MiB", strconv.FormatFloat(l3, 'f', -1, 64))
	return
}

func (s *Source) getL3PerCore(uArch string, coresPerSocketStr string, socketsStr string, virtualization string) (val string) {
	if virtualization == "full" {
		log.Printf("Can't calculate L3 per Core on virtualized host.")
		return
	}
	l3, err := strconv.ParseFloat(strings.Split(s.getL3(uArch), " ")[0], 64)
	if err != nil {
		return
	}
	coresPerSocket, err := strconv.Atoi(coresPerSocketStr)
	if err != nil || coresPerSocket == 0 {
		return
	}
	sockets, err := strconv.Atoi(socketsStr)
	if err != nil || sockets == 0 {
		return
	}
	cacheMB := l3 / float64(coresPerSocket*sockets)
	val = fmt.Sprintf("%s", strconv.FormatFloat(cacheMB, 'f', 3, 64))
	val = strings.TrimRight(val, "0") // trim trailing zeros
	val = strings.TrimRight(val, ".") // trim decimal point if trailing
	val += " MiB"
	return
}

func (s *Source) getPrefetchers() (val string) {
	prefetchers := s.valFromRegexSubmatch("rdmsr 0x1a4", `^([0-9a-fA-F]+)`)
	if prefetchers != "" {
		prefetcherInt, err := strconv.ParseInt(prefetchers, 16, 64)
		if err == nil {
			// prefetchers are enabled when associated bit is 0
			// 1: "L2 HW"
			// 2: "L2 Adj."
			// 4: "DCU HW"
			// 8: "DCU IP"
			var prefList []string
			for i, pref := range []string{"L2 HW", "L2 Adj.", "DCU HW", "DCU IP"} {
				bitMask := int64(math.Pow(2, float64(i)))
				// if bit is zero
				if bitMask&prefetcherInt == 0 {
					prefList = append(prefList, pref)
				}
			}
			if len(prefList) > 0 {
				val = strings.Join(prefList, ", ")
			} else {
				val = "None"
			}
		}
	}
	return
}

/*
....................	bit		default
"BI to IFU",			2		0
"EnableDBPForF",		3		0
"NoHmlessPref",			14		0
"DisFBThreadSlicing",	15		1
"DISABLE_FASTGO",		27		0
"SpecI2MEn",			30		1
"disable_llpref",		42		0
"DPT_DISABLE",			45		0
*/
func (s *Source) getFeatures() (vals []string) {
	features := s.valFromRegexSubmatch("rdmsr 0x6d", `^([0-9a-fA-F]+)`)
	if features != "" {
		featureInt, err := strconv.ParseInt(features, 16, 64)
		if err == nil {
			for _, bit := range []int{2, 3, 14, 15, 27, 30, 42, 45} {
				bitMask := int64(math.Pow(2, float64(bit)))
				vals = append(vals, fmt.Sprintf("%d", bitMask&featureInt>>bit))
			}
		}
	}
	return
}

func (s *Source) getPPINs() (val string) {
	ppins := s.getCommandOutputLines("rdmsr 0x4f")
	uniquePpins := []string{}
	for _, ppin := range ppins {
		found := false
		for _, p := range uniquePpins {
			if string(p) == ppin {
				found = true
				break
			}
		}
		if !found && ppin != "" {
			uniquePpins = append(uniquePpins, ppin)
		}
	}
	val = strings.Join(uniquePpins, ",")
	return
}

func (s *Source) getHyperthreading() (val string) {
	// lscpu on Alder Lake (hybrid cores) reports one thread per core even when hyper-threading is enabled, so
	// use this approach to detect hyperthreading...
	numCPUs, err1 := strconv.Atoi(s.valFromRegexSubmatch("lscpu", `^CPU\(.*:\s*(.+?)$`)) // logical CPUs
	numSockets, err2 := strconv.Atoi(s.valFromRegexSubmatch("lscpu", `^Socket\(.*:\s*(.+?)$`))
	numCores, err3 := strconv.Atoi(s.valFromRegexSubmatch("lscpu", `^Core\(.*:\s*(.+?)$`)) // physical cores
	if err1 != nil || err2 != nil || err3 != nil {
		return
	}
	if numCPUs > numCores*numSockets {
		val = "Enabled"
	} else {
		val = "Disabled"
	}
	return
}

func convertMsrToDecimals(msr string) (decVals []int64, err error) {
	re := regexp.MustCompile(`[0-9a-fA-F][0-9a-fA-F]`)
	hexVals := re.FindAll([]byte(msr), -1)
	if hexVals == nil {
		err = fmt.Errorf("no hex values found in msr")
		return
	}
	decVals = make([]int64, len(hexVals))
	decValsIndex := len(decVals) - 1
	for _, hexVal := range hexVals {
		var decVal int64
		decVal, err = strconv.ParseInt(string(hexVal), 16, 64)
		if err != nil {
			return
		}
		decVals[decValsIndex] = decVal
		decValsIndex--
	}
	return
}

func (s *Source) getSpecCountFrequencies() (countFreqs [][]string, err error) {
	hexCounts := s.valFromRegexSubmatch("rdmsr 0x1ae", `^([0-9a-fA-F]+)`)
	hexFreqs := s.valFromRegexSubmatch("rdmsr 0x1ad", `^([0-9a-fA-F]+)`)
	if hexCounts != "" && hexFreqs != "" {
		var decCounts, decFreqs []int64
		decCounts, err = convertMsrToDecimals(hexCounts)
		if err != nil {
			return
		}
		decFreqs, err = convertMsrToDecimals(hexFreqs)
		if err != nil {
			return
		}
		if len(decCounts) != 8 || len(decFreqs) != 8 {
			err = fmt.Errorf("unexpected number of core counts or frequencies")
			return
		}
		for i, decCount := range decCounts {
			countFreqs = append(countFreqs, []string{fmt.Sprintf("%d", decCount), fmt.Sprintf("%.1f", float64(decFreqs[i])/10.0)})
		}
	}
	return
}

func (s *Source) getMemoryNUMABalancing() (val string) {
	out := s.getCommandOutputLine("automatic numa balancing")
	if out == "1" {
		val = "Enabled"
	} else if out == "0" {
		val = "Disabled"
	}
	return
}

func geoMean(vals []float64) (val float64) {
	m := 0.0
	for i, x := range vals {
		lx := math.Log(x)
		m += (lx - m) / float64(i+1)
	}
	val = math.Exp(m)
	return
}

func (s *Source) getCPUSpeed() (val string) {
	var vals []float64
	for _, line := range s.getCommandOutputLines("stress-ng cpu methods") {
		tokens := strings.Split(line, " ")
		if len(tokens) == 2 {
			fv, err := strconv.ParseFloat(tokens[1], 64)
			if err != nil {
				continue
			}
			vals = append(vals, fv)
		}
	}
	if len(vals) > 0 {
		geoMean := geoMean(vals)
		val = fmt.Sprintf("%.0f ops/s", geoMean)
	}
	return
}

func (s *Source) getTurbo() (singleCoreTurbo, allCoreTurbo, turboTDP string) {
	var allTurbos []string
	var allTDPs []string
	var turbos []string
	var tdps []string
	var headers []string
	idxTurbo := -1
	idxTdp := -1
	re := regexp.MustCompile(`\s+`) // whitespace
	for _, line := range s.getCommandOutputLines("CPU Turbo Test") {
		if strings.Contains(line, "stress-ng") {
			if strings.Contains(line, "completed") {
				if idxTurbo >= 0 && len(allTurbos) >= 2 {
					turbos = append(turbos, allTurbos[len(allTurbos)-2])
					allTurbos = nil
				}
				if idxTdp >= 0 && len(allTDPs) >= 2 {
					tdps = append(tdps, allTDPs[len(allTDPs)-2])
					allTDPs = nil
				}
			}
			continue
		}
		if strings.Contains(line, "Package") || strings.Contains(line, "CPU") || strings.Contains(line, "Core") || strings.Contains(line, "Node") {
			headers = re.Split(line, -1) // split by whitespace
			for i, h := range headers {
				if h == "Bzy_MHz" {
					idxTurbo = i
				} else if h == "PkgWatt" {
					idxTdp = i
				}
			}
			continue
		}
		tokens := re.Split(line, -1)
		if idxTurbo >= 0 {
			allTurbos = append(allTurbos, tokens[idxTurbo])
		}
		if idxTdp >= 0 {
			allTDPs = append(allTDPs, tokens[idxTdp])
		}
	}
	if len(turbos) == 2 {
		singleCoreTurbo = turbos[0] + " MHz"
		allCoreTurbo = turbos[1] + " MHz"
	}
	if len(tdps) == 2 {
		turboTDP = tdps[1] + " Watts"
	}
	return
}

func (s *Source) getIdleTDP() (val string) {
	cmdout := s.getCommandOutputLine("CPU Idle")
	if cmdout != "" && cmdout != "0.00" {
		val = cmdout + " Watts"
	}
	return
}

func (s *Source) getPeakBandwidth(table *Table) (val string) {
	for _, hv := range table.AllHostValues {
		if hv.Name == s.getHostname() {
			var peak float64
			for _, values := range hv.Values {
				if len(values) == 2 {
					bandwidth := values[1]
					bw, err := strconv.ParseFloat(bandwidth, 64)
					if err != nil {
						continue
					}
					peak = math.Max(peak, bw)
				}
			}
			if peak > 0 {
				val = fmt.Sprintf("%.1f GB/s", peak)
			}
			break
		}
	}
	return
}

func (s *Source) getMinLatency(table *Table) (val string) {
	for _, hv := range table.AllHostValues {
		if hv.Name == s.getHostname() {
			var min float64 = math.MaxFloat64
			for _, values := range hv.Values {
				if len(values) == 2 {
					latency := values[0]
					l, err := strconv.ParseFloat(latency, 64)
					if err != nil {
						continue
					}
					min = math.Min(l, min)
				}
			}
			if min < math.MaxFloat64 {
				val = fmt.Sprintf("%.1f ns", min)
			}
			break
		}
	}
	return
}

func (s *Source) getDiskSpeed() (val string) {
	for _, line := range s.getCommandOutputLines("fio") {
		if strings.Contains(line, "read: IOPS") {
			re := regexp.MustCompile(`[=,]`)
			tokens := re.Split(line, 3)
			val = tokens[1] + " iops"
			return
		}
	}
	return
}

func (s *Source) getPowerPerfPolicy() (val string) {
	msrHex := s.getCommandOutputLine("rdmsr 0x1b0")
	msr, err := strconv.ParseInt(msrHex, 16, 0)
	if err == nil {
		if msr < 7 {
			val = "Performance"
		} else if msr > 10 {
			val = "Power"
		} else {
			val = "Balanced"
		}
	}
	return
}

func (s *Source) getTDP() (val string) {
	msrHex := s.getCommandOutputLine("rdmsr 0x610")
	msr, err := strconv.ParseInt(msrHex, 16, 0)
	if err == nil && msr != 0 {
		val = fmt.Sprint(msr/8) + " watts"
	}
	return
}

// get the FwRev for the given device from hdparm
func (s *Source) getDiskFwRev(device string) (fwRev string) {
	reFwRev := regexp.MustCompile(`FwRev=(\w+)`)
	reDev := regexp.MustCompile(fmt.Sprintf(`/dev/%s:`, device))
	devFound := false
	for _, line := range s.getCommandOutputLines("hdparm") {
		if !devFound {
			if reDev.FindString(line) != "" {
				devFound = true
				continue
			}
		} else {
			match := reFwRev.FindStringSubmatch(line)
			if match != nil {
				fwRev = match[1]
				break
			}
		}
	}
	return
}

// get the file system mount options from findmnt
func (s *Source) getMountOptions(filesystem string, mountedOn string) (options string) {
	reFindmnt := regexp.MustCompile(`(.*)\s(.*)\s(.*)\s(.*)`)
	for i, line := range s.getCommandOutputLines("findmnt") {
		if i == 0 {
			continue
		}
		match := reFindmnt.FindStringSubmatch(line)
		if match != nil {
			target := match[1]
			source := match[2]
			if filesystem == source && mountedOn == target {
				options = match[4]
				return
			}
		}
	}
	return
}

// getJavaFolded -- retrieves folded code path frequency data for java processes
func (s *Source) getJavaFolded() (folded string) {
	asyncProfilerOutput := s.getCommandOutputLabeled("analyze", `async-profiler \d+`)
	javaFolded := make(map[string]string)
	re := regexp.MustCompile(`^async-profiler (\d+) (.*)$`)
	for header, stacks := range asyncProfilerOutput {
		if stacks == "" {
			log.Printf("no stacks for: %s", header)
			continue
		}
		match := re.FindStringSubmatch(header)
		if match == nil {
			log.Printf("header didn't match regex: %s", header)
			continue
		}
		pid := match[1]
		processName := match[2]
		_, ok := javaFolded[processName]
		if processName == "" {
			processName = "java (" + pid + ")"
		} else if ok {
			processName = processName + " (" + pid + ")"
		}
		javaFolded[processName] = stacks
	}
	folded, err := mergeJavaFolded(javaFolded)
	if err != nil {
		log.Printf("%v", err)
	}
	return
}

// getSystemFolded -- retrieves folded code path frequency data, i.e., merged output
// from fp and dwarf perf
func (s *Source) getSystemFolded() (folded string) {
	perfSections := s.getCommandOutputLabeled("analyze", `perf_`)
	var dwarfFolded, fpFolded string
	for header, content := range perfSections {
		if header == "pwerf_dwarf" {
			dwarfFolded = content
		} else if header == "perf_fp" {
			fpFolded = content
		}
	}
	folded, err := mergeSystemFolded(fpFolded, dwarfFolded)
	if err != nil {
		log.Printf("error merging folded stacks: %v", err)
	}
	return
}

func (s *Source) getTurboEnabled(family string) (val string) {
	if family == "6" { // Intel
		val = enabledIfValAndTrue(s.valFromRegexSubmatch("cpuid -1", `^Intel Turbo Boost Technology\s*= (.+?)$`))
		return val
	} else if family == "23" || family == "25" { // AMD
		val = s.valFromRegexSubmatch("lscpu", `^Frequency boost.*:\s*(.+?)$`)
		if val != "" {
			val = val + " (AMD Frequency Boost)"
		}
	}
	return
}
