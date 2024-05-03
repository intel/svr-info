/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/intel/svr-info/internal/commandfile"
	"github.com/intel/svr-info/internal/target"
	"github.com/intel/svr-info/internal/util"
	"gopkg.in/yaml.v2"
)

type Collection struct {
	target         target.Target
	cmdLineArgs    *CmdLineArgs
	outputDir      string
	tempDir        string
	outputFilePath string
	stdout         string
	stderr         string
	ok             bool
}

func newCollection(target target.Target, cmdLineArgs *CmdLineArgs, outputDir string, tempDir string) *Collection {
	c := Collection{
		target:      target,
		cmdLineArgs: cmdLineArgs,
		outputDir:   outputDir,
		tempDir:     tempDir,
		stdout:      "",
		stderr:      "",
		ok:          false,
	}
	return &c
}

// getCommandFilePath returns full local path to target specific command file used by collector
func (c *Collection) getCommandFilePath(extra string) (commandFilePath string) {
	commandFilePath = filepath.Join(c.outputDir, c.target.GetName()+extra+"_collector.yaml")
	return
}

// true if string is in list of strings
func stringInList(s string, l []string) bool {
	for _, item := range l {
		if item == s {
			return true
		}
	}
	return false
}

func customizeCommandYAML(cmdTemplate []byte, cmdLineArgs *CmdLineArgs, targetBinDir string, targetHostName string) (customized []byte, err error) {
	var cf commandfile.CommandFile
	err = yaml.Unmarshal(cmdTemplate, &cf)
	if err != nil {
		return
	}
	cf.Args.Name = targetHostName
	cf.Args.Binpath = targetBinDir
	cf.Args.Timeout = cmdLineArgs.cmdTimeout
	for idx := range cf.Commands {
		cmd := &cf.Commands[idx]
		// set path to the lspci data file
		if cmd.Label == "lspci -vmm" {
			cmd.Command = fmt.Sprintf("lspci -i %s -vmm", filepath.Join(targetBinDir, "pci.ids.gz"))
		}
		optionalCommands := []string{"Memory MLC Bandwidth", "Memory MLC Loaded Latency Test", "stress-ng cpu methods", "avx-turbo", "CPU Turbo Test", "CPU Idle", "fio", "profile", "analyze"}
		if !stringInList(cmd.Label, optionalCommands) {
			if !cmdLineArgs.noConfig {
				cmd.Run = true
			}
		} else {
			// benchmark
			if cmd.Label == "Memory MLC Bandwidth" || cmd.Label == "Memory MLC Loaded Latency Test" {
				cmd.Run = strings.Contains(cmdLineArgs.benchmark, "memory") || strings.Contains(cmdLineArgs.benchmark, "all")
			} else if cmd.Label == "stress-ng cpu methods" {
				cmd.Run = strings.Contains(cmdLineArgs.benchmark, "cpu") || strings.Contains(cmdLineArgs.benchmark, "all")
			} else if cmd.Label == "avx-turbo" {
				cmd.Run = strings.Contains(cmdLineArgs.benchmark, "frequency") || strings.Contains(cmdLineArgs.benchmark, "all")
			} else if cmd.Label == "CPU Turbo Test" || cmd.Label == "CPU Idle" {
				cmd.Run = strings.Contains(cmdLineArgs.benchmark, "turbo") || strings.Contains(cmdLineArgs.benchmark, "all")
			} else if cmd.Label == "fio" {
				cmd.Run = strings.Contains(cmdLineArgs.benchmark, "storage") || strings.Contains(cmdLineArgs.benchmark, "all")
				if cmd.Run {
					fioDir := cmdLineArgs.storageDir
					if fioDir == "" {
						fioDir = targetBinDir
					}
					tmpl := template.Must(template.New("fioCommand").Parse(cmd.Command))
					buf := new(bytes.Buffer)
					err = tmpl.Execute(buf, struct {
						FioDir string
					}{
						FioDir: fioDir,
					})
					if err != nil {
						return
					}
					cmd.Command = buf.String()
				}
			} else if cmd.Label == "profile" {
				cmd.Run = cmdLineArgs.profile != ""
				if cmd.Run {
					tmpl := template.Must(template.New("profileCommand").Parse(cmd.Command))
					buf := new(bytes.Buffer)
					err = tmpl.Execute(buf, struct {
						Duration       int
						Interval       int
						ProfileCPU     bool
						ProfileStorage bool
						ProfileMemory  bool
						ProfileNetwork bool
						ProfilePMU     bool
						ProfilePower   bool
					}{
						Duration:       cmdLineArgs.profileDuration,
						Interval:       cmdLineArgs.profileInterval,
						ProfileCPU:     strings.Contains(cmdLineArgs.profile, "cpu") || strings.Contains(cmdLineArgs.profile, "all"),
						ProfileStorage: strings.Contains(cmdLineArgs.profile, "storage") || strings.Contains(cmdLineArgs.profile, "all"),
						ProfileMemory:  strings.Contains(cmdLineArgs.profile, "memory") || strings.Contains(cmdLineArgs.profile, "all"),
						ProfileNetwork: strings.Contains(cmdLineArgs.profile, "network") || strings.Contains(cmdLineArgs.profile, "all"),
						ProfilePMU:     strings.Contains(cmdLineArgs.profile, "pmu") || strings.Contains(cmdLineArgs.profile, "all"),
						ProfilePower:   strings.Contains(cmdLineArgs.profile, "power") || strings.Contains(cmdLineArgs.profile, "all"),
					})
					if err != nil {
						return
					}
					cmd.Command = buf.String()
				}
			} else if cmd.Label == "analyze" {
				cmd.Run = cmdLineArgs.analyze != ""
				if cmd.Run {
					tmpl := template.Must(template.New("analyzeCommand").Parse(cmd.Command))
					buf := new(bytes.Buffer)
					err = tmpl.Execute(buf, struct {
						Duration      int
						Frequency     int
						AnalyzeSystem bool
						AnalyzeJava   bool
					}{
						Duration:      cmdLineArgs.analyzeDuration,
						Frequency:     cmdLineArgs.analyzeFrequency,
						AnalyzeSystem: strings.Contains(cmdLineArgs.analyze, "system") || strings.Contains(cmdLineArgs.analyze, "all"),
						AnalyzeJava:   strings.Contains(cmdLineArgs.analyze, "java") || strings.Contains(cmdLineArgs.analyze, "all"),
					})
					if err != nil {
						return
					}
					cmd.Command = buf.String()
				}
			}
		}
	}
	customized, err = yaml.Marshal(cf)
	return
}

