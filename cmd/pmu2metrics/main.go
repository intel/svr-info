/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
//
// Command line interface and program logic
//
package main

import (
	"bufio"
	"embed"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"log/syslog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// globals
var (
	gVersion             string = "dev"
	gCmdLineArgs         CmdLineArgs
	gCollectionStartTime time.Time
)

// Granularity represents the requested granularity level for produced metrics
type Granularity int

const (
	GranularitySystem Granularity = iota
	GranularitySocket
	GranularityCPU
)

var GranularityOptions = []string{"system", "socket", "cpu"}

// Scope represents the requested scope of event collection
type Scope int

const (
	ScopeSystem Scope = iota
	ScopeProcess
	ScopeCgroup
)

var ScopeOptions = []string{"system", "process", "cgroup"}

// Format represents the format of the metric output
type Format int

const (
	FormatHuman Format = iota
	FormatCSV
	FormatWide
)

var FormatOptions = []string{"human", "csv", "wide"}

// Summary represents the format of the post-processed summary report
type Summary int

const (
	SummaryCSV Summary = iota
	SummaryHTML
)

var SummaryOptions = []string{"csv", "html"}

// CmdLineArgs represents the program arguments provided by the user
type CmdLineArgs struct {
	showHelp    bool
	showVersion bool
	// collection options
	timeout int // seconds
	// collection options
	scope   Scope
	pidList string
	cidList string
	filter  string
	count   int
	refresh int // seconds
	// post-processing options
	inputCSVFilePath string
	summaryFormat    Summary
	// output format options
	granularity  Granularity
	metricsList  string
	outputFormat Format
	verbose      bool
	veryVerbose  bool
	// advanced options
	showMetricNames   bool
	syslog            bool
	eventFilePath     string
	metricFilePath    string
	perfPrintInterval int // milliseconds
	perfMuxInterval   int // milliseconds
	// debugging options
	metadataFilePath string
	perfStatFilePath string
}

//go:embed resources
var resources embed.FS

// extractExecutableResources extracts executables from embedded resources to temporary directory
func extractExecutableResources(tempDir string) (err error) {
	toolNames := []string{"perf"}
	for _, toolName := range toolNames {
		// get the exe from our embedded resources
		var toolBytes []byte
		toolBytes, err = resources.ReadFile("resources/" + toolName)
		if err != nil {
			return
		}
		toolPath := filepath.Join(tempDir, toolName)
		var f *os.File
		f, err = os.OpenFile(toolPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0744)
		if err != nil {
			return
		}
		defer f.Close()
		err = binary.Write(f, binary.LittleEndian, toolBytes)
		if err != nil {
			return
		}
	}
	return
}

// resourceExists confirms that file of provided filename exists in the embedded
// resources
func resourceExists(filename string) (exists bool) {
	f, err := resources.Open(filepath.Join("resources", filename))
	if err != nil {
		exists = false
		return
	}
	f.Close()
	exists = true
	return
}

// getPerfPath returns the path to the perf executable that will be used to collect
// events. If the perf binary is included in the embedded resources, it will be extracted
// to a temporary directory and run from there, otherwise the system-installed perf will
// be used.
func getPerfPath() (path string, tempDir string, err error) {
	if resourceExists("perf") {
		if tempDir, err = os.MkdirTemp("", fmt.Sprintf("%s.tmp.", filepath.Base(os.Args[0]))); err != nil {
			log.Printf("failed to create temporary directory: %v", err)
			return
		}
		if err = extractExecutableResources(tempDir); err != nil {
			log.Printf("failed to extract executable resources to %s: %v", "", err)
			return
		}
		path = filepath.Join(tempDir, "perf")
	} else {
		path, err = exec.LookPath("perf")
	}
	return
}

// printMetrics prints one frame of metrics to stdout in the format requested by the user. The
// frameCount argument is used to control when the headers are printed, e.g., on the first frame
// only.
func printMetrics(metricFrame MetricFrame, frameCount int) {
	if gCmdLineArgs.outputFormat == FormatCSV {
		if frameCount == 1 {
			fmt.Print("TS,SKT,CPU,PID,CMD,CID,")
			names := make([]string, 0, len(metricFrame.Metrics))
			for _, metric := range metricFrame.Metrics {
				names = append(names, metric.Name)
			}
			fmt.Printf("%s\n", strings.Join(names, ","))
		}
		fmt.Printf("%d,%s,%s,%s,%s,%s,", gCollectionStartTime.Unix()+int64(metricFrame.Timestamp), metricFrame.Socket, metricFrame.CPU, metricFrame.PID, metricFrame.Cmd, metricFrame.Cgroup)
		values := make([]string, 0, len(metricFrame.Metrics))
		for _, metric := range metricFrame.Metrics {
			values = append(values, strconv.FormatFloat(metric.Value, 'g', 8, 64))
		}
		fmt.Printf("%s\n", strings.ReplaceAll(strings.Join(values, ","), "NaN", ""))
	} else {
		if gCmdLineArgs.outputFormat == FormatHuman {
			fmt.Println("--------------------------------------------------------------------------------------")
			fmt.Printf("- Metrics captured at %s\n", gCollectionStartTime.Add(time.Second*time.Duration(int(metricFrame.Timestamp))).UTC())
			if metricFrame.PID != "" {
				fmt.Printf("- PID: %s\n", metricFrame.PID)
				fmt.Printf("- CMD: %s\n", metricFrame.Cmd)
			} else if metricFrame.Cgroup != "" {
				fmt.Printf("- CID: %s\n", metricFrame.Cgroup)
			}
			if metricFrame.CPU != "" {
				fmt.Printf("- CPU: %s\n", metricFrame.CPU)
			} else if metricFrame.Socket != "" {
				fmt.Printf("- Socket: %s\n", metricFrame.Socket)
			}
			fmt.Println("--------------------------------------------------------------------------------------")
			fmt.Printf("%-70s %15s\n", "metric", "value")
			fmt.Printf("%-70s %15s\n", "------------------------", "----------")
			for _, metric := range metricFrame.Metrics {
				fmt.Printf("%-70s %15s\n", metric.Name, strconv.FormatFloat(metric.Value, 'g', 4, 64))
			}
		} else { // wide format
			var names []string
			var values []float64
			for _, metric := range metricFrame.Metrics {
				names = append(names, metric.Name)
				values = append(values, metric.Value)
			}
			minColWidth := 6
			colSpacing := 3
			if frameCount == 1 { // print headers
				header := "Timestamp    " // 10 + 3
				if metricFrame.PID != "" {
					header += "PID       "         // 7 + 3
					header += "Command           " // 15 + 3
				} else if metricFrame.Cgroup != "" {
					header += "CID       "
				}
				if metricFrame.CPU != "" {
					header += "CPU   " // 3 + 3
				} else if metricFrame.Socket != "" {
					header += "SKT   " // 3 + 3
				}
				for _, name := range names {
					extend := 0
					if len(name) < minColWidth {
						extend = minColWidth - len(name)
					}
					header += fmt.Sprintf("%s%*s%*s", name, extend, "", colSpacing, "")
				}
				fmt.Println(header)
			}
			// handle values
			TimestampColWidth := 10
			formattedTimestamp := fmt.Sprintf("%d", gCollectionStartTime.Unix()+int64(metricFrame.Timestamp))
			row := fmt.Sprintf("%s%*s%*s", formattedTimestamp, TimestampColWidth-len(formattedTimestamp), "", colSpacing, "")
			if metricFrame.PID != "" {
				PIDColWidth := 7
				commandColWidth := 15
				row += fmt.Sprintf("%s%*s%*s", metricFrame.PID, PIDColWidth-len(metricFrame.PID), "", colSpacing, "")
				var command string
				if len(metricFrame.Cmd) <= commandColWidth {
					command = metricFrame.Cmd
				} else {
					command = metricFrame.Cmd[:commandColWidth]
				}
				row += fmt.Sprintf("%s%*s%*s", command, commandColWidth-len(command), "", colSpacing, "")
			} else if metricFrame.Cgroup != "" {
				CIDColWidth := 7
				row += fmt.Sprintf("%s%*s%*s", metricFrame.Cgroup, CIDColWidth-len(metricFrame.Cgroup), "", colSpacing, "")
			}
			if metricFrame.CPU != "" {
				CPUColWidth := 3
				row += fmt.Sprintf("%s%*s%*s", metricFrame.CPU, CPUColWidth-len(metricFrame.CPU), "", colSpacing, "")
			} else if metricFrame.Socket != "" {
				SKTColWidth := 3
				row += fmt.Sprintf("%s%*s%*s", metricFrame.Socket, SKTColWidth-len(metricFrame.Socket), "", colSpacing, "")
			}
			// handle the metric values
			for i, value := range values {
				colWidth := max(len(names[i]), minColWidth)
				formattedVal := fmt.Sprintf("%.2f", value)
				row += fmt.Sprintf("%s%*s%*s", formattedVal, colWidth-len(formattedVal), "", colSpacing, "")
			}
			fmt.Println(row)
		}
	}
}

// getPerfCommandArgs assembles the arguments that will be passed to Linux perf
func getPerfCommandArgs(pid string, cgroups []string, timeout int, eventGroups []GroupDefinition, metadata Metadata) (args []string, err error) {
	// -I: print interval in ms
	// -j: json formatted event output
	args = append(args, "stat", "-I", fmt.Sprintf("%d", gCmdLineArgs.perfPrintInterval), "-j")
	if gCmdLineArgs.scope == ScopeSystem {
		args = append(args, "-a") // system-wide collection
		if gCmdLineArgs.granularity == GranularityCPU || gCmdLineArgs.granularity == GranularitySocket {
			args = append(args, "-A") // no aggregation
		}
	} else if gCmdLineArgs.scope == ScopeProcess {
		args = append(args, "-p", pid) // collect only for this process
	} else if gCmdLineArgs.scope == ScopeCgroup {
		args = append(args, "--for-each-cgroup", strings.Join(cgroups, ",")) // collect only for these cgroups
	}
	// -i: event groups to collect
	args = append(args, "-e")
	var groups []string
	for _, group := range eventGroups {
		var events []string
		for _, event := range group {
			events = append(events, event.Raw)
		}
		groups = append(groups, fmt.Sprintf("{%s}", strings.Join(events, ",")))
	}
	args = append(args, fmt.Sprintf("'%s'", strings.Join(groups, ",")))
	// add timeout, if applicable
	if gCmdLineArgs.scope != ScopeCgroup && timeout != 0 {
		args = append(args, "sleep", fmt.Sprintf("%d", timeout))
	}
	return
}

// getPerfCommands is responsible for assembling the command(s) that will be
// executed to collect event data
func getPerfCommands(perfPath string, eventGroups []GroupDefinition, metadata Metadata) (processes []Process, perfCommands []*exec.Cmd, err error) {
	if gCmdLineArgs.scope == ScopeSystem {
		var args []string
		if args, err = getPerfCommandArgs("", []string{}, gCmdLineArgs.timeout, eventGroups, metadata); err != nil {
			err = fmt.Errorf("failed to assemble perf args: %v", err)
			return
		}
		cmd := exec.Command(perfPath, args...)
		perfCommands = append(perfCommands, cmd)
	} else if gCmdLineArgs.scope == ScopeProcess {
		if gCmdLineArgs.pidList != "" {
			if processes, err = GetProcesses(gCmdLineArgs.pidList); err != nil {
				return
			}
		} else {
			if processes, err = GetHotProcesses(gCmdLineArgs.count, gCmdLineArgs.filter); err != nil {
				return
			}
		}
		if len(processes) == 0 {
			err = fmt.Errorf("no PIDs selected")
			return
		}
		var timeout int
		if gCmdLineArgs.timeout > 0 && gCmdLineArgs.timeout < gCmdLineArgs.refresh {
			timeout = gCmdLineArgs.timeout
		} else {
			timeout = gCmdLineArgs.refresh
		}
		for _, process := range processes {
			var args []string
			if args, err = getPerfCommandArgs(process.pid, []string{}, timeout, eventGroups, metadata); err != nil {
				err = fmt.Errorf("failed to assemble perf args: %v", err)
				return
			}
			cmd := exec.Command(perfPath, args...)
			perfCommands = append(perfCommands, cmd)
		}
	} else if gCmdLineArgs.scope == ScopeCgroup {
		var cgroups []string
		if gCmdLineArgs.cidList != "" {
			if cgroups, err = GetCgroups(gCmdLineArgs.cidList); err != nil {
				return
			}
		} else {
			if cgroups, err = GetHotCgroups(gCmdLineArgs.count, gCmdLineArgs.filter); err != nil {
				return
			}
		}
		if len(cgroups) == 0 {
			err = fmt.Errorf("no CIDs selected")
			return
		}
		var args []string
		if args, err = getPerfCommandArgs("", cgroups, -1, eventGroups, metadata); err != nil {
			err = fmt.Errorf("failed to assemble perf args: %v", err)
			return
		}
		cmd := exec.Command(perfPath, args...)
		perfCommands = append(perfCommands, cmd)
	}
	return
}

// runPerf starts Linux perf using the provided command, then reads perf's output
// until perf stops. When collecting for cgroups, perf will be manually terminated if/when the
// run duration exceeds the collection time or the time when the cgroup list needs
// to be refreshed.
func runPerf(process Process, cmd *exec.Cmd, eventGroupDefinitions []GroupDefinition, metricDefinitions []MetricDefinition, metadata Metadata, frameChannel chan MetricFrame, errorChannel chan error) {
	var err error
	defer func() { errorChannel <- err }()
	reader, _ := cmd.StderrPipe()
	if gCmdLineArgs.veryVerbose {
		log.Printf("perf command: %s", cmd)
	}
	scanner := bufio.NewScanner(reader)
	cpuCount := metadata.SocketCount * metadata.CoresPerSocket * metadata.ThreadsPerCore
	outputLines := make([][]byte, 0, cpuCount*150) // a rough approximation of expected number of events
	// start perf
	if err = cmd.Start(); err != nil {
		err = fmt.Errorf("failed to run perf: %v", err)
		log.Printf("%v", err)
		return
	}
	// must manually terminate perf in cgroup scope when a timeout is specified and/or need to refresh cgroups
	startPerfTimestamp := time.Now()
	var timeout int
	if gCmdLineArgs.scope == ScopeCgroup && (gCmdLineArgs.timeout != 0 || gCmdLineArgs.cidList == "") {
		if gCmdLineArgs.timeout > 0 && gCmdLineArgs.timeout < gCmdLineArgs.refresh {
			timeout = gCmdLineArgs.timeout
		} else {
			timeout = gCmdLineArgs.refresh
		}
	}
	// Use a timer to determine when we received an entire frame of events from perf
	// The timer will expire when no lines (events) have been received from perf for more than 100ms. This
	// works because perf writes the events to stderr in a burst every collection interval, e.g., 5 seconds.
	// When the timer expires, this code assumes that perf is done writing events to stderr.
	// The first duration needs to be longer than the time it takes for perf to print its first line of output.
	t1 := time.NewTimer(time.Duration(2 * gCmdLineArgs.perfPrintInterval))
	var frameTimestamp float64
	frameCount := 0
	go func() {
		for {
			<-t1.C // waits for timer to expire
			if len(outputLines) != 0 {
				var metricFrames []MetricFrame
				if metricFrames, frameTimestamp, err = ProcessEvents(outputLines, eventGroupDefinitions, metricDefinitions, process, frameTimestamp, metadata); err != nil {
					log.Printf("%v", err)
					return
				}
				for _, metricFrame := range metricFrames {
					frameCount += 1
					metricFrame.FrameCount = frameCount
					frameChannel <- metricFrame
					outputLines = [][]byte{} // empty it
				}
			}
			if timeout != 0 && int(time.Since(startPerfTimestamp).Seconds()) > timeout {
				cmd.Process.Signal(os.Interrupt)
			}
		}
	}()
	// read perf output
	for scanner.Scan() { // blocks waiting for next token (line), loop terminated (Scan returns false) when file empty/closed
		line := scanner.Text()
		if gCmdLineArgs.veryVerbose {
			log.Print(line)
		}
		t1.Stop()
		t1.Reset(100 * time.Millisecond) // 100ms is somewhat arbitrary, but seems to work
		outputLines = append(outputLines, []byte(line))
	}
	t1.Stop()
	if len(outputLines) != 0 {
		var metricFrames []MetricFrame
		if metricFrames, frameTimestamp, err = ProcessEvents(outputLines, eventGroupDefinitions, metricDefinitions, process, frameTimestamp, metadata); err != nil {
			log.Printf("%v", err)
			return
		}
		for _, metricFrame := range metricFrames {
			frameCount += 1
			metricFrame.FrameCount = frameCount
			frameChannel <- metricFrame
		}
	}
	// wait for perf stat to exit
	if err = cmd.Wait(); err != nil {
		if strings.Contains(err.Error(), "signal") { // perf received kill signal, ignore
			err = nil
		} else {
			err = fmt.Errorf("error from perf on exit: %v", err)
			log.Printf("%v", err)
		}
		return
	}
}

// receiveMetrics prints metrics that it receives over the provided channel
func receiveMetrics(frameChannel chan MetricFrame) {
	totalFrameCount := 0
	// block until next frame of metrics arrives, will exit loop when channel is closed
	for frame := range frameChannel {
		totalFrameCount++
		printMetrics(frame, totalFrameCount)
	}
}

// doWork is the primary application event loop. It sets up the goroutines and
// communication channels, runs perf, restarts perf (if necessary), etc.
func doWork(perfPath string, eventGroupDefinitions []GroupDefinition, metricDefinitions []MetricDefinition, metadata Metadata) (err error) {
	// refresh if collecting per-process/cgroup and list of PIDs/CIDs not specified
	refresh := (gCmdLineArgs.scope == ScopeProcess && gCmdLineArgs.pidList == "") ||
		(gCmdLineArgs.scope == ScopeCgroup && gCmdLineArgs.cidList == "")
	errorChannel := make(chan error)
	frameChannel := make(chan MetricFrame)
	totalRuntimeSeconds := 0 // only relevant in process scope
	go receiveMetrics(frameChannel)
	for {
		// get current time for use in setting timestamps on output
		gCollectionStartTime = time.Now()
		var perfCommands []*exec.Cmd
		var processes []Process
		// One perf command when in system or cgroup scope and one or more perf commands when in process scope.
		if processes, perfCommands, err = getPerfCommands(perfPath, eventGroupDefinitions, metadata); err != nil {
			break
		}
		beginTimestamp := time.Now()
		for i, cmd := range perfCommands {
			var process Process
			if len(processes) > i {
				process = processes[i]
			}
			go runPerf(process, cmd, eventGroupDefinitions, metricDefinitions, metadata, frameChannel, errorChannel)
		}
		// wait for all runPerf goroutines to finish
		var perfErrors []error
		for range perfCommands {
			perfErr := <-errorChannel // capture and return all errors
			if perfErr != nil {
				perfErrors = append(perfErrors, perfErr)
			}
		}
		endTimestamp := time.Now()
		if len(perfErrors) > 0 {
			var errStrings []string
			for _, perfErr := range perfErrors {
				errStrings = append(errStrings, fmt.Sprintf("%v", perfErr))
			}
			err = fmt.Errorf("error(s) from perf commands: %s", strings.Join(errStrings, ", "))
			break
		}
		// no perf errors, continue
		totalRuntimeSeconds += int(endTimestamp.Sub(beginTimestamp).Seconds())
		if !refresh || (gCmdLineArgs.timeout != 0 && totalRuntimeSeconds >= gCmdLineArgs.timeout) {
			break
		}
	}
	close(frameChannel) // trigger receiveMetrics to end
	return
}

// doWorkDebug is used for testing and debugging
// Plays back events present in a file that contains perf stat output
func doWorkDebug(perfStatFilePath string, eventGroupDefinitions []GroupDefinition, metricDefinitions []MetricDefinition, metadata Metadata) (err error) {
	gCollectionStartTime = time.Now()
	// var perfCommands []*exec.Cmd
	// var processes []Process
	// if processes, perfCommands, err = getPerfCommands("perf", nil /*eventGroups*/, metadata); err != nil {
	// 	return
	// }
	// for _, cmd := range perfCommands {
	// 	log.Print(cmd)
	// }
	// log.Print(processes)
	file, err := os.Open(perfStatFilePath)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	frameCount := 0
	eventCount := 0
	frameTimestamp := 0.0
	prevEventTimestamp := 0.0
	var outputLines [][]byte
	for scanner.Scan() {
		line := scanner.Text()
		var event Event
		if event, err = parseEventJSON([]byte(line)); err != nil {
			return
		}
		if eventCount == 0 {
			prevEventTimestamp = event.Interval
		}
		if event.Interval != prevEventTimestamp {
			if len(outputLines) > 0 {
				var metricFrames []MetricFrame
				if metricFrames, frameTimestamp, err = ProcessEvents(outputLines, eventGroupDefinitions, metricDefinitions, Process{}, frameTimestamp, metadata); err != nil {
					log.Printf("%v", err)
					return
				}
				for _, metricFrame := range metricFrames {
					frameCount++
					printMetrics(metricFrame, frameCount)
					outputLines = [][]byte{} // empty it
				}
			}
		}
		outputLines = append(outputLines, []byte(line))
		prevEventTimestamp = event.Interval
		eventCount++
	}
	if len(outputLines) != 0 {
		var metricFrames []MetricFrame
		if metricFrames, _, err = ProcessEvents(outputLines, eventGroupDefinitions, metricDefinitions, Process{}, frameTimestamp, metadata); err != nil {
			log.Printf("%v", err)
			return
		}
		for _, metricFrame := range metricFrames {
			frameCount += 1
			printMetrics(metricFrame, frameCount)
		}
	}
	err = scanner.Err()
	return
}

// validateArgs is responsible for checking the sanity of the provided command
// line arguments
func validateArgs() (err error) {
	// collection options
	//  timeout needs to be zero or greater than the print interval
	if gCmdLineArgs.timeout != 0 && gCmdLineArgs.timeout*1000 < gCmdLineArgs.perfPrintInterval {
		err = fmt.Errorf("--timeout must be greater than or equal to --interval")
	}
	//  confirm a valid scope
	if gCmdLineArgs.scope == -1 {
		err = fmt.Errorf("--scope options are %s", strings.Join(ScopeOptions, ", "))
		return
	}
	//  pids only when scope is process
	if gCmdLineArgs.pidList != "" && gCmdLineArgs.scope != ScopeProcess {
		err = fmt.Errorf("--pid only valid when --scope is process")
		return
	}
	//  cids only when scope is cgroup
	if gCmdLineArgs.cidList != "" && gCmdLineArgs.scope != ScopeCgroup {
		err = fmt.Errorf("--cid only valid when --scope is cgroup")
		return
	}
	//  filter only when scope is process or cgroup
	if gCmdLineArgs.filter != "" && (gCmdLineArgs.scope != ScopeProcess && gCmdLineArgs.scope != ScopeCgroup) {
		err = fmt.Errorf("--filter only valid when --scope is process or cgroup")
		return
	}
	//  filter only when no pids/cids
	if gCmdLineArgs.filter != "" && (gCmdLineArgs.pidList != "" || gCmdLineArgs.cidList != "") {
		err = fmt.Errorf("--filter only valid when --pid and --cid are not specified")
		return
	}
	//  count must be greater than 0
	if gCmdLineArgs.count < 1 {
		err = fmt.Errorf("--count must be one or more")
		return
	}
	//  refresh must be greater than perf print intervaal
	if gCmdLineArgs.refresh*1000 < gCmdLineArgs.perfPrintInterval {
		err = fmt.Errorf("--refresh must be greater than or equal to --interval")
	}
	// output options
	//  confirm a valid granularity
	if gCmdLineArgs.granularity == -1 {
		err = fmt.Errorf("--granularity options are %s", strings.Join(GranularityOptions, ", "))
		return
	}
	//  a granularity other than system is only valid when scope is system
	if gCmdLineArgs.granularity != GranularitySystem && gCmdLineArgs.scope != ScopeSystem {
		err = fmt.Errorf("--granularity is relevant only for system scope")
	}
	//  confirm a valid output format
	if gCmdLineArgs.outputFormat == -1 {
		err = fmt.Errorf("--output options are %s", strings.Join(FormatOptions, ", "))
		return
	}
	// post-processing options
	//  confirm a valid summary format
	if gCmdLineArgs.summaryFormat == -1 {
		err = fmt.Errorf("--format options are %s", strings.Join(SummaryOptions, ", "))
		return
	}
	// advanced options
	//  minimum perf print interval
	if gCmdLineArgs.perfPrintInterval < 0 {
		err = fmt.Errorf("--interval value must be a positive integer")
		return
	}
	//  minimum mux interval
	if gCmdLineArgs.perfMuxInterval < 0 {
		err = fmt.Errorf("--muxinterval value must be a positive integer")
		return
	}
	// debugging options
	//  if metadata file path is provided, then perf stat file needs to be provided...and vice versa
	if (gCmdLineArgs.metadataFilePath != "" || gCmdLineArgs.perfStatFilePath != "") &&
		(gCmdLineArgs.metadataFilePath == "" || gCmdLineArgs.perfStatFilePath == "") {
		err = fmt.Errorf("-perfstat and -metadata options must both be specified")
		return
	}
	return
}

// flagUsage is called when a flag parsing error occurs or undefined flag is passed to the program
func flagUsage() {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "See '%s -h' for options.\n", filepath.Base(os.Args[0]))
}

