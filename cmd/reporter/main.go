/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/intel/svr-info/internal/core"
	"github.com/intel/svr-info/internal/cpu"
	"github.com/intel/svr-info/internal/util"
)

//go:embed resources
var resources embed.FS

type CmdLineArgs struct {
	help         bool
	version      bool
	format       string
	input        string
	output       string
	internalJSON bool
}

// globals
var (
	gVersion     string = "dev" // build overrides this, see makefile
	gCmdLineArgs CmdLineArgs
)

func showUsage() {
	flag.PrintDefaults()
}

func showVersion() {
	fmt.Println(gVersion)
}

func init() {
	// init command line flags
	flag.Usage = func() { showUsage() } // override default usage output
	flag.BoolVar(&gCmdLineArgs.help, "h", false, "Print this usage message.")
	flag.BoolVar(&gCmdLineArgs.version, "v", false, "Print program version.")
	flag.StringVar(&gCmdLineArgs.format, "format", "html", "comma separated list of desired report format(s):"+strings.Join(core.ReportTypes[:len(core.ReportTypes)-1], ", ")+", or all")
	flag.StringVar(&gCmdLineArgs.input, "input", "", "required, comma separated list of input files or directory containing input (*.raw.json) files")
	flag.StringVar(&gCmdLineArgs.output, "output", ".", "output directory")
	flag.BoolVar(&gCmdLineArgs.internalJSON, "internal_json", false, "Produce the internal json format introduced in the 2.0 release. This option is deprecated. Recommend transitioning to the new JSON report format ASAP.")
	flag.Parse()
	// validate input flag arguments
	// -format
	if gCmdLineArgs.format != "" {
		reportTypes := strings.Split(gCmdLineArgs.format, ",")
		for _, reportType := range reportTypes {
			if !core.IsValidReportType(reportType) {
				fmt.Fprintf(os.Stderr, "-report %s : invalid report type: %s\n", gCmdLineArgs.format, reportType)
				os.Exit(1)
			}
		}
	}
	// -input
	if gCmdLineArgs.input != "" {
		inputPaths := strings.Split(gCmdLineArgs.input, ",")
		for _, inputPath := range inputPaths {
			path, err := util.AbsPath(inputPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			fileInfo, err := os.Stat(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "-input %s : file (or directory) does not exist\n", path)
				os.Exit(1)
			}
			if !fileInfo.Mode().IsRegular() && !fileInfo.Mode().IsDir() {
				fmt.Fprintf(os.Stderr, "-input %s : must be a file or directory\n", path)
				os.Exit(1)
			}
		}
	} else if !gCmdLineArgs.help && !gCmdLineArgs.version {
		fmt.Fprintf(os.Stderr, "-input : input file list or directory is required\n")
		showUsage()
		os.Exit(1)
	}
	// -output
	if gCmdLineArgs.output != "" {
		path, err := util.AbsPath(gCmdLineArgs.output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fileInfo, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "-output %s : directory does not exist\n", path)
			os.Exit(1)
		}
		if !fileInfo.IsDir() {
			fmt.Fprintf(os.Stderr, "-output %s : must be a directory\n", path)
			os.Exit(1)
		}
	}
}

func getInputFilePaths(input string) (inputFilePaths []string, err error) {
	paths := strings.Split(input, ",")
	for _, filename := range paths {
		var fileInfo fs.FileInfo
		fileInfo, err = os.Stat(filename)
		if err != nil {
			err = fmt.Errorf("%w: %s", err, filename)
			return
		}
		if fileInfo.Mode().IsRegular() {
			inputFilePaths = append(inputFilePaths, filename)
		} else if fileInfo.IsDir() {
			var matches []string
			matches, err = filepath.Glob(filepath.Join(filename, "*.raw.json"))
			if err != nil {
				return
			}
			inputFilePaths = append(inputFilePaths, matches...)
		}
	}
	return
}

