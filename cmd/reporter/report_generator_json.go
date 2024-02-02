/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ReportGeneratorJSON struct {
	reports   []*Report
	outputDir string
}

func newReportGeneratorJSON(outputDir string, configurationData *Report, insightReport *Report, profileReport *Report, benchmarkReport *Report, analyzeReport *Report) (rpt *ReportGeneratorJSON) {
	rpt = &ReportGeneratorJSON{
		reports:   []*Report{configurationData, insightReport, profileReport, benchmarkReport, analyzeReport},
		outputDir: outputDir,
	}
	return
}

func (r *ReportGeneratorJSON) generate() (reportFilePaths []string, err error) {
	var hostnames []string
	for _, values := range r.reports[0].Tables[0].AllHostValues {
		hostnames = append(hostnames, values.Name)
	}
	// one json report per host
	for hostIndex, hostname := range hostnames {
		fileName := hostname + ".json"
		reportFilePath := filepath.Join(r.outputDir, fileName)
		// build new report data with values only from the current source/host
		var genData []Table
		for _, reportData := range r.reports {
			for _, table := range reportData.Tables {
				var genTable Table
				genTable.Name = table.Name
				genTable.Category = table.Category
				if len(table.AllHostValues) > hostIndex {
					genTable.AllHostValues = []HostValues{table.AllHostValues[hostIndex]}
				}
				genData = append(genData, genTable)
			}
		}
		var jsonData []byte
		jsonData, err = json.MarshalIndent(genData, "", "  ")
		if err != nil {
			return
		}
		err = os.WriteFile(reportFilePath, jsonData, 0644)
		if err != nil {
			return
		}
		reportFilePaths = append(reportFilePaths, reportFilePath)
	}
	// combined, all-host json report, if more than one host
	if len(hostnames) > 1 {
		fileName := "all_hosts.json"
		reportFilePath := filepath.Join(r.outputDir, fileName)
		var genData []Table
		for _, reportData := range r.reports {
			for _, table := range reportData.Tables {
				genData = append(genData, *table)
			}
		}
		var jsonData []byte
		jsonData, err = json.MarshalIndent(genData, "", "  ")
		if err != nil {
			return
		}
		err = os.WriteFile(reportFilePath, jsonData, 0644)
		if err != nil {
			return
		}
		reportFilePaths = append(reportFilePaths, reportFilePath)
	}
	return
}