// showArgumentError prints error found while validating arguments
func showArgumentError(err error) {
	out := fmt.Sprintf("Argument validation error: %v", err)
	fmt.Fprintln(os.Stderr, out)
	flagUsage()
}

// showUsage prints program usage and options to stdout
func showUsage() {
	fmt.Printf("Usage:  sudo %s [OPTIONS]\n", filepath.Base(os.Args[0]))
	fmt.Println()
	fmt.Println("Prints system metrics at 5 second intervals until interrupted by user.")
	fmt.Println("Note: Metrics are printed to stdout. Log messages are printed to stderr or, optionally, sent to syslog.")
	fmt.Println()
	args := `Options
  -h, --help
        Show this usage message and exit.
  -V, --version
        Show program version and exit.

Collection Options
  -t, --timeout <seconds>
        Number of seconds to run (default: indefinitely).
  -s, --scope <option>
        Specify the scope of collection. Options: %[1]s (default: system).
  -p, --pid <pids>
        Comma separated list of process ids. Only valid when collecting in process scope. If not provided while collecting at process scope, the currently most active processes will be monitored (default: None).
  -c, --cid <cids>
        Comma separated list of cids. Only valid when collecting at cgroup scope. If not provided while collecting at cgroup scope, the currently most active cgroups will be monitored (default: None).
  -F, --filter <regex>
        Regular expression used to match process names or cgroup IDs when --pid or --cid are not specified (default: None).
  -n, --count <count>
        The maximum number of processes or cgroups to monitor (default: 5).
  -r, --refresh <seconds>
        The number of seconds to run before refreshing the "hot" process or cgroup list (default: 30).

Output Options
  -g, --granularity <option>
        Specify the level of metric granularity. Only valid when collecting at system scope. Options: %[2]s (default: system).
  -o, --output <option>
        Specify the output format. Options: %[3]s. 'csv' is required for post-processing (default: human).
  -[v]v, --[very]verbose
        Enable verbose, or very verbose (-vv) logging (Default: False).

Post-processing Options
  -P, --post-process <CSV file>
        Path to a CSV file created during collection. Outputs a report containing summarized metric values (default: None).
  -f, --format <option>
        File format to generate when post-processing the collected CSV file. Options: %[4]s (default: csv).

Advanced Options
  -l, --list
        Show metric names available on this platform and exit (default: False).
  -S, --syslog
	Send logs to System Log daemon (default: False)
  -m, --metrics <metric names>
        A quoted and comma separated list of metric names to include in output. Use --list to view metric names. (default: all metrics).
  -e, --eventfile <path>
        Path to perf event definition file (default: None).
  -M, --metricfile <path>
        Path to metric definition file (default: None).
  -i, --interval <milliseconds>
        Event collection interval in milliseconds (default: 5000).
  -x, --muxinterval <milliseconds>
        Multiplexing interval in milliseconds (default: 125).
`
	fmt.Printf(args, strings.Join(ScopeOptions, ", "), strings.Join(GranularityOptions, ", "), strings.Join(FormatOptions, ", "), strings.Join(SummaryOptions, ", "))
	fmt.Println()
	examples := `Examples
  Metrics to screen in human readable format.
    $ sudo %[1]s
  Metrics to screen and file in CSV format.
    $ sudo %[1]s --output csv | tee %[1]s.csv
  Metrics with socket-level granularity to screen in CSV format for 60 seconds.
    $ sudo %[1]s --output csv --granularity socket --timeout 60
  Metrics for "hot" processes to screen in CSV format.
    $ sudo %[1]s --output csv --scope process
  Metrics for specified process PIDs to screen in CSV format.
    $ sudo %[1]s --output csv --scope process --pid 12345,67890
  Specified Metrics to screen in wide format.
    $ sudo %[1]s --output wide --metrics "CPU utilization %%, TMA_Frontend_Bound(%%)"
  Metrics for the "hottest" process to screen in CSV format.
    $ sudo %[1]s --output csv --scope process --count 1
Post-processing Examples
  Create summary HTML report from system metrics CSV file.
    $ %[1]s --post-process %[1]s.csv --format html >summary.html
  Create summary CSV report from any metrics CSV file to screen and file.
    $ %[1]s --post-process %[1]s.csv --format csv | tee summary.csv
`
	fmt.Printf(examples, filepath.Base(os.Args[0]))
}