func (c *Collection) customizeCommandFile(cmdTemplate []byte, targetFilePath string, targetBinDir string) (err error) {
	return customizeCmdFile(cmdTemplate, targetFilePath, targetBinDir, c.target.GetName(), c.cmdLineArgs)
}

func customizeCmdFile(cmdTemplate []byte, targetFilePath string, targetBinDir string, targetHostName string, cmdLineArgs *CmdLineArgs) (err error) {
	customized, err := customizeCommandYAML(cmdTemplate, cmdLineArgs, targetBinDir, targetHostName)
	if err != nil {
		return
	}
	err = os.WriteFile(targetFilePath, customized, 0644)
	return
}

func (c *Collection) getDepsFile() (depsFile string, err error) {
	arch, err := c.target.GetArchitecture()
	if err != nil {
		return
	}
	switch arch {
	case "x86_64", "amd64":
		depsFile = filepath.Join(c.tempDir, "collector_deps_amd64.tgz")
	case "aarch64", "arm64":
		depsFile = filepath.Join(c.tempDir, "collector_deps_arm64.tgz")
	}
	if depsFile == "" {
		err = fmt.Errorf("unsupported architecture: '%s'", arch)
	}
	return
}

func (c *Collection) getCollectorFile() (collectorFile string, err error) {
	arch, err := c.target.GetArchitecture()
	if err != nil {
		return
	}
	switch arch {
	case "x86_64", "amd64":
		collectorFile = filepath.Join(c.tempDir, "collector")
	case "aarch64", "arm64":
		collectorFile = filepath.Join(c.tempDir, "collector_arm64")
	}
	if collectorFile == "" {
		err = errors.New("unsupported architecture: " + "'" + arch + "'")
	}
	return
}

func (c *Collection) extractArchive(filename string, tempDir string) (err error) {
	cmd := exec.Command("tar", "-C", tempDir, "-xf", filename)
	_, _, _, err = c.target.RunCommand(cmd)
	return
}

func (c *Collection) cleanupTarget(tempDir string) {
	if !c.cmdLineArgs.debug {
		err := c.target.RemoveDirectory(tempDir)
		if err != nil {
			log.Printf("failed to remove temporary directory for %s", c.target.GetName())
		}
	}
}

func hasPreReqs(t target.Target, preReqs []string) bool {
	for _, pr := range preReqs {
		cmd := exec.Command("which", pr)
		_, _, _, err := t.RunCommand(cmd)
		if err != nil {
			return false
		}
	}
	return true
}

func (c *Collection) getCollectorOutputFile(workingDirectory string) (outputFilePath string, err error) {
	outputFilePath = filepath.Join(c.outputDir, c.target.GetName()+".raw.json")
	err = c.target.PullFile(filepath.Join(workingDirectory, "collector.stdout"), outputFilePath)
	return
}

