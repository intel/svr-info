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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// CmdLineArgs represents the program arguments provided by the user
type CmdLineArgs struct {
	showHelp        bool
	showVersion     bool
	showMetricNames bool
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
	inputCSVFilePath     string
	postProcessingFormat string
	// output format options
	granularity Granularity
	metricsList string
	printWide   bool
	printCSV    bool
	verbose     bool
	veryVerbose bool
	// advanced options
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
	if gCmdLineArgs.printCSV {
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
		if !gCmdLineArgs.printWide {
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

// showUsage prints program usage and options to stdout
func showUsage() {
	fmt.Printf("\nusage: sudo %s [OPTIONS]\n", filepath.Base(os.Args[0]))
	fmt.Println("\ndefault: Prints all available metrics at 5 second intervals in human readable format until interrupted by user.")
	fmt.Println("         Note: Log messages are sent to stderr. Redirect to maintain clean console output. E.g.,")
	fmt.Printf("               $ sudo %s 2>%s.log\n", filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
	fmt.Print("\noptional arguments:")
	usage := `
  -h, --help
  	Print this usage message and exit.
  -V, --version
  	Show program version and exit.
  --list
  	Show metric names available on this platform and exit.

Collection Options:
  -t, --timeout <seconds>
  	Number of seconds to run (default: indefinitely).
  -s, --scope <option>
  	Specify the scope of collection. Options: 'system', 'process', 'cgroup' (default: system).
  -p, --pid <pids>
  	Comma separated list of process ids. Only valid when collecting in process scope (default: None).
  -c, --cid <cids>
  	Comma separated list of cids. Only valid when collecting at cgroup scope (default: None).
  --filter <regex>
  	Regular expression used to match process names or cgroup IDs (default: None).
  --count <count>
  	The maximum number of processes or cgroups to monitor (default: 5).
  --refresh <seconds>
  	The number of seconds to run before refreshing the process or cgroup list, if not provided (default: 30).

Output Options:
  -g, --granularity <option>
  	Specify the level of metric granularity. Only valid when collecting at system scope. Options: 'system', 'socket', or 'cpu' (default: system).
  --metrics <metric names>
  	Metric names to include in output. (Quoted and comma separated list.)
  --csv
  	CSV formatted output. Best for parsing. Required for HTML report generation.
  --wide
  	Wide formatted output. Best used when a small number of metrics are printed.
  -v[v]
  	Enable verbose, or very verbose (-vv) logging.

Post-processing Options:
  --post-process <CSV file>
  	Path to input csv file created from --csv output during collection. When specified, a report containing average metric values will be generated.
  --format <option>
  	File format to generate when post-processing the collected CSV file. Options: 'html' or 'csv' (default: csv).

Advanced Options:
  -e, --eventfile <path>
  	Path to perf event definition file.
  -m, --metricfile <path>
  	Path to metric definition file.
  -i, --interval <milliseconds>
  	Event collection interval in milliseconds (default: 5000).
  -x, --muxinterval <milliseconds>
  	Multiplexing interval in milliseconds (default: 125).`
	fmt.Println(usage)
}

// validateArgs is responsible for checking the sanity of the provided command
// line arguments
func validateArgs() (err error) {
	if gCmdLineArgs.metadataFilePath != "" {
		if gCmdLineArgs.perfStatFilePath == "" {
			err = fmt.Errorf("-perfstat and -metadata options must both be specified")
			return
		}
	}
	if gCmdLineArgs.perfStatFilePath != "" {
		if gCmdLineArgs.metadataFilePath == "" {
			err = fmt.Errorf("-perfstat and -metadata options must both be specified")
			return
		}
	}
	if gCmdLineArgs.printCSV && gCmdLineArgs.printWide {
		err = fmt.Errorf("-csv and -wide are mutually exclusive, choose one")
		return
	}
	if gCmdLineArgs.scope == -1 {
		err = fmt.Errorf("-scope supports three options: 'system', 'process', and 'cgroup'")
		return
	}
	if gCmdLineArgs.granularity == -1 {
		err = fmt.Errorf("-granularity supports three options: 'system', 'socket', and 'cpu'")
		return
	}
	if gCmdLineArgs.granularity != GranularitySystem {
		if gCmdLineArgs.scope != ScopeSystem {
			err = fmt.Errorf("-granularity is relevant only for system scope")
		}
	}
	if gCmdLineArgs.pidList != "" {
		if gCmdLineArgs.inputCSVFilePath == "" && gCmdLineArgs.scope != ScopeProcess {
			err = fmt.Errorf("-pid only valid when -scope is process or post-processing previously collected data")
			return
		}
	}
	if gCmdLineArgs.cidList != "" {
		if gCmdLineArgs.inputCSVFilePath == "" && gCmdLineArgs.scope != ScopeCgroup {
			err = fmt.Errorf("-cid only valid when -scope is cgroup or post-processing previously collected data")
			return
		}
	}
	if gCmdLineArgs.filter != "" && (gCmdLineArgs.scope != ScopeProcess && gCmdLineArgs.scope != ScopeCgroup) {
		err = fmt.Errorf("-filter only valid when scope is process or cgroup")
		return
	}
	if gCmdLineArgs.pidList != "" && gCmdLineArgs.filter != "" {
		err = fmt.Errorf("-pid and -filter are mutually exclusive")
		return
	}
	if gCmdLineArgs.cidList != "" && gCmdLineArgs.filter != "" {
		err = fmt.Errorf("-cid and -filter are mutually exclusive")
		return
	}
	if gCmdLineArgs.postProcessingFormat != "" && gCmdLineArgs.inputCSVFilePath == "" {
		err = fmt.Errorf("--format only valid for post-processing, i.e., --post-process <csv> required")
		return
	}
	if gCmdLineArgs.postProcessingFormat != "" && strings.ToLower(gCmdLineArgs.postProcessingFormat) != "html" && strings.ToLower(gCmdLineArgs.postProcessingFormat) != "csv" {
		err = fmt.Errorf("'html' and 'csv' are valid options for post processing format")
		return
	}
	if gCmdLineArgs.pidList != "" && gCmdLineArgs.inputCSVFilePath != "" {
		pids := strings.Split(gCmdLineArgs.pidList, ",")
		if len(pids) > 1 {
			err = fmt.Errorf("can post-process only one PID at a time")
			return
		}
		if _, err = strconv.Atoi(gCmdLineArgs.pidList); err != nil {
			err = fmt.Errorf("invalid PID: %s", gCmdLineArgs.pidList)
			return
		}
	}
	if gCmdLineArgs.cidList != "" && gCmdLineArgs.inputCSVFilePath != "" {
		cids := strings.Split(gCmdLineArgs.cidList, ",")
		if len(cids) > 1 {
			err = fmt.Errorf("can post-process only one Cgroup at a time")
			return
		}
	}
	return
}

// configureArgs defines and parses the arguments accepted by the application
func configureArgs() {
	flag.Usage = func() { showUsage() } // override default usage output
	flag.BoolVar(&gCmdLineArgs.showHelp, "h", false, "")
	flag.BoolVar(&gCmdLineArgs.showHelp, "help", false, "")
	flag.BoolVar(&gCmdLineArgs.showVersion, "V", false, "")
	flag.BoolVar(&gCmdLineArgs.showVersion, "version", false, "")
	flag.BoolVar(&gCmdLineArgs.showMetricNames, "l", false, "")
	flag.BoolVar(&gCmdLineArgs.showMetricNames, "list", false, "")
	// collection options
	flag.IntVar(&gCmdLineArgs.timeout, "t", 0, "")
	flag.IntVar(&gCmdLineArgs.timeout, "timeout", 0, "")
	var scope string
	flag.StringVar(&scope, "s", "system", "")
	flag.StringVar(&scope, "scope", "system", "")
	flag.StringVar(&gCmdLineArgs.pidList, "p", "", "")
	flag.StringVar(&gCmdLineArgs.pidList, "pid", "", "")
	flag.StringVar(&gCmdLineArgs.cidList, "c", "", "")
	flag.StringVar(&gCmdLineArgs.cidList, "cid", "", "")
	flag.StringVar(&gCmdLineArgs.filter, "filter", "", "")
	flag.IntVar(&gCmdLineArgs.count, "count", 5, "")
	flag.IntVar(&gCmdLineArgs.refresh, "refresh", 30, "")
	// output options
	var granularity string
	flag.StringVar(&granularity, "granularity", "system", "")
	flag.StringVar(&gCmdLineArgs.metricsList, "metrics", "", "")
	flag.BoolVar(&gCmdLineArgs.printCSV, "csv", false, "")
	flag.BoolVar(&gCmdLineArgs.printWide, "wide", false, "")
	flag.BoolVar(&gCmdLineArgs.verbose, "v", false, "")
	flag.BoolVar(&gCmdLineArgs.veryVerbose, "vv", false, "")
	// post-processing options
	flag.StringVar(&gCmdLineArgs.inputCSVFilePath, "post-process", "", "")
	flag.StringVar(&gCmdLineArgs.postProcessingFormat, "format", "", "")
	// advanced options
	flag.IntVar(&gCmdLineArgs.perfPrintInterval, "i", 5000, "")
	flag.IntVar(&gCmdLineArgs.perfPrintInterval, "interval", 5000, "")
	flag.IntVar(&gCmdLineArgs.perfMuxInterval, "x", 125, "")
	flag.IntVar(&gCmdLineArgs.perfMuxInterval, "muxinterval", 125, "")
	flag.StringVar(&gCmdLineArgs.eventFilePath, "e", "", "")
	flag.StringVar(&gCmdLineArgs.eventFilePath, "eventfile", "", "")
	flag.StringVar(&gCmdLineArgs.metricFilePath, "m", "", "")
	flag.StringVar(&gCmdLineArgs.metricFilePath, "metricfile", "", "")
	// debugging options
	flag.StringVar(&gCmdLineArgs.metadataFilePath, "metadata", "", "")
	flag.StringVar(&gCmdLineArgs.perfStatFilePath, "perfstat", "", "")
	flag.Parse()
	if strings.ToLower(scope) == ScopeOptions[ScopeSystem] {
		gCmdLineArgs.scope = ScopeSystem
	} else if strings.ToLower(scope) == ScopeOptions[ScopeProcess] {
		gCmdLineArgs.scope = ScopeProcess
	} else if strings.ToLower(scope) == ScopeOptions[ScopeCgroup] {
		gCmdLineArgs.scope = ScopeCgroup
	} else {
		gCmdLineArgs.scope = -1
	}
	if strings.ToLower(granularity) == GranularityOptions[GranularitySystem] {
		gCmdLineArgs.granularity = GranularitySystem
	} else if strings.ToLower(granularity) == GranularityOptions[GranularitySocket] {
		gCmdLineArgs.granularity = GranularitySocket
	} else if strings.ToLower(granularity) == GranularityOptions[GranularityCPU] {
		gCmdLineArgs.granularity = GranularityCPU
	} else {
		gCmdLineArgs.granularity = -1
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
		log.Printf("Invalid argument error: %v", err)
		showUsage()
		return exitError
	}
	if gCmdLineArgs.veryVerbose {
		gCmdLineArgs.verbose = true
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
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
	}
	if gCmdLineArgs.inputCSVFilePath != "" {
		var output string
		format := gCmdLineArgs.postProcessingFormat
		if format == "" {
			format = "csv" // default format is csv
		}
		if output, err = PostProcess(gCmdLineArgs.inputCSVFilePath, strings.ToLower(format)); err != nil {
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
	if !gCmdLineArgs.printCSV {
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
				fmt.Println("\nElevated permissions required, try again as root user or with sudo.")
				return exitError
			}
			log.Printf("failed to load metadata: %v", err)
			return exitError
		}
	}
	if gCmdLineArgs.verbose {
		log.Printf("%s", metadata)
	}
	if !gCmdLineArgs.printCSV {
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
	if !gCmdLineArgs.printCSV {
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
	if !gCmdLineArgs.printCSV {
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
			fmt.Println("\nElevated permissions required, try again as root user or with sudo.")
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
		if !gCmdLineArgs.printCSV {
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
		if !gCmdLineArgs.printCSV {
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