// short options used:
// c, e, f, F, g, h, i, l, m, M, n, o, p, P, r, s, S, t, v, vv, V, x.

// configureArgs defines and parses the arguments accepted by the application
func configureArgs() {
	flag.Usage = func() { flagUsage() } // override default usage output
	flag.BoolVar(&gCmdLineArgs.showHelp, "h", false, "")
	flag.BoolVar(&gCmdLineArgs.showHelp, "help", false, "")
	flag.BoolVar(&gCmdLineArgs.showVersion, "V", false, "")
	flag.BoolVar(&gCmdLineArgs.showVersion, "version", false, "")
	// collection options
	flag.IntVar(&gCmdLineArgs.timeout, "t", 0, "")
	flag.IntVar(&gCmdLineArgs.timeout, "timeout", 0, "")
	var scope string
	flag.StringVar(&scope, "s", ScopeOptions[ScopeSystem], "")
	flag.StringVar(&scope, "scope", ScopeOptions[ScopeSystem], "")
	flag.StringVar(&gCmdLineArgs.pidList, "p", "", "")
	flag.StringVar(&gCmdLineArgs.pidList, "pid", "", "")
	flag.StringVar(&gCmdLineArgs.cidList, "c", "", "")
	flag.StringVar(&gCmdLineArgs.cidList, "cid", "", "")
	flag.StringVar(&gCmdLineArgs.filter, "F", "", "")
	flag.StringVar(&gCmdLineArgs.filter, "filter", "", "")
	flag.IntVar(&gCmdLineArgs.count, "n", 5, "")
	flag.IntVar(&gCmdLineArgs.count, "count", 5, "")
	flag.IntVar(&gCmdLineArgs.refresh, "r", 30, "")
	flag.IntVar(&gCmdLineArgs.refresh, "refresh", 30, "")
	// output options
	var granularity string
	flag.StringVar(&granularity, "g", GranularityOptions[GranularitySystem], "")
	flag.StringVar(&granularity, "granularity", GranularityOptions[GranularitySystem], "")
	var format string
	flag.StringVar(&format, "o", FormatOptions[FormatHuman], "")
	flag.StringVar(&format, "output", FormatOptions[FormatHuman], "")
	flag.BoolVar(&gCmdLineArgs.verbose, "v", false, "")
	flag.BoolVar(&gCmdLineArgs.verbose, "verbose", false, "")
	flag.BoolVar(&gCmdLineArgs.veryVerbose, "vv", false, "")
	flag.BoolVar(&gCmdLineArgs.veryVerbose, "veryverbose", false, "")
	// post-processing options
	flag.StringVar(&gCmdLineArgs.inputCSVFilePath, "P", "", "")
	flag.StringVar(&gCmdLineArgs.inputCSVFilePath, "post-process", "", "")
	var summary string
	flag.StringVar(&summary, "f", SummaryOptions[SummaryCSV], "")
	flag.StringVar(&summary, "format", SummaryOptions[SummaryCSV], "")
	// advanced options
	flag.BoolVar(&gCmdLineArgs.showMetricNames, "l", false, "")
	flag.BoolVar(&gCmdLineArgs.showMetricNames, "list", false, "")
	flag.BoolVar(&gCmdLineArgs.syslog, "S", false, "")
	flag.BoolVar(&gCmdLineArgs.syslog, "syslog", false, "")
	flag.StringVar(&gCmdLineArgs.metricsList, "m", "", "")
	flag.StringVar(&gCmdLineArgs.metricsList, "metrics", "", "")
	flag.StringVar(&gCmdLineArgs.eventFilePath, "e", "", "")
	flag.StringVar(&gCmdLineArgs.eventFilePath, "eventfile", "", "")
	flag.StringVar(&gCmdLineArgs.metricFilePath, "M", "", "")
	flag.StringVar(&gCmdLineArgs.metricFilePath, "metricfile", "", "")
	flag.IntVar(&gCmdLineArgs.perfPrintInterval, "i", 5000, "")
	flag.IntVar(&gCmdLineArgs.perfPrintInterval, "interval", 5000, "")
	flag.IntVar(&gCmdLineArgs.perfMuxInterval, "x", 125, "")
	flag.IntVar(&gCmdLineArgs.perfMuxInterval, "muxinterval", 125, "")
	// debugging options (not shown in help/usage)
	flag.StringVar(&gCmdLineArgs.metadataFilePath, "metadata", "", "")
	flag.StringVar(&gCmdLineArgs.perfStatFilePath, "perfstat", "", "")
	flag.Parse()
	// deal with string inputs that need to be converted to a type/enum
	// scope
	switch scope = strings.ToLower(scope); scope {
	case ScopeOptions[ScopeSystem]:
		gCmdLineArgs.scope = ScopeSystem
	case ScopeOptions[ScopeProcess]:
		gCmdLineArgs.scope = ScopeProcess
	case ScopeOptions[ScopeCgroup]:
		gCmdLineArgs.scope = ScopeCgroup
	default:
		gCmdLineArgs.scope = -1
	}
	// granularity
	switch granularity = strings.ToLower(granularity); granularity {
	case GranularityOptions[GranularitySystem]:
		gCmdLineArgs.granularity = GranularitySystem
	case GranularityOptions[GranularitySocket]:
		gCmdLineArgs.granularity = GranularitySocket
	case GranularityOptions[GranularityCPU]:
		gCmdLineArgs.granularity = GranularityCPU
	default:
		gCmdLineArgs.granularity = -1
	}
	// format
	switch format = strings.ToLower(format); format {
	case FormatOptions[FormatHuman]:
		gCmdLineArgs.outputFormat = FormatHuman
	case FormatOptions[FormatCSV]:
		gCmdLineArgs.outputFormat = FormatCSV
	case FormatOptions[FormatWide]:
		gCmdLineArgs.outputFormat = FormatWide
	default:
		gCmdLineArgs.outputFormat = -1
	}
	// summary
	switch summary = strings.ToLower(summary); summary {
	case SummaryOptions[SummaryCSV]:
		gCmdLineArgs.summaryFormat = SummaryCSV
	case SummaryOptions[SummaryHTML]:
		gCmdLineArgs.summaryFormat = SummaryHTML
	default:
		gCmdLineArgs.summaryFormat = -1
	}
}

