/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/intel/svr-info/internal/core"
	"github.com/intel/svr-info/internal/util"
)

type CmdLineArgs struct {
	help             bool
	version          bool
	format           string
	benchmark        string
	storageDir       string
	profile          string
	profileDuration  int
	profileInterval  int
	analyze          string
	analyzeDuration  int
	analyzeFrequency int
	all              bool
	ipAddress        string
	port             int
	user             string
	key              string
	targets          string
	megadata         bool
	output           string
	targetTemp       string
	temp             string
	printConfig      bool
	noConfig         bool
	cmdTimeout       int
	reporter         string
	collector        string
	debug            bool
}

var benchmarkTypes = []string{"cpu", "frequency", "memory", "storage", "turbo", "all"}
var profileTypes = []string{"cpu", "network", "storage", "memory", "pmu", "power", "all"}
var analyzeTypes = []string{"system", "java", "all"}

func showUsage() {
	fmt.Fprintf(os.Stderr, "usage: %s [-h] [-v]\n", filepath.Base(os.Args[0]))
	fmt.Fprintf(os.Stderr, "                [-format SELECT]\n")
	fmt.Fprintf(os.Stderr, "                [-benchmark SELECT] [-storage_dir DIR]\n")
	fmt.Fprintf(os.Stderr, "                [-profile SELECT] [-profile_duration SECONDS] [-profile_interval N]\n")
	fmt.Fprintf(os.Stderr, "                [-analyze SELECT] [-analyze_duration SECONDS] [-analyze_frequency N]\n")
	fmt.Fprintf(os.Stderr, "                [-megadata]\n")
	fmt.Fprintf(os.Stderr, "                [-ip IP] [-port PORT] [-user USER] [-key KEY] [-targets TARGETS]\n")
	fmt.Fprintf(os.Stderr, "                [-output OUTPUT] [-temp TEMP] [-targettemp TEMP] [-printconfig] [-noconfig] [-cmd_timeout]\n")
	fmt.Fprintf(os.Stderr, "                [-reporter \"args\"] [-collector \"args\"] [-debug]\n")

	longHelp := `
Intel System Health Inspector. Creates configuration, benchmark, profile, analysis, and insights reports for one or more systems.

general arguments:
  -h                    show this help message and exit
  -v                    show version number and exit

report arguments:
  -format SELECT        comma separated list of desired output format(s): %[2]s,
                        e.g., -format json (default: html,xlsx,json)

benchmark arguments:
  -benchmark SELECT     comma separated list of benchmarks: %[3]s,
                        e.g., -benchmark cpu,turbo (default: None)
  -storage_dir DIR      Path to directory on target (default: -temp DIR)

profile arguments:
  -profile SELECT       comma separated list of profile options: %[4]s,
                        e.g., -profile cpu,memory (default: None)
  -profile_duration N   time, in seconds, to collect profiling data (default: 60)
  -profile_interval N   the amount of time in seconds between each sample (default: 2)

analyze arguments:
  -analyze SELECT       comma separated list of profile options: %[5]s,
                        e.g., -analyze system,java (default: None)
  -analyze_duration N   time, in seconds, to collect analysis data (default: 60)
  -analyze_frequency N  the number of samples taken per second (default: 11)

additional data collection arguments:
  -megadata             collect additional data in megadata directory (default: False)

remote target arguments:
  -ip IP                ip address or hostname (default: Nil)
  -port PORT            ssh port (default: 22)
  -user USER            user on remote target (default: Nil)
  -key KEY              local path to ssh private key file (default: Nil)
  -targets TARGETS      path to targets file, one line per target.
                        Line format: 
                           '<label:>ip_address:ssh_port:user_name:private_key_path:ssh_password:sudo_password'
                              - Provide private_key_path or ssh_password.
                        If provided, overrides single target arguments. (default: Nil)

advanced arguments:
  -output DIR           path to output directory. Directory must exist. (default: $PWD/orchestrator_timestamp)
  -temp DIR             path to temporary directory on localhost. Directory must exist. (default: system default)
  -targettemp DIR       path to temporary directory on target. Directory must exist. (default: system default)
  -printconfig          print the collector configuration file and exit (default: False)
  -noconfig             do not collect system configuration data. (default: False)
  -cmd_timeout          the maximum number of seconds to wait for each data collection command (default: 300)
  -reporter             run the the reporter sub-component with args
                        e.g., -reporter "-input /home/rex -output /home/rex -format html" (default: Nil)
  -collector            run the the collector sub-component with args
                        e.g., -collector "collect.yaml" (default: Nil)
  -debug                additional logging and retain temporary files (default: False)

Examples:
$ ./%[1]s
    Collect configuration data on local machine.
$ ./%[1]s -benchmark all
    Collect configuration and benchmark data on local machine.
$ ./%[1]s -profile all -targets ./targets
    Collect configuration and profile data on remote machines defined in targets file.
$ ./%[1]s -format all
    Collect configuration data on local machine. Generate all report formats.
$ ./%[1]s -ip 198.51.100.255 -port 22 -user user83767 -key ~/.ssh/id_rsa
    Collect configuration data on one remote target.
`
	fmt.Fprintf(os.Stderr, longHelp, filepath.Base(os.Args[0]), strings.Join(core.ReportTypes, ","), strings.Join(benchmarkTypes, ","), strings.Join(profileTypes, ","), strings.Join(analyzeTypes, ","))
}

