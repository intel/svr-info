package main

import (
	"bufio"
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
)

type CmdLineArgs struct {
	showHelp          bool
	showVersion       bool
	duration          int // seconds
	eventFilePath     string
	metricFilePath    string
	perfPrintInterval int // milliseconds
	perfMuxInterval   int // milliseconds
	printCSV          bool
	verbose           bool
	veryVerbose       bool
	metadataFilePath  string
	perfStatFilePath  string
	printMetricNames  bool
	metricsList       string
}

// globals
var (
	gVersion     string = "dev"
	gCmdLineArgs CmdLineArgs
)

//go:embed resources
var resources embed.FS

// build perf args from event groups
func getPerfCommandArgs(eventGroups []GroupDefinition, metadata Metadata) (args []string, err error) {
	uncollectableEvents := mapset.NewSet[string]()
	intervalMS := fmt.Sprintf("%d", gCmdLineArgs.perfPrintInterval)
	args = append(args, []string{"stat", "-I", intervalMS, "-x", ",", "-a", "-e"}...)
	var groups []string
	for _, group := range eventGroups {
		var events []string
		for _, event := range group {
			var collectable bool
			if collectable, err = isCollectableEvent(event, metadata); err != nil {
				return
			}
			if !collectable {
				uncollectableEvents.Add(event.Name)
				continue
			}
			events = append(events, event.Raw)
		}
		if len(events) == 0 {
			if gCmdLineArgs.veryVerbose {
				log.Printf("No collectable events in group: %v", group)
			}
		} else {
			groups = append(groups, fmt.Sprintf("{%s}", strings.Join(events, ",")))
		}
	}
	if uncollectableEvents.Cardinality() != 0 && gCmdLineArgs.verbose {
		log.Printf("Uncollectable events: %s", uncollectableEvents)
	}
	// "fixed" PMU counters are not supported on (most) IaaS VMs, so we add a separate group
	if !isUncoreSupported(metadata) {
		newGroup := []string{"cpu-cycles", "instructions"}
		if metadata.RefCyclesSupported {
			newGroup = append(newGroup, "ref-cycles")
		}
		groups = append(groups, fmt.Sprintf("{%s}", strings.Join(newGroup, ",")))
		newGroup = []string{"cpu-cycles:k", "instructions"}
		if metadata.RefCyclesSupported {
			newGroup = append(newGroup, "ref-cycles:k")
		}
		groups = append(groups, fmt.Sprintf("{%s}", strings.Join(newGroup, ",")))

	}
	groupsArg := fmt.Sprintf("'%s'", strings.Join(groups, ","))
	args = append(args, groupsArg)
	if gCmdLineArgs.duration > 0 {
		args = append(args, "sleep")
		args = append(args, fmt.Sprintf("%d", gCmdLineArgs.duration))
	}
	return
}

// Starts perf, reads from perf's output (stderr), sends a list of events over the
// provided channel when the timestamp on the events changes. Note that waiting for the
// timestamp to change means that the first list won't get sent until the next set
// of events comes from perf, i.e., program output will be one collection duration
// behind the real-time perf processing
func runPerf(eventGroups []GroupDefinition, eventChannel chan []string, metadata Metadata, perfError context.CancelFunc) (err error) {
	var cmd *exec.Cmd
	var reader io.ReadCloser
	var args []string
	if args, err = getPerfCommandArgs(eventGroups, metadata); err != nil {
		return
	}
	cmd = exec.Command("perf", args...)
	reader, _ = cmd.StderrPipe()
	if gCmdLineArgs.veryVerbose {
		log.Print(cmd)
	}
	scanner := bufio.NewScanner(reader)
	var outputLines []string
	// start perf stat
	if err = cmd.Start(); err != nil {
		log.Printf("failed to run perf: %v", err)
		perfError()                // this informs caller that there was an error
		eventChannel <- []string{} // need to send an empy event list because caller is blocking on this channel
		return
	}
	// Use a timer to determine when to send a frame of events back to the caller (over the eventChannel).
	// The timer will expire when no lines (events) have been received from perf for more than 100ms. This
	// works because perf writes the events to stderr in a burst every collection interval, e.g., 5 seconds.
	// When the timer expires, this code assumes that perf is done writing events to stderr.
	// The first duration needs to be longer than the time it takes for perf to print its first line of output.
	t1 := time.NewTimer(time.Duration(2 * gCmdLineArgs.perfPrintInterval))
	go func() {
		for {
			<-t1.C                      // waits for timer to expire
			eventChannel <- outputLines // send it
			outputLines = []string{}    // empty it
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
		outputLines = append(outputLines, line)
	}
	t1.Stop()
	eventChannel <- outputLines // send the last set of lines
	// signal to caller that we're done by closing the event channel
	close(eventChannel)
	// wait for perf stat to exit
	if err = cmd.Wait(); err != nil {
		log.Printf("error from perf stat on exit: %v", err)
		return
	}
	return
}

// Function used for testing and debugging
// Plays back events present in a file that contains perf stat output
func playbackPerf(perfStatFilePath string, eventChannel chan []string, metadata Metadata, perfError context.CancelFunc) (err error) {
	file, err := os.Open(perfStatFilePath)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	frameTimestamp := ""
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lineTimestamp := strings.Split(line, ",")[0]
		if lineTimestamp != frameTimestamp {
			// send lines
			eventChannel <- lines
			frameTimestamp = lineTimestamp
			lines = []string{}
		}
		lines = append(lines, line)
	}
	close(eventChannel)
	err = scanner.Err()
	return
}