func getExtrasDir() (dir string, err error) {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	dir = filepath.Join(filepath.Dir(exePath), "extras")
	return
}

func (c *Collection) getExtrasFiles() (extras []string, err error) {
	extrasDir, err := getExtrasDir()
	if err != nil {
		log.Printf("Failed to get path to extra files: %v", err)
		return
	}
	var exists bool
	exists, err = util.DirectoryExists(extrasDir)
	if err != nil {
		err = fmt.Errorf("failed to determine if extras directory exists: %v", err)
		return
	}
	if !exists {
		log.Printf("Extra collection files dir (%s) not found.", extrasDir)
		return
	}
	dir, err := os.Open(extrasDir)
	if err != nil {
		log.Printf("Failed to open the directory (%s) of extra files: %v", extrasDir, err)
		return
	}
	defer dir.Close()
	files, err := dir.ReadDir(-1)
	if err != nil {
		log.Printf("Failed to read filenames from directory (%s) of extra files: %v", extrasDir, err)
		return
	}
	for _, f := range files {
		if f.Type().IsRegular() {
			extras = append(extras, filepath.Join(extrasDir, f.Name()))
		}
	}
	return
}

func (c *Collection) runCollector(collectorFilePath string, yamlFilePath string, workingDirectory string) (stdout string, stderr string, err error) {
	var cmd *exec.Cmd
	bashCmd := fmt.Sprintf("%s %s > collector.stdout", collectorFilePath, yamlFilePath)
	tType := fmt.Sprintf("%T", c.target)
	if tType == "*target.LocalTarget" {
		cmd = exec.Command("bash", "-c", bashCmd)
		if c.target.GetSudo() != "" {
			cmd.Env = append(os.Environ(), "SUDO_PASSWORD="+c.target.GetSudo())
		}
		cmd.Dir = workingDirectory
	} else { // RemoteTarget
		if c.target.GetSudo() != "" {
			cmd = exec.Command(fmt.Sprintf("cd %s && SUDO_PASSWORD=%s %s", workingDirectory, c.target.GetSudo(), bashCmd))
		} else {
			cmd = exec.Command(fmt.Sprintf("cd %s && %s", workingDirectory, bashCmd))
		}
	}
	stdout, stderr, _, err = c.target.RunCommand(cmd)
	return
}

