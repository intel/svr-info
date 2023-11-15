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

type CmdLineArgs struct {
	showHelp          bool
	showVersion       bool
	timeout           int // seconds
	eventFilePath     string
	metricFilePath    string
	perfPrintInterval int // milliseconds
	perfMuxInterval   int // milliseconds
	printCSV          bool
	verbose           bool
	veryVerbose       bool
	metadataFilePath  string
	perfStatFilePath  string
	showMetricNames   bool
	metricsList       string
	printWide         bool
}

// globals
var (
	gVersion             string = "dev"
	gCmdLineArgs         CmdLineArgs
	gCollectionStartTime time.Time
)

//go:embed resources
var resources embed.FS

// extract executables from embedded resources to temporary directory
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

func existsExecutableResource(filename string) (exists bool) {
	f, err := resources.Open(filepath.Join("resources", filename))
	if err != nil {
		exists = false
		return
	}
	f.Close()
	exists = true
	return
}

func getPerfPath() (path string, tempDir string, err error) {
	if existsExecutableResource("perf") {
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

// build perf args from event groups
func getPerfCommandArgs(eventGroups []GroupDefinition, metadata Metadata) (args []string, err error) {
	args = append(args, []string{"stat", "-I", fmt.Sprintf("%d", gCmdLineArgs.perfPrintInterval), "-a", "-j", "-e"}...)
	var groups []string
	for _, group := range eventGroups {
		var events []string
		for _, event := range group {
			events = append(events, event.Raw)
		}
		groups = append(groups, fmt.Sprintf("{%s}", strings.Join(events, ",")))
	}
	args = append(args, fmt.Sprintf("'%s'", strings.Join(groups, ",")))
	if gCmdLineArgs.timeout > 0 {
		args = append(args, "sleep")
		args = append(args, fmt.Sprintf("%d", gCmdLineArgs.timeout))
	}
	return
}

func doWork(perfPath string, eventGroups []GroupDefinition, metricDefinitions []MetricDefinition, metadata Metadata) (err error) {
	var args []string
	if args, err = getPerfCommandArgs(eventGroups, metadata); err != nil {
		log.Printf("failed to assemble perf args: %v", err)
		return
	}
	cmd := exec.Command(perfPath, args...)
	reader, _ := cmd.StderrPipe()
	if gCmdLineArgs.veryVerbose {
		log.Print(cmd)
	}
	scanner := bufio.NewScanner(reader)
	var outputLines [][]byte
	// start perf stat
	if err = cmd.Start(); err != nil {
		log.Printf("failed to run perf: %v", err)
		return
	}
	// get current time for use in setting timestamps on output
	gCollectionStartTime = time.Now()
	// Use a timer to determine when to send a frame of events back to the caller (over the eventChannel).
	// The timer will expire when no lines (events) have been received from perf for more than 100ms. This
	// works because perf writes the events to stderr in a burst every collection interval, e.g., 5 seconds.
	// When the timer expires, this code assumes that perf is done writing events to stderr.
	// The first duration needs to be longer than the time it takes for perf to print its first line of output.
	t1 := time.NewTimer(time.Duration(2 * gCmdLineArgs.perfPrintInterval))
	var frameTimestamp float64
	var metrics []Metric
	frameCount := 0
	go func() {
		for {
			<-t1.C // waits for timer to expire
			if len(outputLines) != 0 {
				if metrics, frameTimestamp, err = processEvents(outputLines, metricDefinitions, frameTimestamp, metadata); err != nil {
					log.Printf("%v", err)
					return
				}
				frameCount += 1
				printMetrics(metrics, frameCount, frameTimestamp)
				outputLines = [][]byte{} // empty it
			}
		}
	}()
	// read perf stat output
	for scanner.Scan() { // blocks waiting for next token (line)
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
		if metrics, frameTimestamp, err = processEvents(outputLines, metricDefinitions, frameTimestamp, metadata); err != nil {
			log.Printf("%v", err)
			return
		}
		frameCount += 1
		printMetrics(metrics, frameCount, frameTimestamp)
	}
	// wait for perf stat to exit
	if err = cmd.Wait(); err != nil {
		if strings.Contains(err.Error(), "signal") {
			err = nil
		} else {
			log.Printf("error from perf stat on exit: %v", err)
		}
		return
	}
	return
}

// Function used for testing and debugging
// Plays back events present in a file that contains perf stat output
func doWorkDebug(perfStatFilePath string, metricDefinitions []MetricDefinition, metadata Metadata) (err error) {
	file, err := os.Open(perfStatFilePath)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var metrics []Metric
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
			prevEventTimestamp = event.Timestamp
		}
		if event.Timestamp != prevEventTimestamp {
			if len(outputLines) > 0 {
				if metrics, frameTimestamp, err = processEvents(outputLines, metricDefinitions, frameTimestamp, metadata); err != nil {
					log.Printf("%v", err)
					return
				}
				frameCount++
				printMetrics(metrics, frameCount, frameTimestamp)
				outputLines = [][]byte{} // empty it
			}
		}
		outputLines = append(outputLines, []byte(line))
		prevEventTimestamp = event.Timestamp
		eventCount++
	}
	if len(outputLines) != 0 {
		if metrics, frameTimestamp, err = processEvents(outputLines, metricDefinitions, frameTimestamp, metadata); err != nil {
			log.Printf("%v", err)
			return
		}
		frameCount += 1
		printMetrics(metrics, frameCount, frameTimestamp)
	}
	err = scanner.Err()
	return
}

