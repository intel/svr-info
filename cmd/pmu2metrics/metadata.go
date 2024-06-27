/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
//
// defines a structure and a loading funciton to hold information about the platform to be
// used during data collection and metric production
//
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/intel/svr-info/internal/cpudb"
	"gopkg.in/yaml.v2"
)

// Metadata is the representation of the platform's state and capabilities
type Metadata struct {
	CoresPerSocket           int `yaml:"CoresPerSocket"`
	CPUSocketMap             map[int]int
	DeviceIDs                map[string][]int `yaml:"DeviceIDs"`
	FixedCounterTMASupported bool             `yaml:"FixedCounterTMASupported"`
	Microarchitecture        string           `yaml:"Microarchitecture"`
	ModelName                string
	PerfSupportedEvents      string `yaml:"PerfSupportedEvents"`
	PMUDriverVersion         string `yaml:"PMUDriverVersion"`
	RefCyclesSupported       bool   `yaml:"RefCyclesSupported"`
	SocketCount              int    `yaml:"SocketCount"`
	ThreadsPerCore           int    `yaml:"ThreadsPerCore"`
	TSC                      int    `yaml:"TSC"`
	TSCFrequencyHz           int    `yaml:"TSCFrequencyHz"`
}

// LoadMetadata - populates and returns a Metadata structure containing state of the
// system.
func LoadMetadata(perfPath string) (metadata Metadata, err error) {
	// PMU driver version
	metadata.PMUDriverVersion, err = getPMUDriverVersion()
	if err != nil {
		err = fmt.Errorf("failed to retrieve PMU driver version: %v", err)
		return
	}
	// reduce startup time by running the three perf commands in their own threads while
	// the rest of the metadata is being collected
	slowFuncChannel := make(chan error)
	// perf list
	go func() {
		var err error
		if metadata.PerfSupportedEvents, err = getPerfSupportedEvents(perfPath); err != nil {
			err = fmt.Errorf("failed to load perf list: %v", err)
		}
		slowFuncChannel <- err
	}()
	// ref_cycles
	go func() {
		var err error
		var output string
		if metadata.RefCyclesSupported, output, err = getRefCyclesSupported(perfPath); err != nil {
			err = fmt.Errorf("failed to determine if ref_cycles is supported: %v", err)
		}
		if !metadata.RefCyclesSupported && gCmdLineArgs.verbose {
			log.Printf("ref-cycles not supported:\n%s\n", output)
		}
		slowFuncChannel <- err
	}()
	// Fixed-counter TMA events
	go func() {
		var err error
		var output string
		if metadata.FixedCounterTMASupported, output, err = getFixedCounterTMASupported(perfPath); err != nil {
			err = fmt.Errorf("failed to determine if TMA is supported: %v", err)
		}
		if !metadata.FixedCounterTMASupported && gCmdLineArgs.verbose {
			log.Printf("TMA fixed counter not supported:\n%s\n", output)
		}
		slowFuncChannel <- err
	}()
	defer func() {
		var errs []error
		errs = append(errs, <-slowFuncChannel)
		errs = append(errs, <-slowFuncChannel)
		errs = append(errs, <-slowFuncChannel)
		for _, errInside := range errs {
			if errInside != nil {
				if err == nil {
					err = errInside
				} else {
					err = fmt.Errorf("%v : %v", err, errInside)
				}
			}
		}
	}()
	// CPU Info
	var cpuInfo []map[string]string
	cpuInfo, err = getCPUInfo()
	if err != nil || len(cpuInfo) < 1 {
		err = fmt.Errorf("failed to read cpu info: %v", err)
		return
	}
	// Core Count (per socket)
	metadata.CoresPerSocket, err = strconv.Atoi(cpuInfo[0]["cpu cores"])
	if err != nil || metadata.CoresPerSocket == 0 {
		err = fmt.Errorf("failed to retrieve cores per socket: %v", err)
		return
	}
	// Socket Count
	var maxPhysicalID int
	if maxPhysicalID, err = strconv.Atoi(cpuInfo[len(cpuInfo)-1]["physical id"]); err != nil {
		err = fmt.Errorf("failed to retrieve max physical id: %v", err)
		return
	}
	metadata.SocketCount = maxPhysicalID + 1
	// Hyperthreading - threads per core
	if cpuInfo[0]["siblings"] != cpuInfo[0]["cpu cores"] {
		metadata.ThreadsPerCore = 2
	} else {
		metadata.ThreadsPerCore = 1
	}
	// CPUSocketMap
	metadata.CPUSocketMap = createCPUSocketMap(metadata.CoresPerSocket, metadata.SocketCount, metadata.ThreadsPerCore == 2)
	// System TSC Frequency
	metadata.TSCFrequencyHz = GetTSCFreqMHz() * 1000000
	// calculate TSC
	metadata.TSC = metadata.SocketCount * metadata.CoresPerSocket * metadata.ThreadsPerCore * metadata.TSCFrequencyHz
	// uncore device IDs
	if metadata.DeviceIDs, err = getUncoreDeviceIDs(); err != nil {
		return
	}
	// Model Name
	metadata.ModelName = cpuInfo[0]["model name"]
	// CPU microarchitecture
	var cpu cpudb.CPU
	cpu, err = cpudb.NewCPUDB().GetCPU(cpuInfo[0]["cpu family"], cpuInfo[0]["model"], cpuInfo[0]["stepping"], "", "", "")
	if err != nil {
		return
	}
	metadata.Microarchitecture = cpu.Architecture
	return
}

