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

// globals
var (
	gVersion string = "dev"
	gVerbose bool   = false
	gDebug   bool   = false
)

const metricInterval int = 5 // the number of seconds between metric reports
const muxInterval int = 125  // the number of milliseconds for perf multiplexing interval

//go:embed resources
var resources embed.FS

// build perf args from event groups
func getPerfCommandArgs(eventGroups []GroupDefinition, runSeconds int, metadata Metadata) (args []string, err error) {
	uncollectableEvents := mapset.NewSet[string]()
	intervalMS := fmt.Sprintf("%d", metricInterval*1000) // milliseconds
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
			if gVerbose {
				log.Printf("No collectable events in group.")
			}
		} else {
			groups = append(groups, fmt.Sprintf("{%s}", strings.Join(events, ",")))
		}
	}
	if uncollectableEvents.Cardinality() != 0 && gVerbose {
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
	if runSeconds > 0 {
		args = append(args, "sleep")
		args = append(args, fmt.Sprintf("%d", runSeconds))
	}
	return
}

// Starts perf, reads from perf's output (stderr), sends a list of events over the
// provided channel when the timestamp on the events changes. Note that waiting for the
// timestamp to change means that the first list won't get sent until the next set
// of events comes from perf, i.e., program output will be one collection duration
// behind the real-time perf processing
func runPerf(eventGroups []GroupDefinition, eventChannel chan []string, runSeconds int, metadata Metadata, perfError context.CancelFunc) (err error) {
	var cmd *exec.Cmd
	var reader io.ReadCloser
	var args []string
	if args, err = getPerfCommandArgs(eventGroups, runSeconds, metadata); err != nil {
		return
	}
	// TODO: remove this debug option that's used for development/debugging
	if gDebug {
		cmd = exec.Command("scripts/perfstat_sim")
		reader, _ = cmd.StdoutPipe()
	} else {
		cmd = exec.Command("perf", args...)
		reader, _ = cmd.StderrPipe()
	}
	if gVerbose {
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
	t1 := time.NewTimer(time.Duration(2*metricInterval) * time.Second)
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
		if gVerbose {
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

func showUsage() {
	fmt.Printf("%s Version: %s\n", filepath.Base(os.Args[0]), gVersion)
	fmt.Println("Options:")
	usage := `  -csv
  	CSV formatted output.
  -e <path>
  	Path to perf event definition file.
  -h
  	Print this usage message.
  -m <path>
  	Path to metric definition file.
  -t <seconds>
  	Number of seconds to run. By default, runs indefinitely.
  -v
  	Enable verbose logging.
  -V
  	Print program version.`
	fmt.Println(usage)
}

const (
	exitNoError   = 0
	exitError     = 1
	exitInterrupt = 2
)

func mainReturnWithCode(ctx context.Context) int {
	var showHelp bool
	var showVersion bool
	var runSeconds int
	var eventFilePath string
	var metricFilePath string
	var printCSV bool

	flag.Usage = func() { showUsage() } // override default usage output
	flag.BoolVar(&showHelp, "h", false, "Print this usage message.")
	flag.BoolVar(&showVersion, "V", false, "Print program version.")
	flag.IntVar(&runSeconds, "t", 0, "Number of seconds to run. By default, runs indefinitely.")
	flag.StringVar(&eventFilePath, "e", "", "Path to custom perf event definition file.")
	flag.StringVar(&metricFilePath, "m", "", "Path to custom metric definition file.")
	flag.BoolVar(&gVerbose, "v", false, "Enable verbose logging.")
	flag.BoolVar(&printCSV, "csv", false, "Print output to stdout in CSV format.")
	flag.BoolVar(&gDebug, "devonly", false, "Temporary debug option used during development. Remove me.")
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if showHelp {
		showUsage()
		return exitNoError
	}
	if showVersion {
		fmt.Println(gVersion)
		return exitNoError
	}
	if !gDebug {
		if os.Geteuid() != 0 {
			fmt.Println("Elevated permissions required, try again as root user or with sudo.")
			return exitError
		}
	}
	if gVerbose {
		log.Printf("Starting up %s, version: %s, arguments: %s",
			filepath.Base(os.Args[0]),
			gVersion,
			strings.Join(os.Args[1:], " "),
		)
	}
	if runSeconds != 0 {
		// round up to next -metricInterval- second interval (the collection frequency used for perf)
		qf := float64(runSeconds) / float64(metricInterval)
		qi := runSeconds / metricInterval
		if qf > float64(qi) {
			runSeconds = (qi + 1) * metricInterval
		}
	}
	if !printCSV {
		fmt.Print("Loading.")
	}
	var err error
	var metadata Metadata
	if metadata, err = loadMetadata(); err != nil {
		log.Printf("failed to load metadata: %v", err)
		return exitError
	}
	if !printCSV {
		fmt.Print(".")
	}
	var groupDefinitions []GroupDefinition
	if groupDefinitions, err = loadEventDefinitions(eventFilePath, metadata); err != nil {
		log.Printf("failed to load event definitions: %v", err)
		return exitError
	}
	var metricDefinitions []MetricDefinition
	if metricDefinitions, err = loadMetricDefinitions(metricFilePath, groupDefinitions, metadata); err != nil {
		log.Printf("failed to load metric definitions: %v", err)
		return exitError
	}
	if gVerbose {
		log.Printf("%s", metadata)
	}
	functions := getEvaluatorFunctions()
	var nmiWatchdog string
	if nmiWatchdog, err = getNmiWatchdog(); err != nil {
		if !gDebug {
			log.Printf("failed to retrieve NMI watchdog status: %v", err)
			return exitError
		}
	}
	if nmiWatchdog != "0" {
		if err = setNmiWatchdog("0"); err != nil {
			if !gDebug {
				log.Printf("failed to set NMI watchdog status: %v", err)
				return exitError
			}
		}
		defer setNmiWatchdog(nmiWatchdog)
	}
	var perfMuxIntervals map[string]string
	if perfMuxIntervals, err = getMuxIntervals(); err != nil {
		log.Printf("failed to get perf mux intervals: %v", err)
		return exitError
	}
	if err = setAllMuxIntervals(muxInterval); err != nil {
		if !gDebug {
			log.Printf("failed to set all perf mux intervals to %d: %v", muxInterval, err)
			return exitError
		}
	}
	defer setMuxIntervals(perfMuxIntervals)
	if !printCSV {
		fmt.Print(".\n")
		fmt.Printf("Reporting metrics in %d second intervals...\n", metricInterval)
	}
	// run perf in a goroutine, get events through eventChannel
	eventChannel := make(chan []string)
	perfCtx, perfError := context.WithCancel(context.Background())
	defer perfError()
	go runPerf(groupDefinitions, eventChannel, runSeconds, metadata, perfError)
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
				if metrics, frameTimestamp, err = processEvents(perfEvents, metricDefinitions, functions, frameTimestamp, metadata); err != nil {
					log.Printf("%v", err)
					return exitError
				}
				if printCSV {
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