func showVersion() {
	fmt.Println(gVersion)
}

func newCmdLineArgs() *CmdLineArgs {
	cmdLineArgs := CmdLineArgs{}
	return &cmdLineArgs
}

func (cmdLineArgs *CmdLineArgs) parse(name string, arguments []string) (err error) {
	flagSet := flag.NewFlagSet(name, flag.ContinueOnError)
	flagSet.Usage = func() { showUsage() } // override default usage output
	flagSet.BoolVar(&cmdLineArgs.help, "h", false, "")
	flagSet.BoolVar(&cmdLineArgs.version, "v", false, "")
	flagSet.StringVar(&cmdLineArgs.output, "output", "", "")
	flagSet.StringVar(&cmdLineArgs.temp, "temp", "", "")
	flagSet.StringVar(&cmdLineArgs.targetTemp, "targettemp", "", "")
	flagSet.BoolVar(&cmdLineArgs.printConfig, "printconfig", false, "")
	flagSet.BoolVar(&cmdLineArgs.noConfig, "noconfig", false, "")
	flagSet.IntVar(&cmdLineArgs.cmdTimeout, "cmd_timeout", 300, "")
	flagSet.StringVar(&cmdLineArgs.format, "format", "html,xlsx,json", "")
	flagSet.StringVar(&cmdLineArgs.benchmark, "benchmark", "", "")
	flagSet.StringVar(&cmdLineArgs.profile, "profile", "", "")
	flagSet.StringVar(&cmdLineArgs.analyze, "analyze", "", "")
	flagSet.StringVar(&cmdLineArgs.storageDir, "storage_dir", "", "")
	flagSet.BoolVar(&cmdLineArgs.all, "all", false, "")
	flagSet.StringVar(&cmdLineArgs.ipAddress, "ip", "", "")
	flagSet.IntVar(&cmdLineArgs.port, "port", 22, "")
	flagSet.StringVar(&cmdLineArgs.user, "user", "", "")
	flagSet.StringVar(&cmdLineArgs.key, "key", "", "")
	flagSet.StringVar(&cmdLineArgs.targets, "targets", "", "")
	flagSet.BoolVar(&cmdLineArgs.debug, "debug", false, "")
	flagSet.BoolVar(&cmdLineArgs.megadata, "megadata", false, "")
	flagSet.IntVar(&cmdLineArgs.profileDuration, "profile_duration", 60, "")
	flagSet.IntVar(&cmdLineArgs.analyzeDuration, "analyze_duration", 60, "")
	flagSet.IntVar(&cmdLineArgs.profileInterval, "profile_interval", 2, "")
	flagSet.IntVar(&cmdLineArgs.analyzeFrequency, "analyze_frequency", 11, "")
	flagSet.StringVar(&cmdLineArgs.reporter, "reporter", "", "")
	flagSet.StringVar(&cmdLineArgs.collector, "collector", "", "")
	err = flagSet.Parse(arguments)
	if err != nil {
		return
	}
	if flagSet.NArg() != 0 {
		err = fmt.Errorf("unrecognized argument(s): %s", strings.Join(flagSet.Args(), " "))
		return
	}
	return
}

func argDirExists(dir string, label string) (err error) {
	if dir != "" {
		var path string
		path, err = util.AbsPath(dir)
		if err != nil {
			return
		}
		var fileInfo fs.FileInfo
		fileInfo, err = os.Stat(path)
		if err != nil {
			err = fmt.Errorf("-%s %s : directory does not exist", label, path)
			return
		}
		if !fileInfo.IsDir() {
			err = fmt.Errorf("-%s %s : must be a directory", label, path)
			return
		}
	}
	return
}

func isValidType(validTypes []string, input string) (valid bool) {
	inputTypes := strings.Split(input, ",")
	for _, inputType := range inputTypes {
		for _, validType := range validTypes {
			if inputType == validType {
				return true
			}
		}
	}
	return false
}

