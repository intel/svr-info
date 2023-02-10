/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
)

type ReportGeneratorTXT struct {
	sources   []*Source
	outputDir string
}

func newReportGeneratorTXT(sources []*Source, outputDir string) (rpt *ReportGeneratorTXT) {
	rpt = &ReportGeneratorTXT{
		sources:   sources,
		outputDir: outputDir,
	}
	return
}

func (r *ReportGeneratorTXT) generate() (reportFilePaths []string, err error) {
	for _, source := range r.sources {
		fileName := source.getHostname() + ".txt"
		reportFilePath := filepath.Join(r.outputDir, fileName)
		f, err := os.OpenFile(reportFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			log.Printf("Failed to create/open file for writing: %s", reportFilePath)
			continue
		}
		defer f.Close()
		f.WriteString(fmt.Sprintf("Host: %s\n", source.getHostname()))
		var keys []string
		for key := range source.ParsedData {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			cmd := source.ParsedData[key]
			f.WriteString("\n----------------------------------\n")
			f.WriteString(fmt.Sprintf("label:     %s\n", key))
			f.WriteString(fmt.Sprintf("command:   %s\n", cmd.Command))
			f.WriteString(fmt.Sprintf("exit code: %s\n", cmd.ExitStatus))
			f.WriteString(fmt.Sprintf("stderr:    %s\n", cmd.Stderr))
			f.WriteString(fmt.Sprintf("stdout:    %s\n", cmd.Stdout))
		}
		reportFilePaths = append(reportFilePaths, reportFilePath)
	}
	return
}
