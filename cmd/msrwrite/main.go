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

	"github.com/intel/svr-info/internal/msr"
)

type CmdLineArgs struct {
	help      bool
	version   bool
	all       bool
	processor int
	msr       uint64
	val       uint64
}

// globals
var (
	gVersion     string = "dev" // build overrides this, see makefile
	gCmdLineArgs CmdLineArgs
)

func showUsage() {
	appName := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "Usage: %s <args> msr value\n", appName)
	fmt.Fprintf(os.Stderr, "Example: %s -p 1 0x123 0xabc\n", appName)
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
	flag.BoolVar(&gCmdLineArgs.all, "a", false, "Write for all processors.")
	flag.IntVar(&gCmdLineArgs.processor, "p", 0, "Select processor number. Default 0.")
	flag.Parse()
	if gCmdLineArgs.help || gCmdLineArgs.version {
		return
	}
	// positional arg
	if flag.NArg() < 2 {
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

		valHex := flag.Arg(1)
		if len(valHex) > 2 && valHex[:2] == "0x" {
			valHex = valHex[2:]
		}
		val, err := strconv.ParseInt(valHex, 16, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not parse provided msr value: %v\n", err)
			showUsage()
			os.Exit(1)
		}
		gCmdLineArgs.val = uint64(val)
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
	msrWriter, err := msr.NewMSR()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if gCmdLineArgs.all {
		err = msrWriter.WriteAll(gCmdLineArgs.msr, gCmdLineArgs.val)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else {
		err := msrWriter.WriteOne(gCmdLineArgs.msr, gCmdLineArgs.processor, gCmdLineArgs.val)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return 0
}

func main() { os.Exit(mainReturnWithCode()) }
