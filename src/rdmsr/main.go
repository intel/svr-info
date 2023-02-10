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

	"intel.com/svr-info/pkg/msr"
)

type CmdLineArgs struct {
	help      bool
	version   bool
	all       bool
	processor int
	socket    bool
	bitrange  string
	msr       uint64
}

// globals
var (
	gVersion     string = "dev" // build overrides this, see makefile
	gCmdLineArgs CmdLineArgs
)

func showUsage() {
	appName := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "Usage: %s <args> msr\n", appName)
	fmt.Fprintf(os.Stderr, "Example: %s -p 1 0x123\n", appName)
	flag.PrintDefaults()
}

func showVersion() {
	fmt.Println(gVersion)
}

func parseBitrangeArg() (highBit, lowBit int, err error) {
	bitrangeOK := false
	fields := strings.Split(gCmdLineArgs.bitrange, ":")
	if len(fields) == 2 {
		highBit, err = strconv.Atoi(fields[0])
		if err == nil && highBit > 0 && highBit <= 63 {
			lowBit, err = strconv.Atoi(fields[1])
			if err == nil && lowBit >= 0 && lowBit < 63 {
				if highBit > lowBit {
					bitrangeOK = true
				}
			}
		}
	}
	if !bitrangeOK {
		err = fmt.Errorf("failed to parse bit range: %s", gCmdLineArgs.bitrange)
	}
	return
}

func init() {
	// init command line flags
	flag.Usage = func() { showUsage() } // override default usage output
	flag.BoolVar(&gCmdLineArgs.help, "h", false, "Print this usage message.")
	flag.BoolVar(&gCmdLineArgs.version, "v", false, "Print program version.")
	flag.BoolVar(&gCmdLineArgs.all, "a", false, "Read for all processors.")
	flag.IntVar(&gCmdLineArgs.processor, "p", 0, "Select processor number.")
	flag.BoolVar(&gCmdLineArgs.socket, "s", false, "Read for one processor on each socket (package/CPU).")
	flag.StringVar(&gCmdLineArgs.bitrange, "f", "", "Output bits [h:l] only")
	flag.Parse()
	if gCmdLineArgs.help || gCmdLineArgs.version {
		return
	}
	// positional arg
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	} else {
		msrHex := flag.Arg(0)
		if len(msrHex) > 2 && msrHex[:2] == "0x" {
			msrHex = msrHex[2:]
		}
		msr, err := strconv.ParseInt(msrHex, 16, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not parse provided msr address: %v\n", err)
			showUsage()
			os.Exit(1)
		}
		gCmdLineArgs.msr = uint64(msr)
	}
	// validate input flag arguments
	if gCmdLineArgs.bitrange != "" {
		_, _, err := parseBitrangeArg()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			showUsage()
			os.Exit(1)
		}
	}
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
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if gCmdLineArgs.bitrange != "" {
		highBit, lowBit, _ := parseBitrangeArg()
		err = msrReader.SetBitRange(highBit, lowBit)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	var vals []uint64
	if gCmdLineArgs.all {
		vals, err = msrReader.ReadAll(gCmdLineArgs.msr)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else if gCmdLineArgs.socket {
		vals, err = msrReader.ReadPackages(gCmdLineArgs.msr)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else {
		val, err := msrReader.ReadOne(gCmdLineArgs.msr, gCmdLineArgs.processor)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		vals = append(vals, val)
	}
	format := "%016x\n"
	if gCmdLineArgs.bitrange != "" { // don't pad output if bitrange requested
		format = "%x\n"
	}
	for _, val := range vals {
		fmt.Printf(format, val)
	}
	return 0
}

func main() { os.Exit(mainReturnWithCode()) }
