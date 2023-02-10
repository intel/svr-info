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

type ReportGeneratorJSONSimplified struct {
	reports   []*Report
	outputDir string
}

func newReportGeneratorJSONSimplified(outputDir string, configurationReport *Report, briefReport *Report, insightReport *Report, profileReport *Report, benchmarkReport *Report, analyzeReport *Report) (rpt *ReportGeneratorJSONSimplified) {
	rpt = &ReportGeneratorJSONSimplified{
		reports:   []*Report{configurationReport, briefReport, insightReport, profileReport, benchmarkReport, analyzeReport},
		outputDir: outputDir,
	}
	return
}

type SimpleRow map[string]string         //valuename->value
type SimpleTable map[string][]SimpleRow  //tablename->[]rows
type SimpleReport map[string]SimpleTable //reportname->tables
type SimpleHosts map[string]SimpleReport //hostname->reports

func convertToSimple(hostNames []string, reportsData []*Report) (simpleHosts SimpleHosts, err error) {
	simpleHosts = make(SimpleHosts)
	for hostIndex, hostName := range hostNames {
		simpleReport := make(SimpleReport)
		for _, report := range reportsData {
			simpleTable := make(SimpleTable)
			for _, table := range report.Tables {
				hostValues := table.AllHostValues[hostIndex]
				for _, values := range hostValues.Values {
					simpleRow := make(SimpleRow)
					for valueIndex, value := range values {
						simpleRow[hostValues.ValueNames[valueIndex]] = value
					}
					simpleTable[table.Name] = append(simpleTable[table.Name], simpleRow)
				}
			}
			simpleReport[report.InternalName] = simpleTable
		}
		simpleHosts[hostName] = simpleReport
	}
	return
}

func (r *ReportGeneratorJSONSimplified) generate() (reportFilePaths []string, err error) {
	var hostnames []string
	for _, values := range r.reports[0].Tables[0].AllHostValues {
		hostnames = append(hostnames, values.Name)
	}
	allHosts, err := convertToSimple(hostnames, r.reports)
	if err != nil {
		return
	}
	// one json report per host
	for hostName, host := range allHosts {
		fileName := hostName + ".json"
		reportFilePath := filepath.Join(r.outputDir, fileName)
		var jsonData []byte
		jsonData, err = json.MarshalIndent(host, "", "  ")
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
		var jsonData []byte
		jsonData, err = json.MarshalIndent(allHosts, "", "  ")
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
