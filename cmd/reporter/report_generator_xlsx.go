/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

type ReportGeneratorXLSX struct {
	reports    []*Report
	sheetNames []string
	outputDir  string
}

func newReportGeneratorXLSX(outputDir string, configurationReport *Report, briefReport *Report, insightReport *Report, profileReport *Report, benchmarkReport *Report, analyzeReport *Report) (rpt *ReportGeneratorXLSX) {
	rpt = &ReportGeneratorXLSX{
		reports:    []*Report{configurationReport, briefReport, benchmarkReport, profileReport, analyzeReport, insightReport}, // this is the order the tabs will appear in the spreadsheet
		sheetNames: []string{"Configuration", "Brief", "Benchmark", "Profile", "Analyze", "Insights"},
		outputDir:  outputDir,
	}
	return
}

func cellName(col int, row int) (name string) {
	columnName, err := excelize.ColumnNumberToName(col)
	if err != nil {
		return
	}
	name, err = excelize.JoinCellName(columnName, row)
	if err != nil {
		return
	}
	return
}

func renderExcelTable(tableHeaders []string, tableValues [][]string, f *excelize.File, reportSheetName string, originRow int, originCol int, boldFirstCol bool) int {
	row := originRow
	col := originCol
	bold, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
		},
	})
	alignLeft, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{
			Horizontal: "left",
		},
	})
	boldAlignLeft, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
		},
	})
	if len(tableValues) > 0 {
		if len(tableHeaders) > 0 {
			for _, header := range tableHeaders {
				// if possible, convert strings to floats before inserting into the sheet
				floatValue, err := strconv.ParseFloat(header, 64)
				if err == nil {
					f.SetCellFloat(reportSheetName, cellName(col, row), floatValue, 1, 64)
					f.SetCellStyle(reportSheetName, cellName(col, row), cellName(col, row), boldAlignLeft)
				} else {

					f.SetCellStr(reportSheetName, cellName(col, row), header)
					f.SetCellStyle(reportSheetName, cellName(col, row), cellName(col, row), bold)
				}
				col += 1
			}
			row += 1
		}
		for _, rowValues := range tableValues {
			col = originCol
			if len(rowValues) > 0 {
				for rowIdx, value := range rowValues {
					// if possible, convert strings to floats before inserting into the sheet
					floatValue, err := strconv.ParseFloat(value, 64)
					if err == nil {
						f.SetCellFloat(reportSheetName, cellName(col, row), floatValue, 1, 64)
						f.SetCellStyle(reportSheetName, cellName(col, row), cellName(col, row), alignLeft)
					} else {
						if rowIdx == 0 && boldFirstCol {
							f.SetCellStyle(reportSheetName, cellName(col, row), cellName(col, row), bold)
						}
						f.SetCellStr(reportSheetName, cellName(col, row), value)
					}
					col += 1
				}
			} else {
				f.SetCellStr(reportSheetName, cellName(col, row), "")
			}
			row += 1
		}
	} else {
		f.SetCellStr(reportSheetName, cellName(col, row), "No data found.")
		row += 1
	}
	return row
}

func (r *ReportGeneratorXLSX) renderSingleValueTable(table *Table, allHostValues []HostValues, f *excelize.File, reportSheetName string, row int, col int, noHeader bool) int {
	var tableHeaders []string
	var tableValues [][]string

	if len(allHostValues) > 1 && !noHeader {
		tableHeaders = append(tableHeaders, "")
		for _, hv := range allHostValues {
			tableHeaders = append(tableHeaders, hv.Name)
		}
	}
	// a host with no values will not have value names, so find a host with value names
	var valueNames []string
	for _, hv := range allHostValues {
		if len(hv.ValueNames) > 0 {
			valueNames = hv.ValueNames
			break
		}
	}
	for valueIndex, valueName := range valueNames {
		var rowValues []string
		rowValues = append(rowValues, valueName)
		for _, hv := range allHostValues {
			if len(hv.Values) > 0 && len(hv.Values[0]) > valueIndex {
				rowValues = append(rowValues, hv.Values[0][valueIndex])
			} else {
				rowValues = append(rowValues, "")
			}
		}
		tableValues = append(tableValues, rowValues)
	}
	// if all data fields are empty string, then don't render the table
	haveData := false
	for _, rowValues := range tableValues {
		for col, val := range rowValues {
			if val != "" && col != 0 {
				haveData = true
				break
			}
		}
		if haveData {
			break
		}
	}
	if !haveData {
		tableValues = [][]string{} // this will cause renderExcelTable to indicate "No data found."
	}
	return renderExcelTable(tableHeaders, tableValues, f, reportSheetName, row, col, true)
}