func (c *Collection) Collect() (err error) {
	log.Printf("collection starting for target: %s", c.target.GetName())
	if !c.target.CanConnect() {
		err = fmt.Errorf("failed to connect to target: %s", c.target.GetName())
		log.Print(err)
		return
	}
	if !hasPreReqs(c.target, []string{"tar"}) {
		err = fmt.Errorf("tar not found on target: %s", c.target.GetName())
		log.Print(err)
		return
	}

	if (strings.Contains(c.cmdLineArgs.analyze, "system") || strings.Contains(c.cmdLineArgs.analyze, "all")) &&
		!hasPreReqs(c.target, []string{"perl"}) {
		log.Printf("perl not found on target: %s. Analyze system requires perl to process data.", c.target.GetName())
	}

	tempDir, err := c.target.CreateTempDirectory(c.cmdLineArgs.targetTemp)
	if err != nil {
		log.Printf("failed to create temporary directory for %s", c.target.GetName())
		return
	}
	defer c.cleanupTarget(tempDir)
	cmdTemplate, err := resources.ReadFile("resources/collector_reports.yaml.tmpl")
	if err != nil {
		return
	}
	commandFilePath := c.getCommandFilePath("_reports")
	err = c.customizeCommandFile(cmdTemplate, commandFilePath, tempDir)
	if err != nil {
		log.Print("failed to customize command file path")
		return
	}
	var depsFilename string
	depsFilename, err = c.getDepsFile()
	if err != nil || depsFilename == "" {
		log.Printf("failed to get dependencies file for %s", c.target.GetName())
		return
	}
	err = c.target.PushFile(depsFilename, tempDir)
	if err != nil {
		log.Printf("failed to push dependencies file to temporary directory for %s", c.target.GetName())
		return
	}
	err = c.extractArchive(filepath.Join(tempDir, filepath.Base(depsFilename)), tempDir)
	if err != nil {
		log.Printf("failed to extract dependencies file in temporary directory for %s", c.target.GetName())
		return
	}
	var collectorFilename string
	collectorFilename, err = c.getCollectorFile()
	if err != nil {
		log.Printf("failed to get collector file for %s", c.target.GetName())
		return
	}
	err = c.target.PushFile(collectorFilename, filepath.Join(tempDir, "collector"))
	if err != nil {
		log.Printf("failed to push collector to temporary directory for %s", c.target.GetName())
		return
	}
	err = c.target.PushFile(commandFilePath, tempDir)
	if err != nil {
		log.Printf("failed to push command file to temporary directory for %s", c.target.GetName())
		return
	}
	extrasDir, err := getExtrasDir()
	if err != nil {
		log.Printf("Failed to get extras dir: %v", err)
		return
	}
	var exists bool
	exists, err = util.DirectoryExists(extrasDir)
	if err != nil {
		err = fmt.Errorf("failed to determine if extras directory exists: %v", err)
		return
	}
	if exists {
		var extraFilenames []string
		extraFilenames, err = c.getExtrasFiles()
		if err != nil {
			log.Printf("failed to get extra file names: %v", err)
			return
		}
		for _, extraFile := range extraFilenames {
			err = c.target.PushFile(extraFile, tempDir)
			if err != nil {
				log.Printf("failed to push extra file %s to target at %s", extraFile, tempDir)
				return
			}
		}
	} else {
		log.Printf("Optional directory of extra collection files (%s) not found.", extrasDir)
	}
	c.stdout, c.stderr, err = c.runCollector(
		filepath.Join(tempDir, "collector"),
		filepath.Join(tempDir, filepath.Base(commandFilePath)),
		tempDir,
	)
	if err != nil {
		log.Printf("failed to run collector on %s, stderr: [%s]. "+
			"Override the temporary directory used by svr-info with the "+
			"--targettemp option if the target's temporary directory does "+
			"not support binary execution.",
			c.target.GetName(), c.stderr)
		return
	}
	c.outputFilePath, err = c.getCollectorOutputFile(tempDir)
	if err != nil {
		log.Printf("failed to retrieve collector output file for %s", c.target.GetName())
		return
	}
	if c.cmdLineArgs.megadata {
		var cmdTemplate []byte
		cmdTemplate, err = resources.ReadFile("resources/collector_megadata.yaml.tmpl")
		if err != nil {
			return
		}
		commandFilePath := c.getCommandFilePath("_megadata")
		err = c.customizeCommandFile(cmdTemplate, commandFilePath, tempDir)
		if err != nil {
			log.Print("failed to customize command file path")
			return
		}
		err = c.target.PushFile(commandFilePath, tempDir)
		if err != nil {
			log.Printf("failed to push megadata command file to temporary directory for %s", c.target.GetName())
			return
		}
		megaDir := c.target.GetName() + "_" + "megadata"
		var megaPath string
		megaPath, err = c.target.CreateDirectory(tempDir, megaDir)
		if err != nil {
			log.Printf("failed to create megadata directory on %s", c.target.GetName())
			return
		}
		// run collector in the megadata directory so output from commands will land in that directory
		_, _, err = c.runCollector(
			filepath.Join(tempDir, "collector"),
			filepath.Join(tempDir, filepath.Base(commandFilePath)),
			megaPath,
		)
		if err != nil {
			log.Printf("failed to run megadata collector on %s, stderr: [%s]",
				c.target.GetName(), c.stderr)
			return
		}
		megadataTarball := filepath.Join(tempDir, c.target.GetName()+"_megadata.tgz")
		cmd := exec.Command("tar", "-C", tempDir, "-czf", megadataTarball, megaDir)
		_, _, _, err = c.target.RunCommand(cmd)
		if err != nil {
			log.Printf("failed to create megadata tarball")
			return
		}
		err = c.target.PullFile(megadataTarball, c.outputDir)
		if err != nil {
			log.Printf("failed to retrieve megadata tarball")
			return
		}
		err = c.target.PullFile(filepath.Join(tempDir, megaDir, "collector.log"), filepath.Join(c.outputDir, c.target.GetName()+"_megadata_collector.log"))
		if err != nil {
			log.Printf("failed to retrieve megadata collector.log")
			return
		}
		cmd = exec.Command("tar", "-C", c.outputDir, "-xf", filepath.Join(c.outputDir, c.target.GetName()+"_megadata.tgz"))
		_, _, _, err = target.RunLocalCommand(cmd)
		if err != nil {
			log.Printf("failed to extract megadata tarball")
			return
		}
		cmd = exec.Command("rm", filepath.Join(c.outputDir, c.target.GetName()+"_megadata.tgz"))
		_, _, _, err = target.RunLocalCommand(cmd)
		if err != nil {
			log.Printf("failed to remove megadata tarball")
			return
		}
	}
	err = c.target.PullFile(filepath.Join(tempDir, "collector.log"), filepath.Join(c.outputDir, c.target.GetName()+"_collector.log"))
	if err != nil {
		log.Printf("failed to retrieve collector.log")
		return
	}
	c.ok = true
	return
}