func showUsage() {
	fmt.Printf("\nusage: sudo %s [OPTIONS]\n", filepath.Base(os.Args[0]))
	fmt.Println("\ndefault: Prints all available metrics at 5 second intervals in human readable format until interrupted by user.")
	fmt.Println("         Note: Log messages are sent to stderr. Redirect to maintain clean console output. E.g.,")
	fmt.Printf("               $ sudo %s 2>pmu2metrics.log\n", filepath.Base(os.Args[0]))
	fmt.Print("\noptional arguments:")
	usage := `
  -h, --help
  	Print this usage message and exit.
  --csv
  	CSV formatted output.
  --list
  	Show metric names available on this platform and exit.
  --metrics <metric names>
  	Metric names to include in output. (Quoted and comma separated list.)
  --wide
  	Wide formatted output. Best when a few selected metrics are printed.
  -t, --timeout <seconds>
  	Number of seconds to run. By default, runs indefinitely.
  -v[v]
  	Enable verbose logging.
  -V, --version
  	Show program version and exit.

Advanced Options:
  -e, --eventfile <path>
  	Path to perf event definition file.
  -m, --metricfile <path>
  	Path to metric definition file.
  -i, --interval <milliseconds>
  	Event collection interval in milliseconds
  -x, --muxinterval <milliseconds>
  	Multiplexing interval in milliseconds`
	fmt.Println(usage)
}

func validateArgs() (err error) {
	if gCmdLineArgs.metadataFilePath != "" {
		if gCmdLineArgs.perfStatFilePath == "" {
			err = fmt.Errorf("-p and -d options must both be specified")
			return
		}
	}
	if gCmdLineArgs.perfStatFilePath != "" {
		if gCmdLineArgs.metadataFilePath == "" {
			err = fmt.Errorf("-p and -d options must both be specified")
			return
		}
	}
	if gCmdLineArgs.printCSV && gCmdLineArgs.printWide {
		err = fmt.Errorf("-csv and -wide are mutually exclusive, choose one")
		return
	}
	return
}

func printMetrics(metrics []Metric, frameCount int, frameTimestamp float64) {
	if gCmdLineArgs.printCSV {
		if frameCount == 1 {
			// print "Timestamp,", then metric names as CSV headers
			fmt.Print("Timestamp,")
			var names []string
			for _, metric := range metrics {
				names = append(names, metric.Name)
			}
			fmt.Printf("%s\n", strings.Join(names, ","))
		}
		fmt.Printf("%d,", gCollectionStartTime.Unix()+int64(frameTimestamp))
		var values []string
		for _, metric := range metrics {
			values = append(values, strconv.FormatFloat(metric.Value, 'g', 8, 64))
		}
		fmt.Printf("%s\n", strings.Join(values, ","))
	} else { // human readable output
		if !gCmdLineArgs.printWide {
			fmt.Println("--------------------------------------------------------------------------------------")
			fmt.Printf("- Metrics captured at %s\n", gCollectionStartTime.Add(time.Second*time.Duration(int(frameTimestamp))).UTC())
			fmt.Println("--------------------------------------------------------------------------------------")
			fmt.Printf("%-70s %15s\n", "metric", "value")
			fmt.Printf("%-70s %15s\n", "------------------------", "----------")
			for _, metric := range metrics {
				fmt.Printf("%-70s %15s\n", metric.Name, strconv.FormatFloat(metric.Value, 'g', 4, 64))
			}
		} else { // wide format
			var names []string
			var values []float64
			for _, metric := range metrics {
				names = append(names, metric.Name)
				values = append(values, metric.Value)
			}
			if frameCount == 1 { // print headers
				header := "Timestamp    "
				header += strings.Join(names, "   ")
				fmt.Printf("%s\n", header)
			}
			// handle timestamp
			colWidth := 10
			colSpacing := 3
			val := fmt.Sprintf("%d", gCollectionStartTime.Unix()+int64(frameTimestamp))
			row := fmt.Sprintf("%s%*s%*s", val, colWidth-len(val), "", colSpacing, "")
			// handle the metric values
			for i, value := range values {
				colWidth = len(names[i])
				val = fmt.Sprintf("%.2f", value)
				row += fmt.Sprintf("%s%*s%*s", val, colWidth-len(val), "", colSpacing, "")
			}
			fmt.Println(row)
		}
	}
}

const (
	exitNoError   = 0
	exitError     = 1
	exitInterrupt = 2
)