// LoadMetadataFromFile - used for testing and debugging only
// needed for generating metrics:
// CoresPerSocket      int
// Microarchitecture   string
// SocketCount         int
// ThreadsPerCore      int
// TSC                 int
// TSCFrequencyHz      int
func LoadMetadataFromFile(metadataFilePath string) (metadata Metadata, err error) {
	var yamlData []byte
	if yamlData, err = os.ReadFile(metadataFilePath); err != nil {
		return
	}
	if err = yaml.UnmarshalStrict([]byte(yamlData), &metadata); err != nil {
		return
	}
	if metadata.CoresPerSocket == 0 || metadata.SocketCount == 0 {
		err = fmt.Errorf("cores per socket and socket count cannot be zero")
		return
	}
	metadata.CPUSocketMap = createCPUSocketMap(metadata.CoresPerSocket, metadata.SocketCount, metadata.ThreadsPerCore == 2)
	return
}

// String - provides a string representation of the Metadata structure
func (md Metadata) String() string {
	out := fmt.Sprintf(""+
		"Model Name: %s, "+
		"Microarchitecture: %s, "+
		"Socket Count: %d, "+
		"Cores Per Socket: %d, "+
		"Threads per Core: %d, "+
		"TSC Frequency (Hz): %d, "+
		"TSC: %d, "+
		"ref-cycles supported: %t, "+
		"Fixed Counter TMA events supported: %t, "+
		"PMU Driver version: %s, ",
		md.ModelName,
		md.Microarchitecture,
		md.SocketCount,
		md.CoresPerSocket,
		md.ThreadsPerCore,
		md.TSCFrequencyHz,
		md.TSC,
		md.RefCyclesSupported,
		md.FixedCounterTMASupported,
		md.PMUDriverVersion)
	for deviceName, deviceIds := range md.DeviceIDs {
		var ids []string
		for _, id := range deviceIds {
			ids = append(ids, fmt.Sprintf("%d", id))
		}
		out += fmt.Sprintf("%s: [%s] ", deviceName, strings.Join(ids, ","))
	}
	return out
}

// WriteJSONToFile writes the metadata structure (minus perf's supported events) to the filename provided
// Note that the file will be truncated.
func (md Metadata) WriteJSONToFile(path string) (err error) {
	var rawFile *os.File
	if rawFile, err = os.OpenFile(gCmdLineArgs.rawFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644); err != nil {
		log.Printf("failed to open raw file for writing, %v", err)
		return
	}
	defer rawFile.Close()
	var out []byte
	mdCopy := md
	mdCopy.PerfSupportedEvents = ""
	if out, err = json.Marshal(mdCopy); err != nil {
		log.Printf("failed to marshal metadata structure, %v", err)
		return
	}
	out = append(out, []byte("\n")...)
	if _, err = rawFile.Write(out); err != nil {
		log.Printf("failed to write metadata json to file, %v", err)
		return
	}
	return
}

// getUncoreDeviceIDs - returns a map of device type to list of device indices
// e.g., "upi" -> [0,1,2,3],
func getUncoreDeviceIDs() (IDs map[string][]int, err error) {
	pattern := filepath.Join("/", "sys", "bus", "event_source", "devices", "uncore_*")
	var fileNames []string
	if fileNames, err = filepath.Glob(pattern); err != nil {
		return
	}
	IDs = make(map[string][]int)
	re := regexp.MustCompile(`uncore_(.*)_(\d+)`)
	for _, fileName := range fileNames {
		match := re.FindStringSubmatch(fileName)
		if match == nil {
			continue
		}
		var id int
		if id, err = strconv.Atoi(match[2]); err != nil {
			return
		}
		IDs[match[1]] = append(IDs[match[1]], id)
	}
	return
}

