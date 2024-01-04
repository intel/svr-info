package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Process struct {
	pid  string
	ppid string
	comm string
	cmd  string
}

// pid,ppid,comm,cmd
var psRegex = `^\s*(\d+)\s+(\d+)\s+([\w\d\(\)\:\/_\-\:\.]+)\s+(.*)`

// pid,ppid,comm,cmd,%cpu,cgroup
var psCgroupRegex = `^\s*(\d+)\s+(\d+)\s+([\w\d\(\)\:\/_\-\:\.]+)\s+(.*)\s+(\d+\.?\d+)\s+\d+::(.*)`

func processExists(pid string) (exists bool) {
	exists = false
	path := filepath.Join("/", "proc", pid)
	if fileInfo, err := os.Stat(path); err == nil {
		if fileInfo.Mode().IsDir() {
			exists = true
		}
	}
	return
}

func getProcess(pid string) (process Process, err error) {
	cmd := exec.Command("ps", "-q", pid, "h", "-o", "pid,ppid,comm,cmd", "ww")
	var outBuffer, errBuffer bytes.Buffer
	cmd.Stderr = &errBuffer
	cmd.Stdout = &outBuffer
	if err = cmd.Run(); err != nil {
		return
	}
	psOutput := outBuffer.String()
	reProcess := regexp.MustCompile(psRegex)
	match := reProcess.FindStringSubmatch(psOutput)
	if match == nil {
		err = fmt.Errorf("Process not found, PID: %s, ps output: %s", pid, psOutput)
		return
	}
	process = Process{pid: match[1], ppid: match[2], comm: match[3], cmd: match[4]}
	return
}

func getCgroup(cid string) (cgroupName string, err error) {
	cmd := exec.Command("ps", "-a", "-x", "-h", "-o", "pid,ppid,comm,cmd,cgroup", "--sort=-%cpu")
	var outBuffer, errBuffer bytes.Buffer
	cmd.Stderr = &errBuffer
	cmd.Stdout = &outBuffer
	if err = cmd.Run(); err != nil {
		return
	}
	psOutput := outBuffer.String()
	reCgroup := regexp.MustCompile(psCgroupRegex)
	for _, line := range strings.Split(psOutput, "\n") {
		match := reCgroup.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if strings.Contains(match[6], cid) {
			cgroupName = match[6]
			return
		}
	}
	err = fmt.Errorf("cid not found: %s", cid)
	return
}

func getProcesses(pidList string) (processes []Process, err error) {
	pids := strings.Split(pidList, ",")
	for _, pid := range pids {
		if processExists(pid) {
			var process Process
			if process, err = getProcess(pid); err != nil {
				return
			}
			processes = append(processes, process)
		}
	}
	return
}

func getCgroups(cidList string) (cgroups []string, err error) {
	cids := strings.Split(cidList, ",")
	for _, cid := range cids {
		var cgroup string
		if cgroup, err = getCgroup(cid); err != nil {
			return
		}
		cgroups = append(cgroups, cgroup)
	}
	return
}

// get maxProcesses processes with highest CPU utilization, matching filter if provided
func getHotProcesses(maxProcesses int, filter string) (processes []Process, err error) {
	// run ps to get list of processes sorted by cpu utilization (descending)
	cmd := exec.Command("ps", "-a", "-x", "-h", "-o", "pid,ppid,comm,cmd", "--sort=-%cpu")
	var outBuffer, errBuffer bytes.Buffer
	cmd.Stderr = &errBuffer
	cmd.Stdout = &outBuffer
	if err = cmd.Run(); err != nil {
		return
	}
	psOutput := outBuffer.String()
	var reFilter *regexp.Regexp
	if filter != "" {
		if reFilter, err = regexp.Compile(filter); err != nil {
			return
		}
	}
	reProcess := regexp.MustCompile(psRegex)
	for _, line := range strings.Split(psOutput, "\n") {
		match := reProcess.FindStringSubmatch(line)
		if match == nil {
			log.Printf("Error regex not matching ps output: %s", line)
			continue
		}
		pid := match[1]
		ppid := match[2]
		comm := match[3]
		cmd := match[4]
		if (reFilter != nil && reFilter.MatchString(cmd)) || reFilter == nil {
			processes = append(processes, Process{pid: pid, ppid: ppid, comm: comm, cmd: cmd})
		}
		if len(processes) == maxProcesses {
			break
		}
	}
	if gCmdLineArgs.veryVerbose {
		var pids []string
		for _, process := range processes {
			pids = append(pids, process.pid)
		}
		log.Printf("Hot PIDs: %s", strings.Join(pids, ", "))
	}
	return
}

func getHotCgroups(maxCgroups int, filter string) (cgroups []string, err error) {
	cmd := exec.Command("ps", "-a", "-x", "-h", "-o", "pid,ppid,comm,cmd,%cpu,cgroup", "--sort=-%cpu")
	var outBuffer, errBuffer bytes.Buffer
	cmd.Stderr = &errBuffer
	cmd.Stdout = &outBuffer
	if err = cmd.Run(); err != nil {
		return
	}
	psOutput := outBuffer.String()
	var reFilter *regexp.Regexp
	if filter != "" {
		if reFilter, err = regexp.Compile(filter); err != nil {
			return
		}
	}
	reCgroup := regexp.MustCompile(psCgroupRegex)
	uniqueCgroups := make(map[string]float64)
	for _, line := range strings.Split(psOutput, "\n") {
		match := reCgroup.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		pid := match[1]
		ppid := match[2]
		comm := match[3]
		cmd := match[4]
		cpup := match[5]
		cid := match[6]
		if !strings.Contains(cid, "docker") && !strings.Contains(cid, "containerd") {
			continue
		}
		if !strings.HasSuffix(cid, ".scope") {
			continue
		}
		if gCmdLineArgs.veryVerbose {
			log.Printf("Process with CGroup: %s,%s,%s,%s,%s,%s", pid, ppid, comm, cmd, cpup, cid)
		}
		if reFilter != nil && !reFilter.MatchString(cid) { // must match filter, if provided
			continue
		}
		if _, ok := uniqueCgroups[cid]; !ok {
			uniqueCgroups[cid] = 0
		}
		var utilization float64
		if utilization, err = strconv.ParseFloat(cpup, 64); err != nil {
			return
		}
		uniqueCgroups[cid] += utilization
	}
	// sort aggregated Cgroups by accumulated CPU utilization
	keys := make([]string, 0, len(uniqueCgroups))
	for key := range uniqueCgroups {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return uniqueCgroups[keys[i]] < uniqueCgroups[keys[j]]
	})
	// get the final list of CIDs
	for _, key := range keys {
		cgroups = append(cgroups, key)
		if len(cgroups) == maxCgroups {
			break
		}
	}
	if gCmdLineArgs.veryVerbose {
		log.Printf("Hot CIDs: %s", strings.Join(cgroups, ", "))
	}
	return
}
