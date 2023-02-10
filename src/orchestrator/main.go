/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
	"intel.com/svr-info/pkg/core"
	"intel.com/svr-info/pkg/progress"
	"intel.com/svr-info/pkg/target"
)

// globals
var (
	gVersion string = "dev" // build overrides this, see makefile
)

func getTargets(assets *core.Assets, cmdLineArgs *CmdLineArgs) (targets []target.Target, err error) {
	// if we have a targets file
	if cmdLineArgs.targets != "" {
		targetsFile := newTargetsFile(cmdLineArgs.targets)
		var targetsFromFile []targetFromFile
		targetsFromFile, err = targetsFile.parse()
		if err != nil {
			return
		}
		for _, t := range targetsFromFile {
			if t.ip == "localhost" { // special case, "localhost" in targets file
				var hostname string
				hostname, err = os.Hostname()
				if err != nil {
					return
				}
				localTarget := target.NewLocalTarget(hostname, t.sudo)
				if !localTarget.CanElevatePrivileges() {
					log.Print("local target in targets file without root privileges.")
					fmt.Println("WARNING: User does not have root privileges. Not all data will be collected.")
				}
				targets = append(targets, localTarget)
			} else {
				targets = append(targets, target.NewRemoteTarget(t.label, t.ip, t.port, t.user, t.key, t.pwd, assets[core.Sshpass], t.sudo))
			}
		}
	} else {
		// if collecting on localhost
		if cmdLineArgs.ipAddress == "" {
			var hostname string
			hostname, err = os.Hostname()
			if err != nil {
				return
			}
			localTarget := target.NewLocalTarget(hostname, "")
			// ask for password if can't elevate privileges without it, but only if getting
			// input from a terminal, i.e., not from a script (for testing)
			if !localTarget.CanElevatePrivileges() {
				fmt.Println("WARNING:  Some data items cannot be collected without elevated privileges.")
				if !term.IsTerminal(int(os.Stdin.Fd())) {
					log.Print("NOT prompting for password because STDIN isn't coming from a terminal.")
				} else {
					log.Print("Prompting for password.")
					fmt.Print("To collect all data, enter sudo password followed by Enter. Otherwise, press Enter:")
					var pwd []byte
					pwd, err = term.ReadPassword(0)
					if err != nil {
						return
					}
					fmt.Printf("\n") // newline after password
					localTarget.SetSudo(string(pwd))
					if localTarget.GetSudo() != "" && !localTarget.CanElevatePrivileges() {
						log.Print("Password provided but failed to elevate privileges.")
						fmt.Println("WARNING: Not able to establish elevated privileges with provided password.")
						fmt.Println("Continuing with regular user privileges. Some data will not be collected.")
						localTarget.SetSudo("")
					}
				}
			}
			targets = append(targets, localTarget)
		} else {
			targets = append(targets, target.NewRemoteTarget(cmdLineArgs.ipAddress, cmdLineArgs.ipAddress, fmt.Sprintf("%d", cmdLineArgs.port), cmdLineArgs.user, cmdLineArgs.key, "", "", ""))
		}
	}
	return
}

// go routine
func doCollection(collection *Collection, ch chan *Collection, statusUpdate progress.MultiSpinnerUpdateFunc) {
	if statusUpdate != nil {
		statusUpdate(collection.target.GetName(), "collecting data")
	}
	err := collection.Collect()
	if err != nil {
		log.Printf("Error: %v", err)
		if statusUpdate != nil {
			statusUpdate(collection.target.GetName(), "error collecting data")
		}
	} else {
		if statusUpdate != nil {
			statusUpdate(collection.target.GetName(), "finished collecting data")
		}
	}
	ch <- collection
}

func getCollections(targets []target.Target, assets *core.Assets, workDir string, cmdLineArgs *CmdLineArgs, statusUpdate progress.MultiSpinnerUpdateFunc) (collections []*Collection, err error) {
	// run collections in parallel
	ch := make(chan *Collection)
	for _, target := range targets {
		collection := newCollection(target, cmdLineArgs, assets, workDir)
		go doCollection(collection, ch, statusUpdate)
	}
	// wait for all collections to complete collecting
	for range targets {
		collection := <-ch
		collections = append(collections, collection)
	}
	return
}

func getReports(collections []*Collection, assets *core.Assets, outputDir string, cmdLineArgs *CmdLineArgs, statusUpdate progress.MultiSpinnerUpdateFunc) (reportFilePaths []string, err error) {
	var okCollections = make([]*Collection, 0)
	for _, collection := range collections {
		if collection.ok {
			okCollections = append(okCollections, collection)
			if statusUpdate != nil {
				statusUpdate(collection.target.GetName(), "creating report(s)")
			}
		}
	}
	if len(okCollections) == 0 {
		err = fmt.Errorf("no data collected")
		return
	}
	var collectionFilePaths []string
	for _, collection := range okCollections {
		collectionFilePaths = append(collectionFilePaths, collection.outputFilePath)
	}
	cmd := exec.Command(assets[core.Reporter], "-input", strings.Join(collectionFilePaths, ","), "-output", outputDir, "-format", cmdLineArgs.format)
	log.Printf("run: %s", strings.Join(cmd.Args, " "))
	stdout, _, _, err := target.RunLocalCommand(cmd)
	if err != nil {
		for _, collection := range collections {
			if statusUpdate != nil {
				statusUpdate(collection.target.GetName(), "error creating report(s)")
			}
		}
		return
	}
	reportFilePaths = strings.Split(stdout, "\n")
	reportFilePaths = reportFilePaths[:len(reportFilePaths)-1]
	for _, collection := range collections {
		if collection.ok {
			if statusUpdate != nil {
				statusUpdate(collection.target.GetName(), "finished creating report(s)")
			}
		}
	}
	return
}