// getCPUInfo - reads and returns all data from /proc/cpuinfo
func getCPUInfo() (cpuInfo []map[string]string, err error) {
	var file fs.File
	if file, err = os.Open("/proc/cpuinfo"); err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	oneCPUInfo := make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")
		if len(fields) < 2 {
			cpuInfo = append(cpuInfo, oneCPUInfo)
			oneCPUInfo = make(map[string]string)
			continue
		}
		oneCPUInfo[strings.TrimSpace(fields[0])] = strings.TrimSpace(fields[1])
	}
	return
}

// getPerfSupportedEvents - returns a string containing the output from
// 'perf list'
func getPerfSupportedEvents(perfPath string) (supportedEvents string, err error) {
	cmd := exec.Command(perfPath, "list")
	var bytes []byte
	if bytes, err = cmd.Output(); err != nil {
		return
	}
	supportedEvents = string(bytes)
	return
}

// getRefCyclesSupported() - checks if the ref-cycles event is supported by perf
func getRefCyclesSupported(perfPath string) (supported bool, output string, err error) {
	cmd := exec.Command(perfPath, "stat", "-a", "-e", "ref-cycles", "sleep", ".1")
	var outBuffer, errBuffer bytes.Buffer
	cmd.Stderr = &errBuffer
	cmd.Stdout = &outBuffer
	if err = cmd.Run(); err != nil {
		return
	}
	output = errBuffer.String()
	supported = !strings.Contains(output, "<not supported>")
	return
}

// getFixedCounterTMASupported - checks if the fixed TMA counter events are supported by perf
// We check for the TOPDOWN.SLOTS and PERF_METRICS.BAD_SPECULATION events as an indicator of support for fixed TMA counter support
func getFixedCounterTMASupported(perfPath string) (supported bool, output string, err error) {
	cmd := exec.Command(perfPath, "stat", "-a", "-e", "'{cpu/event=0x00,umask=0x04,period=10000003,name='TOPDOWN.SLOTS'/,cpu/event=0x00,umask=0x81,period=10000003,name='PERF_METRICS.BAD_SPECULATION'/}'", "sleep", ".1")
	var outBuffer, errBuffer bytes.Buffer
	cmd.Stderr = &errBuffer
	cmd.Stdout = &outBuffer
	if err = cmd.Run(); err != nil {
		// err from perf stat is 1st indication that these events are not supported, so return a nil error
		supported = false
		output = fmt.Sprint(err)
		err = nil
		return
	}
	// event values being zero or equal to each other is 2nd indication that these events are not (properly) supported
	output = errBuffer.String()
	vals := make(map[string]float64)
	lines := strings.Split(output, "\n")
	// example line: "         784333932      TOPDOWN.SLOTS                                                        (59.75%)"
	re := regexp.MustCompile(`\s+(\d+)\s+(\w*\.*\w*)\s+.*`)
	for _, line := range lines {
		// count may include commas as thousands separators, remove them
		line := strings.ReplaceAll(line, ",", "")
		match := re.FindStringSubmatch(line)
		if match != nil {
			vals[match[2]], err = strconv.ParseFloat(match[1], 64)
			if err != nil {
				// this should never happen
				panic("failed to parse float")
			}
		}
	}
	topDownSlots := vals["TOPDOWN.SLOTS"]
	badSpeculation := vals["PERF_METRICS.BAD_SPECULATION"]
	supported = topDownSlots != badSpeculation && topDownSlots != 0 && badSpeculation != 0
	return
}

func getPMUDriverVersion() (version string, err error) {
	cmd := exec.Command("sh", "-c", `dmesg | grep -A 1 "Intel PMU driver" | tail -1 | awk '{print $NF}'`)
	var outBuffer, errBuffer bytes.Buffer
	cmd.Stderr = &errBuffer
	cmd.Stdout = &outBuffer
	if err = cmd.Run(); err != nil {
		return
	}
	version = strings.TrimSpace(outBuffer.String())
	return
}

// createCPUSocketMap creates a map from CPU number to socket number
func createCPUSocketMap(coresPerSocket int, sockets int, hyperthreading bool) (cpuSocketMap map[int]int) {
	// Create an empty map
	cpuSocketMap = make(map[int]int)

	// Calculate the total number of logical CPUs
	totalCPUs := coresPerSocket * sockets
	if hyperthreading {
		totalCPUs *= 2 // hyperthreading doubles the number of logical CPUs
	}
	// Assign each CPU to a socket
	for i := 0; i < totalCPUs; i++ {
		// Assume that the CPUs are evenly distributed between the sockets
		socket := i / coresPerSocket
		if hyperthreading {
			// With non-adjacent hyperthreading, the second logical CPU of each core is in the second half
			if i >= totalCPUs/2 {
				socket = (i - totalCPUs/2) / coresPerSocket
			}
		}
		// Store the mapping
		cpuSocketMap[i] = socket
	}
	return cpuSocketMap
}