func (r *ReportGeneratorXLSX) renderMultiValueTable(table *Table, allHostValues []HostValues, f *excelize.File, reportSheetName string, row int, col int) int {
	// render one Excel table per host
	for idx, hv := range allHostValues {
		// if more than one host, put hostname above table
		if len(allHostValues) > 1 {
			f.SetCellStr(reportSheetName, cellName(2, row), hv.Name)
			headerStyle, _ := f.NewStyle(&excelize.Style{
				Font: &excelize.Font{
					Bold: true,
				},
			})
			f.SetCellStyle(reportSheetName, cellName(2, row), cellName(2, row), headerStyle)
			row += 1
		}
		row = renderExcelTable(hv.ValueNames, hv.Values, f, reportSheetName, row, col, false)
		if idx < len(allHostValues)-1 {
			row += 1
		}
	}
	return row
}

func (r *ReportGeneratorXLSX) renderNumaBandwidthTable(table *Table, allHostValues []HostValues, f *excelize.File, reportSheetName string, row int) int {
	// render one Excel table per host
	for idx, hv := range allHostValues {
		// if more than one host, put hostname above table
		if len(allHostValues) > 1 {
			f.SetCellStr(reportSheetName, cellName(2, row), hv.Name)
			headerStyle, _ := f.NewStyle(&excelize.Style{
				Font: &excelize.Font{
					Bold: true,
				},
			})
			f.SetCellStyle(reportSheetName, cellName(2, row), cellName(2, row), headerStyle)
			row += 1
		}
		var tableHeaders []string
		var tableValues [][]string
		tableHeaders = append(tableHeaders, "Node")
		for nodeIdx, node := range hv.Values {
			tableHeaders = append(tableHeaders, fmt.Sprintf("%d", nodeIdx))
			rowValues := []string{node[0]}
			bandwidths := strings.Split(node[1], ",")
			rowValues = append(rowValues, bandwidths...)
			tableValues = append(tableValues, rowValues)
		}
		row = renderExcelTable(tableHeaders, tableValues, f, reportSheetName, row, 2, true)
		if idx < len(allHostValues)-1 {
			row += 1
		}
	}
	return row
}

func (r *ReportGeneratorXLSX) renderDIMMPopulationTable(table *Table, allHostValues []HostValues, f *excelize.File, reportSheetName string, row int) int {
	// render one Excel table per host
	for idx, hv := range allHostValues {
		// if more than one host, put hostname above table
		if len(allHostValues) > 1 {
			f.SetCellStr(reportSheetName, cellName(2, row), hv.Name)
			headerStyle, _ := f.NewStyle(&excelize.Style{
				Font: &excelize.Font{
					Bold: true,
				},
			})
			f.SetCellStyle(reportSheetName, cellName(2, row), cellName(2, row), headerStyle)
			row += 1
		}
		var tableHeaders = []string{"Socket", "Channel", "Slot", "Details"}
		var tableValues [][]string
		for _, dimm := range hv.Values {
			tableValues = append(tableValues, []string{dimm[DerivedSocketIdx], dimm[DerivedChannelIdx], dimm[DerivedSlotIdx], dimmDetails(dimm)})
		}
		row = renderExcelTable(tableHeaders, tableValues, f, reportSheetName, row, 2, false)
		if idx < len(allHostValues)-1 {
			row += 1
		}
	}
	return row
}