func showUsage() {
	fmt.Printf("%s Version: %s\n", filepath.Base(os.Args[0]), gVersion)
	fmt.Println("Options:")
	usage := `  -csv
  	CSV formatted output.
  -h
  	Print this usage message.
  -t <seconds>
  	Number of seconds to run. By default, runs indefinitely.
  -v[v]
  	Enable verbose logging.
  -V
  	Print program version.
  -o
  	Print metric names availabe on this platform.
  -metrics <metric names>
  	Quoted and comma separated list of metric names to include in output.

Advanced Options:
  -e <path>
  	Path to perf event definition file.
  -m <path>
  	Path to metric definition file.
  -i <milliseconds>
  	Event collection interval in milliseconds
  -x <milliseconds>
  	Multiplexing interval in milliseconds

Debug Options:
  -p <path>
  	Path to perf stat data file.
  -d <path>
  	Path to metadata file.`
	fmt.Println(usage)
}

func validateArgs() (err error) {
	if gCmdLineArgs.metadataFilePath != "" {
		if gCmdLineArgs.perfStatFilePath == "" {
			err = fmt.Errorf("-p and -d options must both be specified")
		}
	}
	if gCmdLineArgs.perfStatFilePath != "" {
		if gCmdLineArgs.metadataFilePath == "" {
			err = fmt.Errorf("-p and -d options must both be specified")
		}
	}
	return
}

const (
	exitNoError   = 0
	exitError     = 1
	exitInterrupt = 2
)

