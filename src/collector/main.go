/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
	"intel.com/svr-info/pkg/commandfile"
	"intel.com/svr-info/pkg/core"
)

// globals
var (
	gVersion string = "dev" // build overrides this, see makefile
)

type ResultType map[string]string

type RunConfiguration struct {
	cmdFile commandfile.CommandFile
	sudo    string
}

func newRunConfiguration(yamlData []byte) (config *RunConfiguration, err error) {
	config = new(RunConfiguration)
	err = yaml.Unmarshal(yamlData, &(config.cmdFile))
	return
}

func showUsage() {
	fmt.Printf("%s Version: %s\n", filepath.Base(os.Args[0]), gVersion)
	fmt.Println("Reads password from environment variable SUDO_PASSWORD, if provided.")
	fmt.Println("Usage:")
	fmt.Println("  [SUDO_PASSWORD=*********] collector < file[.yaml]")
	fmt.Println("  [SUDO_PASSWORD=*********] collector [OPTION...] file[.yaml]")
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println(
		`YAML Format:
  Root level keys:
      arguments
      commands
  Required arguments:
    name - a string that will be the primary key of the output
  Optional arguments
      bin_path - a string containing the path to executables
  Commands are list items. Command names label the command output.
  Required command attributes:
      command - will be executed by bash:
  Optional command attributes:
      superuser: bool indicates need for elevated privilege (default: false)
      run: bool indicates if command will be run (default: true)
      modprobe: comma separated list of kernel modules required to run command
      parallel: bool indicates if command can be run in parallel with other commands (default: false)`)
	fmt.Println(
		`YAML Example:
    arguments:
        name: json output will be a dictionary with this -name- as the root key
        bin_path: .
		command_timeout: 300
    commands:
    - date -u:
        command: date -u
        parallel: true
    - cpuid -1:
        command: cpuid -1 | grep family
        modprobe: cpuid
        parallel: true`)
}

func printResult(out io.Writer, result ResultType, firstCommand bool) error {
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if firstCommand {
		fmt.Fprintf(out, "%s\n", string(b))
	} else {
		fmt.Fprintf(out, ",%s\n", string(b))
	}
	return nil
}

func runConfigCommand(cmd commandfile.Command, args commandfile.Arguments, sudo string, ch chan ResultType) {
	result := make(ResultType)
	result["label"] = cmd.Label
	result["command"] = cmd.Command
	if cmd.Superuser {
		result["superuser"] = "true"
	} else {
		result["superuser"] = "false"
	}
	stdout, stderr, exitCode, err := runCommand(cmd.Label, cmd.Command, cmd.Superuser, sudo, args.Binpath, args.Timeout)
	if err != nil {
		log.Printf("Error: %v Stderr: %s, Exit Code: %d", err, stderr, exitCode)
	}
	result["stdout"] = stdout
	result["stderr"] = stderr
	result["exitstatus"] = fmt.Sprint(exitCode)
	ch <- result
}

func runConfigCommands(config *RunConfiguration, out io.Writer) error {
	// build a unique list of loadable kernel modules that must be installed
	install := make(map[string]int)
	for _, cmd := range config.cmdFile.Commands {
		if cmd.Run && cmd.Modprobe != "" {
			modList := strings.Split(cmd.Modprobe, ",")
			for _, mod := range modList {
				install[mod] = 1
			}
		}
	}
	// install all loadable kernel modules
	mods := make([]string, 0, len(install))
	for mod := range install {
		mods = append(mods, mod)
	}
	modList := strings.Join(mods, ",")
	installedMods := installMods(modList, config.sudo)
	defer uninstallMods(installedMods, config.sudo)
	// separate commands into parallel (those that can run in parallel) and serial
	var parallelCommands []commandfile.Command
	var serialCommands []commandfile.Command
	for _, cmd := range config.cmdFile.Commands {
		if cmd.Run {
			if cmd.Parallel {
				parallelCommands = append(parallelCommands, cmd)
			} else {
				serialCommands = append(serialCommands, cmd)
			}
		}
	}
	// run serial commands one at a time
	// we run these first because they, typically, are more time sensitive...especially for profiling
	ch := make(chan ResultType)
	for idx, cmd := range serialCommands {
		go runConfigCommand(cmd, config.cmdFile.Args, config.sudo, ch)
		result := <-ch
		err := printResult(out, result, idx == 0)
		if err != nil {
			log.Printf("Error: %v", err)
			return err
		}
	}
	// run parallel commands in parallel goroutines
	for _, cmd := range parallelCommands {
		go runConfigCommand(cmd, config.cmdFile.Args, config.sudo, ch)
	}
	for idx := range parallelCommands {
		result := <-ch
		err := printResult(out, result, (idx+len(serialCommands)) == 0)
		if err != nil {
			log.Printf("Error: %v", err)
			return err
		}
	}
	return nil
}

func mainReturnWithCode() int {
	var showHelp bool
	var showVersion bool
	flag.Usage = func() { showUsage() } // override default usage output
	flag.BoolVar(&showHelp, "h", false, "Print this usage message.")
	flag.BoolVar(&showVersion, "v", false, "Print program version.")
	flag.Parse()
	if showHelp {
		showUsage()
		return 0
	}
	if showVersion {
		fmt.Println(gVersion)
		return 0
	}

	// configure logging
	logFilename := filepath.Base(os.Args[0]) + ".log"
	logFile, err := os.OpenFile(logFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Error: %v", err)
		return 1
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	log.Printf("Starting up %s, version %s, PID %d, PPID %d, arguments: %s",
		filepath.Base(os.Args[0]),
		gVersion,
		os.Getpid(),
		os.Getppid(),
		strings.Join(os.Args, " "),
	)

	// write pid to file
	pidFilename := filepath.Base(os.Args[0]) + ".pid"
	pidFile, err := os.OpenFile(pidFilename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Error: %v", err)
		return 1
	}
	pidFile.WriteString(fmt.Sprintf("%d", os.Getpid()))
	pidFile.Close()

	// read input
	var data []byte
	if flag.NArg() == 0 {
		log.Print("Reading data from stdin")
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			log.Printf("Error: %v", err)
			return 1
		}
	} else if flag.NArg() == 1 {
		absFilename, err := core.AbsPath(flag.Arg(0))
		if err != nil {
			log.Printf("Error: %v", err)
			return 1
		}
		log.Printf("Reading data from file: %s", absFilename)
		data, err = os.ReadFile(absFilename)
		if err != nil {
			log.Printf("Error: %v", err)
			return 1
		}
	} else {
		log.Print("Incorrect usage.")
		showUsage()
		return 1
	}

	// parse input data into config
	runConfig, err := newRunConfiguration(data)
	if err != nil {
		log.Printf("Error: %v", err)
		return 1
	}
	runConfig.sudo = os.Getenv("SUDO_PASSWORD")

	// start json
	fmt.Printf("{\n\"%s\": [\n", runConfig.cmdFile.Args.Name)

	// run commands - prints json formatted output for each command
	err = runConfigCommands(runConfig, os.Stdout)
	if err != nil {
		return 1
	}

	// end json
	fmt.Printf("]\n}\n")

	log.Print("All done.")

	return 0
}

func main() { os.Exit(mainReturnWithCode()) }