func (r *ReportGeneratorXLSX) fillSheet(f *excelize.File, reportSheetName string, reportData *Report, sourceIndex int, briefReport bool) (err error) {
	combinedReport := sourceIndex < 0
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
		},
	})
	if briefReport { // wider first column for brief report
		f.SetColWidth(reportSheetName, "A", "A", 25)
	} else {
		f.SetColWidth(reportSheetName, "A", "A", 15)
	}
	f.SetColWidth(reportSheetName, "B", "L", 25)
	row, col := 1, 1
	for tableIdx, table := range reportData.Tables {
		if table == nil {
			continue
		}
		var allHostValues []HostValues
		if combinedReport {
			allHostValues = table.AllHostValues
		} else {
			allHostValues = []HostValues{table.AllHostValues[sourceIndex]}
		}
		col = 1
		if !briefReport { // no table names in brief report
			f.SetCellStr(reportSheetName, cellName(col, row), table.Name)
			f.SetCellStyle(reportSheetName, cellName(col, row), cellName(col, row), headerStyle)
			col++
		}

		if table.Name == "Memory NUMA Bandwidth" {
			row = r.renderNumaBandwidthTable(table, allHostValues, f, reportSheetName, row)
		} else if table.Name == "DIMM Population" {
			row = r.renderDIMMPopulationTable(table, allHostValues, f, reportSheetName, row)
		} else if isSingleValueTable(table) {
			noHeader := briefReport && tableIdx != 0
			row = r.renderSingleValueTable(table, allHostValues, f, reportSheetName, row, col, noHeader)
		} else {
			row = r.renderMultiValueTable(table, allHostValues, f, reportSheetName, row, col)
		}
		if !briefReport { //no row between tables in brief report
			row += 1
		}
	}
	if briefReport {
		row += 1
	}
	return
}

// one Excel report for each host in reportData and a combined report if more than one host
// Note: an Excel report includes a full report, a brief report, a benchmark report, a profile report, an analyze reportk, and a insights report
func (r *ReportGeneratorXLSX) generate() (reportFilePaths []string, err error) {
	var hostnames []string
	for _, values := range r.reports[0].Tables[0].AllHostValues {
		hostnames = append(hostnames, values.Name)
	}
	// generate one excel file for every host
	for hostIndex, hostname := range hostnames {
		fileName := hostname + ".xlsx"
		reportFilePath := filepath.Join(r.outputDir, fileName)
		f := excelize.NewFile()
		for reportIndex, reportData := range r.reports {
			if reportIndex == 0 {
				f.SetSheetName("Sheet1", r.sheetNames[reportIndex])
			} else {
				f.NewSheet(r.sheetNames[reportIndex])
			}
			r.fillSheet(f, r.sheetNames[reportIndex], reportData, hostIndex, reportIndex == 1)
		}
		var outFile *os.File
		outFile, err = os.OpenFile(reportFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return
		}
		_, err = f.WriteTo(outFile)
		outFile.Close()
		if err != nil {
			return
		}
		reportFilePaths = append(reportFilePaths, reportFilePath)
	}
	// if more than one host create a combined report
	if len(r.reports[0].Sources) > 1 {
		fileName := "all_hosts.xlsx"
		reportFilePath := filepath.Join(r.outputDir, fileName)
		f := excelize.NewFile()
		for reportIndex, reportData := range r.reports {
			if reportIndex == 0 {
				f.SetSheetName("Sheet1", r.sheetNames[reportIndex])
			} else {
				f.NewSheet(r.sheetNames[reportIndex])
			}
			r.fillSheet(f, r.sheetNames[reportIndex], reportData, -1, reportIndex == 1) // -1 means all sources
		}
		var outFile *os.File
		outFile, err = os.OpenFile(reportFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return
		}
		_, err = f.WriteTo(outFile)
		outFile.Close()
		if err != nil {
			return
		}
		reportFilePaths = append(reportFilePaths, reportFilePath)
	}
	return
}
