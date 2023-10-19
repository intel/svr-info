package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Metadata struct {
	CoresPerSocket      int
	DeviceCounts        map[string]int
	IMCDeviceIDs        []int
	Microarchitecture   string
	ModelName           string
	PerfSupportedEvents string
	RefCyclesSupported  bool
	SocketCount         int
	ThreadsPerCore      int
	TMASupported        bool
	TSC                 int
	TSCFrequencyHz      int
}

func (md Metadata) String() string {
	var uncoreDeviceCounts []string
	for deviceType := range md.DeviceCounts {
		uncoreDeviceCounts = append(uncoreDeviceCounts, fmt.Sprintf("%s: %d", deviceType, md.DeviceCounts[deviceType]))
	}
	return fmt.Sprintf(""+
		"Model Name: %s, "+
		"Microarchitecture: %s, "+
		"Socket Count: %d, "+
		"Cores Per Socket: %d, "+
		"Threads per Core: %d, "+
		"TSC Frequency (Hz): %d, "+
		"TSC: %d, "+
		"Uncore Device Counts: %s, "+
		"ref-cycles supported: %t, "+
		"TMA events supported: %t",
		md.ModelName,
		md.Microarchitecture,
		md.SocketCount,
		md.CoresPerSocket,
		md.ThreadsPerCore,
		md.TSCFrequencyHz,
		md.TSC,
		strings.Join(uncoreDeviceCounts, ", "),
		md.RefCyclesSupported,
		md.TMASupported)
}

// counts uncore device files of format "uncore_<device type>_id" in /sys/devices
func getDeviceCounts() (counts map[string]int, err error) {
	counts = make(map[string]int)
	var paths []string
	var pattern string
	if gDebug {
		pattern = filepath.Join("sys", "devices", "uncore_*")
	} else {
		pattern = filepath.Join("/", "sys", "devices", "uncore_*")
	}
	if paths, err = filepath.Glob(pattern); err != nil {
		return
	}
	for _, path := range paths {
		file := filepath.Base(path)
		fields := strings.Split(file, "_")
		if len(fields) == 3 {
			counts[fields[1]] += 1
		}
	}
	return
}

func getIMCDeviceIds() (ids []int, err error) {
	var pattern string
	if gDebug {
		pattern = filepath.Join("sys", "devices", "uncore_imc_*")
	} else {
		pattern = filepath.Join("/", "sys", "devices", "uncore_imc_*")
	}
	var files []string
	if files, err = filepath.Glob(pattern); err != nil {
		return
	}
	re := regexp.MustCompile(`uncore_imc_(\d+)`)
	for _, fileName := range files {
		match := re.FindStringSubmatch(fileName)
		if match != nil {
			id, _ := strconv.Atoi(match[1])
			ids = append(ids, id)
		}
	}
	return
}

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

func getMicroarchitecture(cpuInfo []map[string]string) (arch string, err error) {
	var family, model, stepping int
	if family, err = strconv.Atoi(cpuInfo[0]["cpu family"]); err != nil {
		err = fmt.Errorf("failed to retrieve cpu family: %v", err)
		return
	}
	if model, err = strconv.Atoi(cpuInfo[0]["model"]); err != nil {
		err = fmt.Errorf("failed to retrieve model: %v", err)
		return
	}
	if stepping, err = strconv.Atoi(cpuInfo[0]["stepping"]); err != nil {
		err = fmt.Errorf("failed to retrieve stepping: %v", err)
		return
	}
	if family != 6 {
		err = fmt.Errorf("non-Intel CPU detected: family=%d", family)
		return
	}
	if model == 79 && stepping == 1 {
		arch = "bdx"
	} else if model == 85 {
		if stepping == 4 {
			arch = "skx"
		} else if stepping >= 5 {
			arch = "clx"
		}
	} else if model == 106 && stepping >= 4 {
		arch = "icx"
	} else if model == 143 && stepping >= 3 {
		arch = "spr"
	} else if model == 207 {
		arch = "emr"
	} else if model == 175 {
		arch = "srf"
	} else {
		err = fmt.Errorf("unrecognized Intel architecture: model=%d, stepping=%d", model, stepping)
		return
	}
	return
}