func archiveOutputDir(outputDir string) (err error) {
	tarFilePath := filepath.Join(outputDir, filepath.Base(outputDir)+".tgz")
	out, err := os.Create(tarFilePath)
	if err != nil {
		return
	}
	defer out.Close()
	gw := gzip.NewWriter(out)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	baseDir, err := os.Getwd()
	if err != nil {
		return
	}
	err = os.Chdir(outputDir)
	if err != nil {
		return
	}
	defer os.Chdir(baseDir)
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Base(path) != filepath.Base(tarFilePath) {
			info, err := d.Info()
			if err != nil {
				return err
			}
			var header *tar.Header
			header, err = tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}
			header.Name = filepath.Join(filepath.Base(outputDir), path)
			err = tw.WriteHeader(header)
			if err != nil {
				return err
			}
			var file *os.File
			file, err = os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, file)
			file.Close()
			if err != nil {
				return err
			}
		}
		return nil
	})
	return
}

func cleanupOutputDir(outputDir string, collections []*Collection, reportFilePaths []string) (err error) {
	var filesToRemove []string
	for _, collection := range collections {
		hostname := collection.target.GetName()
		filesToRemove = append(filesToRemove, filepath.Join(outputDir, getLogfileName()))
		filesToRemove = append(filesToRemove, filepath.Join(outputDir, hostname+"_reports_collector.yaml"))
		filesToRemove = append(filesToRemove, filepath.Join(outputDir, hostname+"_collector.log"))
		filesToRemove = append(filesToRemove, filepath.Join(outputDir, hostname+"_megadata_collector.yaml"))
		filesToRemove = append(filesToRemove, filepath.Join(outputDir, hostname+"_megadata_collector.log"))
		filesToRemove = append(filesToRemove, filepath.Join(outputDir, hostname+"_megadata", "collector.log"))
		filesToRemove = append(filesToRemove, filepath.Join(outputDir, hostname+"_megadata", "collector.pid"))
		filesToRemove = append(filesToRemove, filepath.Join(outputDir, hostname+".raw.json"))
	}
	filesToRemove = append(filesToRemove, filepath.Join(outputDir, "reporter.log"))
	for _, file := range filesToRemove {
		os.Remove(file)
	}
	return
}

func doWork(outputDir string, cmdLineArgs *CmdLineArgs) error {
	assets, err := core.NewAssets()
	if err != nil {
		return err
	}
	_, nomatch, nodata, err := assets.Verify()
	if err != nil {
		log.Printf("Warning: %v", err)
	}
	for _, a := range nomatch {
		log.Printf("checksum does not match -- %s has been modified.", a)
	}
	for _, a := range nodata {
		log.Printf("checksum not found for %s.", a)
	}
	if cmdLineArgs.dumpConfig {
		customized, err := customizeCommandYAML(assets[core.ReportsYaml], cmdLineArgs, ".", "target_hostname")
		if err != nil {
			return err
		}
		fmt.Print(string(customized))
		return nil
	}
	targets, err := getTargets(assets, cmdLineArgs)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("no targets provided")
	}
	multiSpinner := progress.NewMultiSpinner()
	for _, t := range targets {
		multiSpinner.AddSpinner(t.GetName())
	}
	multiSpinner.Start()
	defer multiSpinner.Finish()
	collections, err := getCollections(targets, assets, outputDir, cmdLineArgs, multiSpinner.Status)
	if err != nil {
		return err
	}
	var reportFilePaths []string
	reportFilePaths, err = getReports(collections, assets, outputDir, cmdLineArgs, multiSpinner.Status)
	if err != nil {
		return err
	}
	err = archiveOutputDir(outputDir)
	if err != nil {
		return err
	}
	if !cmdLineArgs.debug {
		err = cleanupOutputDir(outputDir, collections, reportFilePaths)
		if err != nil {
			return err
		}
	}
	multiSpinner.Finish()
	fmt.Print("Reports:\n")
	for _, reportFilePath := range reportFilePaths {
		relativePath, err := filepath.Rel(filepath.Join(outputDir, ".."), reportFilePath)
		if err != nil {
			return err
		}
		fmt.Printf("  %s\n", relativePath)
	}
	return nil
}

func getLogfileName() string {
	return filepath.Base(os.Args[0]) + ".log"
}

const (
	retNoError = 0
	retError   = 1
)

func mainReturnWithCode() int {
	// command line
	cmdLineArgs := newCmdLineArgs()
	err := cmdLineArgs.parse(os.Args[0], os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return retError
	}
	err = cmdLineArgs.validate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return retError
	}
	// show help
	if cmdLineArgs.help {
		showUsage()
		return retNoError
	}
	// show version
	if cmdLineArgs.version {
		showVersion()
		return retNoError
	}
	// output directory
	var outputDir string
	if cmdLineArgs.output != "" {
		var err error
		outputDir, err = core.AbsPath(cmdLineArgs.output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return retError
		}
	} else {
		outputDirName := filepath.Base(os.Args[0]) + "_" + time.Now().Local().Format("2006-01-02_15-04-05")
		var err error
		// outputDir will be created in current working directory
		outputDir, err = core.AbsPath(outputDirName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return retError
		}
		err = os.Mkdir(outputDir, 0755)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return retError
		}
	}
	// logging
	logFilename := getLogfileName()
	logFile, err := os.OpenFile(filepath.Join(outputDir, logFilename), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return retError
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
	// get to work
	err = doWork(outputDir, cmdLineArgs)
	if err != nil {
		log.Printf("Error: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return retError
	}
	return retNoError
}

func main() { os.Exit(mainReturnWithCode()) }