func getOutputDir(input string) (outputDir string, err error) {
	fileInfo, err := os.Stat(input)
	if err != nil {
		err = fmt.Errorf("%w: %s", err, input)
		return
	}
	if !fileInfo.IsDir() {
		err = fmt.Errorf("%s is not a directory", input)
		return
	}
	outputDir = input
	return
}

func getSources(inputFilePaths []string) (sources []*Source) {
	for _, inputFilePath := range inputFilePaths {
		source := newSource(inputFilePath)
		err := source.parse()
		if err != nil {
			log.Printf("Failed to parse %s: %v", inputFilePath, err)
			continue
		}
		sources = append(sources, source)
	}
	return
}

func getReports(sources []*Source, reportTypes []string, outputDir string) (reportFilePaths []string, err error) {
	cpusInfo, err := cpu.NewCPU()
	if err != nil {
		return
	}
	configReport := NewConfigurationReport(sources, cpusInfo)
	briefReport := NewBriefReport(sources, configReport, cpusInfo)
	profileReport := NewProfileReport(sources)
	analyzeReport := NewAnalyzeReport(sources)
	benchmarkReport := NewBenchmarkReport(sources)
	insightsReport := NewInsightsReport(sources, configReport, briefReport, profileReport, benchmarkReport, analyzeReport, cpusInfo)
	var rpt ReportGenerator
	for _, rt := range reportTypes {
		switch rt {
		case "html":
			rpt = newReportGeneratorHTML(outputDir, cpusInfo, configReport, insightsReport, profileReport, benchmarkReport, analyzeReport)
		case "json":
			if gCmdLineArgs.internalJSON {
				rpt = newReportGeneratorJSON(outputDir, configReport, insightsReport, profileReport, benchmarkReport, analyzeReport)
			} else {
				rpt = newReportGeneratorJSONSimplified(outputDir, configReport, briefReport, insightsReport, profileReport, benchmarkReport, analyzeReport)
			}
		case "xlsx":
			rpt = newReportGeneratorXLSX(outputDir, configReport, briefReport, insightsReport, profileReport, benchmarkReport, analyzeReport) // only Excel has 'brief' report
		case "txt":
			rpt = newReportGeneratorTXT(sources, outputDir) // txt report is special...more of a raw data dump than a report
		default:
			err = fmt.Errorf("unsupported report type: %s", rt)
			return
		}
		var reportPaths []string
		reportPaths, err = rpt.generate()
		if err != nil {
			return
		}
		reportFilePaths = append(reportFilePaths, reportPaths...)
	}
	return
}

func mainReturnWithCode() int {
	if gCmdLineArgs.help {
		showUsage()
		return 0
	}
	if gCmdLineArgs.version {
		showVersion()
		return 0
	}
	outputDir, err := getOutputDir(gCmdLineArgs.output)
	if err != nil {
		log.Printf("Error: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	logFilename := filepath.Base(os.Args[0]) + ".log"
	logFile, err := os.OpenFile(filepath.Join(outputDir, logFilename), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Error: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	log.Printf("Starting up %s, version %s, PID %d, PPID %d, arguments: %s",
		filepath.Base(os.Args[0]),
		gVersion,
		os.Getpid(),
		os.Getppid(),
		strings.Join(os.Args, " "),
	)
	inputFilePaths, err := getInputFilePaths(gCmdLineArgs.input)
	if err != nil {
		log.Printf("Error: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	reportTypes, err := core.GetReportTypes(gCmdLineArgs.format)
	if err != nil {
		log.Printf("Error: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	sources := getSources(inputFilePaths)
	if len(sources) == 0 {
		err = fmt.Errorf("no input files found")
		log.Printf("Error: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	reportFilePaths, err := getReports(sources, reportTypes, outputDir)
	if err != nil {
		log.Printf("Error: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	for _, reportFilePath := range reportFilePaths {
		log.Printf("Created report: %s", reportFilePath)
		fmt.Println(reportFilePath)
	}
	return 0
}

func main() { os.Exit(mainReturnWithCode()) }