func mainReturnWithCode() int {
	flag.Usage = func() { showUsage() } // override default usage output
	flag.BoolVar(&gCmdLineArgs.showHelp, "h", false, "")
	flag.BoolVar(&gCmdLineArgs.showHelp, "help", false, "")
	flag.BoolVar(&gCmdLineArgs.showVersion, "V", false, "")
	flag.BoolVar(&gCmdLineArgs.showVersion, "version", false, "")
	flag.BoolVar(&gCmdLineArgs.showMetricNames, "l", false, "")
	flag.BoolVar(&gCmdLineArgs.showMetricNames, "list", false, "")
	flag.IntVar(&gCmdLineArgs.timeout, "t", 0, "")
	flag.IntVar(&gCmdLineArgs.timeout, "timeout", 0, "")
	flag.IntVar(&gCmdLineArgs.perfPrintInterval, "i", 5000, "")
	flag.IntVar(&gCmdLineArgs.perfPrintInterval, "interval", 5000, "")
	flag.IntVar(&gCmdLineArgs.perfMuxInterval, "x", 125, "")
	flag.IntVar(&gCmdLineArgs.perfMuxInterval, "muxinterval", 125, "")
	flag.StringVar(&gCmdLineArgs.eventFilePath, "e", "", "")
	flag.StringVar(&gCmdLineArgs.eventFilePath, "eventfile", "", "")
	flag.StringVar(&gCmdLineArgs.metricFilePath, "m", "", "")
	flag.StringVar(&gCmdLineArgs.metricFilePath, "metricfile", "", "")
	flag.BoolVar(&gCmdLineArgs.printCSV, "csv", false, "")
	flag.BoolVar(&gCmdLineArgs.printWide, "wide", false, "")
	flag.StringVar(&gCmdLineArgs.metricsList, "metrics", "", "")
	flag.BoolVar(&gCmdLineArgs.verbose, "v", false, "")
	flag.BoolVar(&gCmdLineArgs.veryVerbose, "vv", false, "")
	// debugging options
	flag.StringVar(&gCmdLineArgs.metadataFilePath, "d", "", "")
	flag.StringVar(&gCmdLineArgs.perfStatFilePath, "p", "", "")
	flag.Parse()
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
		if metadata, err = loadMetadataFromFile(gCmdLineArgs.metadataFilePath); err != nil {
			log.Printf("failed to load metadata from file: %v", err)
			return exitError
		}
	} else {
		if metadata, err = loadMetadata(); err != nil {
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
	evaluatorFunctions := getEvaluatorFunctions()
	var metricDefinitions []MetricDefinition
	var selectedMetricNames []string
	if gCmdLineArgs.metricsList != "" {
		selectedMetricNames = strings.Split(gCmdLineArgs.metricsList, ",")
		for i := range selectedMetricNames {
			selectedMetricNames[i] = strings.TrimSpace(selectedMetricNames[i])
		}
	}
	if metricDefinitions, err = loadMetricDefinitions(gCmdLineArgs.metricFilePath, selectedMetricNames, metadata); err != nil {
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
	if err = configureMetrics(metricDefinitions, evaluatorFunctions, metadata); err != nil {
		log.Printf("failed to configure metrics: %v", err)
		return exitError
	}
	if gCmdLineArgs.perfStatFilePath != "" { // testing/debugging flow
		fmt.Print(".\n")
		gCollectionStartTime = time.Now()
		if err = doWorkDebug(gCmdLineArgs.perfStatFilePath, metricDefinitions, metadata); err != nil {
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
		var groupDefinitions []GroupDefinition
		if groupDefinitions, err = loadEventDefinitions(gCmdLineArgs.eventFilePath, metadata); err != nil {
			log.Printf("failed to load event definitions: %v", err)
			return exitError
		}
		if !gCmdLineArgs.printCSV {
			fmt.Print(".")
		}
		var nmiWatchdog string
		if nmiWatchdog, err = getNmiWatchdog(); err != nil {
			log.Printf("failed to retrieve NMI watchdog status: %v", err)
			return exitError
		}
		if nmiWatchdog != "0" {
			if err = setNmiWatchdog("0"); err != nil {
				log.Printf("failed to set NMI watchdog status: %v", err)
				return exitError
			}
			defer setNmiWatchdog(nmiWatchdog)
		}
		if !gCmdLineArgs.printCSV {
			fmt.Print(".")
		}
		var perfMuxIntervals map[string]string
		if perfMuxIntervals, err = getMuxIntervals(); err != nil {
			log.Printf("failed to get perf mux intervals: %v", err)
			return exitError
		}
		if err = setAllMuxIntervals(gCmdLineArgs.perfMuxInterval); err != nil {
			log.Printf("failed to set all perf mux intervals to %d: %v", gCmdLineArgs.perfMuxInterval, err)
			return exitError
		}
		defer setMuxIntervals(perfMuxIntervals)
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

func main() {
	os.Exit(mainReturnWithCode())
}