func getPerfSupportedEvents() (supportedEvents string, err error) {
	cmd := exec.Command("perf", "list")
	var bytes []byte
	if bytes, err = cmd.Output(); err != nil {
		return
	}
	supportedEvents = string(bytes)
	return
}

func getRefCyclesSupported() (supported bool, err error) {
	cmd := exec.Command("perf", "stat", "-a", "-e", "ref-cycles", "sleep", ".1")
	var bytes []byte
	if bytes, err = cmd.Output(); err != nil {
		return
	}
	supported = !strings.Contains(string(bytes), "<not supported>")
	return
}

func getTMASupported() (supported bool, err error) {
	cmd := exec.Command("perf", "stat", "-a", "-e", "'{cpu/event=0x00,umask=0x04,period=10000003,name='TOPDOWN.SLOTS'/,cpu/event=0x00,umask=0x81,period=10000003,name='PERF_METRICS.BAD_SPECULATION'/}'", "sleep", ".1")
	var bytes []byte
	if bytes, err = cmd.Output(); err != nil {
		err = nil
		supported = false
		return
	}
	vals := make(map[string]float64)
	lines := strings.Split(string(bytes), "\n")
	for _, line := range lines {
		if strings.Contains(line, "TOPDOWN.SLOTS") || strings.Contains(line, "PERF_METRICS.BAD_SPECULATION") {
			fields := strings.Split(strings.TrimSpace(line), " ")
			if len(fields) >= 2 {
				val, err := strconv.ParseFloat(fields[len(fields)-1], 64)
				if err != nil {
					continue
				}
				vals[fields[0]] = val
			}
		}
	}
	supported = !(vals["TOPDOWN.SLOTS"] == vals["PERF_METRICS.BAD_SPECULATION"])
	return

}

func loadMetadata() (metadata Metadata, err error) {
	// reduce startup time by running the two perf commands in their own threads while
	// the rest of the metadata is being collected
	slowFuncChannel := make(chan error)
	// perf list
	go func() {
		var err error
		if metadata.PerfSupportedEvents, err = getPerfSupportedEvents(); err != nil {
			if gDebug {
				err = nil
			} else {
				err = fmt.Errorf("failed to load perf list: %v", err)
			}
		}
		slowFuncChannel <- err
	}()
	// ref_cycles
	go func() {
		var err error
		if metadata.RefCyclesSupported, err = getRefCyclesSupported(); err != nil {
			if gDebug {
				err = nil
			} else {
				err = fmt.Errorf("failed to determine if ref_cycles is supported: %v", err)
			}
		}
		slowFuncChannel <- err
	}()
	// TMA
	go func() {
		var err error
		if metadata.TMASupported, err = getTMASupported(); err != nil {
			if gDebug {
				err = nil
			} else {
				err = fmt.Errorf("failed to determine if TMA is supported: %v", err)
			}
		}
		slowFuncChannel <- err
	}()
	defer func() {
		var errs []error
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
	// TODO: does this account for off-lined cores?
	metadata.CoresPerSocket, err = strconv.Atoi(cpuInfo[0]["cpu cores"])
	if err != nil {
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
	// System TSC Frequency
	metadata.TSCFrequencyHz = GetTSCFreqMHz() * 1000000
	// calculate TSC
	metadata.TSC = metadata.SocketCount * metadata.CoresPerSocket * metadata.ThreadsPerCore * metadata.TSCFrequencyHz
	// /sys/devices counts
	if metadata.DeviceCounts, err = getDeviceCounts(); err != nil {
		return
	}
	// uncore imc device ids (may not be consecutive)
	if metadata.IMCDeviceIDs, err = getIMCDeviceIds(); err != nil {
		return
	}
	// Model Name
	metadata.ModelName = cpuInfo[0]["model name"]
	// CPU microarchitecture
	metadata.Microarchitecture, err = getMicroarchitecture(cpuInfo)
	if err != nil {
		// TODO: remove this override used for development/debugging
		if gDebug {
			err = nil
			metadata.Microarchitecture = "spr"
		} else {
			err = fmt.Errorf("failed to retrieve microarchitecture: %v", err)
			return
		}
	}
	return
}