func mainReturnWithCode(ctx context.Context) int {
	flag.Usage = func() { showUsage() } // override default usage output
	flag.BoolVar(&gCmdLineArgs.showHelp, "h", false, "Print this usage message.")
	flag.BoolVar(&gCmdLineArgs.showVersion, "V", false, "Print program version.")
	flag.IntVar(&gCmdLineArgs.duration, "t", 0, "Number of seconds to run. By default, runs indefinitely.")
	flag.IntVar(&gCmdLineArgs.perfPrintInterval, "i", 5000, "Event collection interval in milliseconds.")
	flag.IntVar(&gCmdLineArgs.perfMuxInterval, "x", 125, "Multiplexing interval in milliseconds.")
	flag.StringVar(&gCmdLineArgs.eventFilePath, "e", "", "Path to event definition file.")
	flag.StringVar(&gCmdLineArgs.metricFilePath, "m", "", "Path to metric definition file.")
	flag.StringVar(&gCmdLineArgs.metadataFilePath, "d", "", "Path to metadata file.")
	flag.StringVar(&gCmdLineArgs.perfStatFilePath, "p", "", "Path to perf stat data file.")
	flag.BoolVar(&gCmdLineArgs.verbose, "v", false, "Enable verbose logging.")
	flag.BoolVar(&gCmdLineArgs.veryVerbose, "vv", false, "Enable verbose logging.")
	flag.BoolVar(&gCmdLineArgs.printCSV, "csv", false, "Print output to stdout in CSV format.")
	flag.BoolVar(&gCmdLineArgs.printMetricNames, "o", false, "")
	flag.StringVar(&gCmdLineArgs.metricsList, "metrics", "", "")
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
	if gCmdLineArgs.duration != 0 {
		// round up to next perfPrintInterval second (the collection interval used by perf stat)
		intervalSeconds := gCmdLineArgs.perfPrintInterval / 1000
		qf := float64(gCmdLineArgs.duration) / float64(intervalSeconds)
		qi := gCmdLineArgs.duration / intervalSeconds
		if qf > float64(qi) {
			gCmdLineArgs.duration = (qi + 1) * intervalSeconds
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
	if gCmdLineArgs.printMetricNames {
		fmt.Println()
		for _, metric := range metricDefinitions {
			fmt.Println(metric.Name[7:])
		}
		return exitNoError
	}
	if err = configureMetrics(metricDefinitions, evaluatorFunctions, metadata); err != nil {
		log.Printf("failed to configure metrics: %v", err)
		return exitError
	}
	eventChannel := make(chan []string)
	perfCtx, perfError := context.WithCancel(context.Background())
	defer perfError()
	if gCmdLineArgs.perfStatFilePath != "" { // testing/debugging flow
		go playbackPerf(gCmdLineArgs.perfStatFilePath, eventChannel, metadata, perfError)
	} else {
		if os.Geteuid() != 0 {
			fmt.Println("\nElevated permissions required, try again as root user or with sudo.")
			return exitError
		}
		var groupDefinitions []GroupDefinition
		if groupDefinitions, err = loadEventDefinitions(gCmdLineArgs.eventFilePath, metadata); err != nil {
			log.Printf("failed to load event definitions: %v", err)
			return exitError
		}
		if gCmdLineArgs.verbose {
			log.Printf("%s", metadata)
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
		// run perf in a goroutine, get events through eventChannel
		go runPerf(groupDefinitions, eventChannel, metadata, perfError)
	}
	// loop until the perf goroutine closes the eventChannel or user hits ctrl-c
	var frameTimestamp float64
	var metrics []Metric
	more := true
	firstFrame := true
	for more {
		select {
		case <-perfCtx.Done(): // error from perf
			return exitError
		case <-ctx.Done(): // ctrl-c
			return exitNoError
		default:
			var perfEvents []string
			perfEvents, more = <-eventChannel // events from one frame of collection (all same timestamp)
			if more && len(perfEvents) > 0 {
				if metrics, frameTimestamp, err = processEvents(perfEvents, metricDefinitions, frameTimestamp, metadata); err != nil {
					log.Printf("%v", err)
					return exitError
				}
				if gCmdLineArgs.printCSV {
					if firstFrame {
						firstFrame = false
						// print "timestamp,", then metric names as CSV headers
						fmt.Print("timestamp,")
						var names []string
						for _, metric := range metrics {
							names = append(names, metric.Name[7:])
						}
						fmt.Printf("%s\n", strings.Join(names, ","))
					}
					fmt.Printf("%0.4f,", frameTimestamp)
					var values []string
					for _, metric := range metrics {
						values = append(values, strconv.FormatFloat(metric.Value, 'g', 8, 64))
					}
					fmt.Printf("%s\n", strings.Join(values, ","))
				} else { // human readable output
					fmt.Println("--------------------------------------------------------------------------------------")
					fmt.Printf("- Metrics captured at t + %0.2fs\n", frameTimestamp)
					fmt.Println("--------------------------------------------------------------------------------------")
					fmt.Printf("%-70s %15s\n", "metric", "value")
					fmt.Printf("%-70s %15s\n", "------------------------", "----------")
					for _, metric := range metrics {
						fmt.Printf("%-70s %15s\n", metric.Name[7:], strconv.FormatFloat(metric.Value, 'g', 4, 64))
					}
				}
			}
		}
	}
	return exitNoError
}

func main() {
	// watch for user interrupt (ctrl-c)
	// adapted from here: https://pace.dev/blog/2020/02/17/repond-to-ctrl-c-interrupt-signals-gracefully-with-context-in-golang-by-mat-ryer.html
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	defer func() {
		signal.Stop(signalChan)
		cancel()
	}()
	go func() {
		select {
		case <-signalChan: // first signal, cancel context
			cancel()
		case <-ctx.Done():
		}
		<-signalChan // second signal, hard exit
		os.Exit(exitInterrupt)
	}()
	os.Exit(mainReturnWithCode(ctx))
}
