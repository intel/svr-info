package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

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

type Process struct {
	pid  string
	ppid string
	comm string
	cmd  string
}

// match output of, and capture fields from, ps commands used below
var psRegex = `^\s*(\d+)\s+(\d+)\s+([\w\d\(\)\:\/_\-\:\.]+)\s+(.*)`

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

// get maxProcesses processes with highest CPU utilization, matching filter if provided
func getHotProcesses(maxProcesses int, filter string) (processes []Process, err error) {
	// run ps to get list of processes sorted by cpu utilization (descending)
	// e.g., ps -e h -o pid,ppid,comm,cmd ww --sort=-%cpu
	cmd := exec.Command("ps", "-e", "h", "-o", "pid,ppid,comm,cmd", "ww", "--sort=-%cpu")
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
		if !processExists(pid) {
			continue
		}
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