func (cmdLineArgs *CmdLineArgs) validate() (err error) {
	// -all (deprecated)  TODO: remove the -all option in a future release
	if cmdLineArgs.all {
		fmt.Fprintf(os.Stderr, "\nWARNING: the -all flag is deprecated and will be removed soon. Use '-benchmark all' to run all benchmarks.\n\n")
		cmdLineArgs.benchmark = "all"
	}

	// -output dir
	if cmdLineArgs.output != "" {
		// if dir is specified, make sure it is a dir and that it exists
		err = argDirExists(cmdLineArgs.output, "output")
		if err != nil {
			return
		}
	}
	// -format
	if cmdLineArgs.format != "" {
		if !isValidType(core.ReportTypes, cmdLineArgs.format) {
			err = fmt.Errorf("-format %s : invalid format type: %s", cmdLineArgs.format, cmdLineArgs.format)
			return
		}
	}
	// -benchmark
	if cmdLineArgs.benchmark != "" {
		if !isValidType(benchmarkTypes, cmdLineArgs.benchmark) {
			err = fmt.Errorf("-benchmark %s : invalid benchmark type: %s", cmdLineArgs.benchmark, cmdLineArgs.benchmark)
			return
		}
	}
	// -profile
	if cmdLineArgs.profile != "" {
		if !isValidType(profileTypes, cmdLineArgs.profile) {
			err = fmt.Errorf("-profile %s : invalid profile type: %s", cmdLineArgs.profile, cmdLineArgs.profile)
			return
		}
		if cmdLineArgs.profileDuration <= 0 {
			err = fmt.Errorf("-profile_duration %d : invalid value", cmdLineArgs.profileDuration)
			return
		}
		if cmdLineArgs.profileInterval <= 0 {
			err = fmt.Errorf("-profile_interval %d : invalid value", cmdLineArgs.profileInterval)
			return
		}
		numSamples := cmdLineArgs.profileDuration / cmdLineArgs.profileInterval
		maxSamples := (5 * 60) / 2 // 5 minutes at default interval
		if numSamples > maxSamples {
			err = fmt.Errorf("-profile_duration %d -profile_interval %d may result in too much data. Please reduce total samples (duration/interval) to %d or less", cmdLineArgs.profileDuration, cmdLineArgs.profileInterval, maxSamples)
			return
		}
	}
	// -analyze
	if cmdLineArgs.analyze != "" {
		if !isValidType(analyzeTypes, cmdLineArgs.analyze) {
			err = fmt.Errorf("-analyze %s : invalid profile type: %s", cmdLineArgs.analyze, cmdLineArgs.analyze)
			return
		}
		if cmdLineArgs.analyzeDuration <= 0 {
			err = fmt.Errorf("-analyze_duration %d : invalid value", cmdLineArgs.analyzeDuration)
			return
		}
		if cmdLineArgs.analyzeFrequency <= 0 {
			err = fmt.Errorf("-analyze_frequency %d : invalid value", cmdLineArgs.analyzeFrequency)
			return
		}
		numSamples := cmdLineArgs.analyzeDuration * cmdLineArgs.analyzeFrequency
		maxSamples := (5 * 60) * 11 // 5 minutes at default frequency
		if numSamples > maxSamples {
			err = fmt.Errorf("-analyze_duration %d -analyze_frequency %d may result in too much data. Please reduce total samples (duration*frequency) to %d or less", cmdLineArgs.analyzeDuration, cmdLineArgs.analyzeFrequency, maxSamples)
			return
		}
	}
	// -ip
	if cmdLineArgs.ipAddress != "" {
		// make sure it isn't too long (max FQDN length is 255)
		if len(cmdLineArgs.ipAddress) > 255 {
			err = fmt.Errorf("-ip %s : longer than allowed max (255)", cmdLineArgs.ipAddress)
			return
		}
	}
	if cmdLineArgs.ipAddress != "" && cmdLineArgs.user == "" {
		// if ip is provided, user is required
		err = fmt.Errorf("-user <blank> : user required when -ip %s provided", cmdLineArgs.ipAddress)
		return
	}
	if cmdLineArgs.ipAddress == "" && cmdLineArgs.user != "" {
		// if user is provided, ip is required
		err = fmt.Errorf("-ip <blank> : ip required when -user %s provided", cmdLineArgs.user)
		return
	}
	// -port
	if cmdLineArgs.port <= 0 {
		err = fmt.Errorf("-port %d : port must be a positive integer", cmdLineArgs.port)
		return
	}
	if cmdLineArgs.port != 22 && (cmdLineArgs.ipAddress == "" || cmdLineArgs.user == "") {
		err = fmt.Errorf("-port %d : user and ip required when port provided", cmdLineArgs.port)
		return
	}
	// -key
	if cmdLineArgs.key != "" {
		var path string
		path, err = util.AbsPath(cmdLineArgs.key)
		if err != nil {
			return
		}
		var exists bool
		exists, err = util.FileExists(path)
		if err != nil {
			err = fmt.Errorf("-key %s : %s", path, err.Error())
			return
		}
		if !exists {
			err = fmt.Errorf("-key %s : file does not exist", path)
			return
		}
		if cmdLineArgs.ipAddress == "" || cmdLineArgs.user == "" {
			err = fmt.Errorf("-key %s : user and ip required when key provided", cmdLineArgs.key)
			return
		}
	}
	// -targets
	if cmdLineArgs.targets != "" {
		var path string
		path, err = util.AbsPath(cmdLineArgs.targets)
		if err != nil {
			return
		}
		var exists bool
		exists, err = util.FileExists(path)
		if err != nil {
			err = fmt.Errorf("-targets %s : %s", path, err.Error())
			return
		}
		if !exists {
			err = fmt.Errorf("-targets %s : file does not exist", path)
			return
		}
	}
	// -collector and -reporter are mutually exclusive
	if cmdLineArgs.collector != "" && cmdLineArgs.reporter != "" {
		err = fmt.Errorf("-collector and -reporter are mutually exclusive options")
		return
	}
	return
}