// The program will exit with one of these exit codes
const (
	exitNoError   = 0
	exitError     = 1
	exitInterrupt = 2
)

// mainReturnWithCode is responsible for initialization and highest-level program
// logic/flow
func mainReturnWithCode() int {
	configureArgs()
	err := validateArgs()
	if err != nil {
		showArgumentError(err)
		return exitError
	}
	if gCmdLineArgs.veryVerbose {
		gCmdLineArgs.verbose = true
	}
	if gCmdLineArgs.syslog {
		// log to syslog (/var/log/syslog)
		var logwriter *syslog.Writer
		if logwriter, err = syslog.New(syslog.LOG_NOTICE, filepath.Base(os.Args[0])); err != nil {
			log.Printf("Failed to connect system log daemon: %v", err)
			return exitError
		}
		log.SetOutput(logwriter)
		log.SetFlags(0) // syslog will add date/time stamp
	} else {
		// log to stderr
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	}
	if gCmdLineArgs.showHelp {
		showUsage()
		return exitNoError
	}
	if gCmdLineArgs.showVersion {
		fmt.Println(gVersion)
		return exitNoError
	}
	if gCmdLineArgs.verbose {
		log.Printf("Starting up %s, version: %s, arguments: %s",
			filepath.Base(os.Args[0]),
			gVersion,
			strings.Join(os.Args[1:], " "),
		)
		defer log.Printf("Shutting down %s", filepath.Base(os.Args[0]))
	}
	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChannel
		if gCmdLineArgs.verbose {
			log.Printf("Received signal: %v", sig)
		}
	}()
	if gCmdLineArgs.inputCSVFilePath != "" {
		var output string
		if output, err = PostProcess(gCmdLineArgs.inputCSVFilePath, gCmdLineArgs.summaryFormat); err != nil {
			log.Printf("Error while post-processing: %v", err)
			return exitError
		}
		fmt.Print(output)
		return exitNoError
	}
	if gCmdLineArgs.timeout != 0 {
		// round up to next perfPrintInterval second (the collection interval used by perf stat)
		intervalSeconds := gCmdLineArgs.perfPrintInterval / 1000
		qf := float64(gCmdLineArgs.timeout) / float64(intervalSeconds)
		qi := gCmdLineArgs.timeout / intervalSeconds
		if qf > float64(qi) {
			gCmdLineArgs.timeout = (qi + 1) * intervalSeconds
		}
	}
	if gCmdLineArgs.outputFormat != FormatCSV {
		fmt.Print("Loading.")
	}
	var metadata Metadata
	if gCmdLineArgs.metadataFilePath != "" { // testing/debugging flow
		if metadata, err = LoadMetadataFromFile(gCmdLineArgs.metadataFilePath); err != nil {
			log.Printf("failed to load metadata from file: %v", err)
			return exitError
		}
	} else {
		if metadata, err = LoadMetadata(); err != nil {
			if os.Geteuid() != 0 {
				log.Println("Elevated permissions required, try again as root user or with sudo.")
				return exitError
			}
			log.Printf("failed to load metadata: %v", err)
			return exitError
		}
	}
	if gCmdLineArgs.verbose {
		log.Printf("%s", metadata)
	}
	if gCmdLineArgs.outputFormat != FormatCSV {
		fmt.Print(".")
	}
	evaluatorFunctions := GetEvaluatorFunctions()
	var metricDefinitions []MetricDefinition
	var selectedMetricNames []string
	if gCmdLineArgs.metricsList != "" {
		selectedMetricNames = strings.Split(gCmdLineArgs.metricsList, ",")
		for i := range selectedMetricNames {
			selectedMetricNames[i] = strings.TrimSpace(selectedMetricNames[i])
		}
	}
	if metricDefinitions, err = LoadMetricDefinitions(gCmdLineArgs.metricFilePath, selectedMetricNames, metadata); err != nil {
		log.Printf("failed to load metric definitions: %v", err)
		return exitError
	}
	if gCmdLineArgs.outputFormat != FormatCSV {
		fmt.Print(".")
	}
	if gCmdLineArgs.showMetricNames {
		fmt.Println()
		for _, metric := range metricDefinitions {
			fmt.Println(metric.Name)
		}
		return exitNoError
	}
	if err = ConfigureMetrics(metricDefinitions, evaluatorFunctions, metadata); err != nil {
		log.Printf("failed to configure metrics: %v", err)
		return exitError
	}
	var groupDefinitions []GroupDefinition
	if groupDefinitions, err = LoadEventGroups(gCmdLineArgs.eventFilePath, metadata); err != nil {
		log.Printf("failed to load event definitions: %v", err)
		return exitError
	}
	if gCmdLineArgs.outputFormat != FormatCSV {
		fmt.Print(".")
	}
	if gCmdLineArgs.perfStatFilePath != "" { // testing/debugging flow
		fmt.Print(".\n")
		if err = doWorkDebug(gCmdLineArgs.perfStatFilePath, groupDefinitions, metricDefinitions, metadata); err != nil {
			log.Printf("%v", err)
			return exitError
		}
	} else {
		if os.Geteuid() != 0 {
			log.Println("Elevated permissions required, try again as root user or with sudo.")
			return exitError
		}
		var perfPath, tempDir string
		if perfPath, tempDir, err = getPerfPath(); err != nil {
			log.Printf("failed to find perf: %v", err)
			return exitError
		}
		if tempDir != "" {
			defer os.RemoveAll(tempDir)
		}
		if gCmdLineArgs.verbose {
			log.Printf("Using perf at %s.", perfPath)
		}
		var nmiWatchdog string
		if nmiWatchdog, err = GetNMIWatchdog(); err != nil {
			log.Printf("failed to retrieve NMI watchdog status: %v", err)
			return exitError
		}
		if nmiWatchdog != "0" {
			if err = SetNMIWatchdog("0"); err != nil {
				log.Printf("failed to set NMI watchdog status: %v", err)
				return exitError
			}
			defer SetNMIWatchdog(nmiWatchdog)
		}
		if gCmdLineArgs.outputFormat != FormatCSV {
			fmt.Print(".")
		}
		var perfMuxIntervals map[string]int
		if perfMuxIntervals, err = GetMuxIntervals(); err != nil {
			log.Printf("failed to get perf mux intervals: %v", err)
			return exitError
		}
		if err = SetAllMuxIntervals(gCmdLineArgs.perfMuxInterval); err != nil {
			log.Printf("failed to set all perf mux intervals to %d: %v", gCmdLineArgs.perfMuxInterval, err)
			return exitError
		}
		defer SetMuxIntervals(perfMuxIntervals)
		if gCmdLineArgs.outputFormat != FormatCSV {
			fmt.Print(".\n")
			fmt.Printf("Reporting metrics in %d millisecond intervals...\n", gCmdLineArgs.perfPrintInterval)
		}
		if err = doWork(perfPath, groupDefinitions, metricDefinitions, metadata); err != nil {
			log.Printf("%v", err)
			return exitError
		}
	}
	return exitNoError
}

// main exits the process with code returned by called function
func main() {
	os.Exit(mainReturnWithCode())
}
