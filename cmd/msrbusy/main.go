/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/intel/svr-info/internal/msr"
)

type CmdLineArgs struct {
	help       bool
	version    bool
	processor  int
	iterations int
	msrs       []uint64
}

// globals
var (
	gVersion     string = "dev" // build overrides this, see makefile
	gCmdLineArgs CmdLineArgs
)

func showUsage() {
	appName := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "Usage: %s <args> msr1 msr2 msr3\n", appName)
	fmt.Fprintf(os.Stderr, "Example: %s -i 6 -p 0 0x123 0x234\n", appName)
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
	flag.IntVar(&gCmdLineArgs.iterations, "i", 6, "Number of iterations.")
	flag.IntVar(&gCmdLineArgs.processor, "p", 0, "Select processor number.")
	flag.Parse()
	if gCmdLineArgs.help || gCmdLineArgs.version {
		return
	}
	// positional args
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	} else {
		for _, arg := range flag.Args() {
			if len(arg) > 2 && arg[:2] == "0x" {
				arg = arg[2:]
			}
			msr, err := strconv.ParseInt(arg, 16, 0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Could not parse provided msr address: %s, %v\n", arg, err)
				showUsage()
				os.Exit(1)
			}
			gCmdLineArgs.msrs = append(gCmdLineArgs.msrs, uint64(msr))
		}
	}
}

type msrVals struct {
	msrTxt string
	msr    uint64
	vals   []uint64
}

func getMSRVals(msrReader *msr.MSR, msrTxt string, msrNum uint64, processor int, iterations int, ch chan msrVals) {
	var m msrVals
	m.msrTxt = msrTxt
	m.msr = msrNum
	for i := 0; i < iterations; i++ {
		var vals []uint64
		if processor == 0 {
			//read msr off of core 0 on processor 0
			val, err := msrReader.ReadOne(msrNum, 0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read MSR %s: %v\n", msrTxt, err)
				break
			}
			vals = append(vals, val)
		} else {
			// ReadPackages will fail if PPID msr can't be read, so only call it if processor > 0
			var err error
			vals, err = msrReader.ReadPackages(msrNum)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read package MSR %s: %v\n", msrTxt, err)
				break
			}
		}
		if len(vals) <= processor {
			fmt.Fprintf(os.Stderr, "Invalid processor number specified: %d\n", processor)
			break
		}
		m.vals = append(m.vals, vals[processor])
	}
	ch <- m
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
	msrReader, err := msr.NewMSR()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		return 1
	}
	// run in parallel
	ch := make(chan msrVals)
	for i, msr := range gCmdLineArgs.msrs {
		go getMSRVals(msrReader, flag.Arg(i), msr, gCmdLineArgs.processor, gCmdLineArgs.iterations, ch)
	}
	// wait for completion
	msrVals := make(map[string][]uint64)
	for range gCmdLineArgs.msrs {
		x := <-ch
		msrVals[x.msrTxt] = x.vals
	}
	var results []string
	for _, msrTxt := range flag.Args() {
		var prevVal uint64
		busy := false
		for i, val := range msrVals[msrTxt] {
			if i != 0 {
				if val != prevVal {
					busy = true
					break
				}
			}
			prevVal = val
		}
		if len(msrVals[msrTxt]) > 1 {
			if busy {
				results = append(results, "Active")
			} else {
				results = append(results, "Inactive")
			}
		} else {
			results = append(results, "Unknown")
		}
	}
	fmt.Printf("%s\n", strings.Join(flag.Args(), "|"))
	fmt.Printf("%s\n", strings.Join(results, "|"))
	return 0
}

func main() { os.Exit(mainReturnWithCode()) }
