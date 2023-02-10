/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
/* Defines the context and functions used by the rules engine */

package main

import (
	"log"
	"regexp"
	"strconv"
	"strings"
)

// RulesEngineContext struct is used as context for rules engine, i.e. the rules
// can call the exported functions below and access any exported data in the
// struct (currently none)
type RulesEngineContext struct {
	insightTable *Table
	reportsData  []*Report
	sourceIdx    int
}

// GetValue returns a string value from a table
func (r *RulesEngineContext) GetValue(reportName string, tableName string, valueName string) (value string) {
	var reportData *Report
	for _, rd := range r.reportsData {
		if rd.InternalName == reportName {
			reportData = rd
			break
		}
	}
	if reportData == nil {
		log.Printf("report specified in rule not found: %s", reportName)
		return
	}
	table := reportData.findTable(tableName)
	if table == nil {
		log.Printf("table specified in rule not found: %s", tableName)
		return
	}
	value, err := table.getValue(r.sourceIdx, valueName)
	if err != nil {
		log.Printf("failed to get value from table, %s:%s, %v", tableName, valueName, err)
	}
	return
}

func (r *RulesEngineContext) GetValueFromColumn(reportName, tableName, rowValueName, rowValue, targetValueName string) (value string) {
	var reportData *Report
	for _, rd := range r.reportsData {
		if rd.InternalName == reportName {
			reportData = rd
			break
		}
	}
	if reportData == nil {
		log.Printf("report specified in rule not found: %s", reportName)
		return
	}
	table := reportData.findTable(tableName)
	if table == nil {
		log.Printf("table specified in rule not found: %s", tableName)
		return
	}
	hv := &table.AllHostValues[r.sourceIdx]
	rowValueIndex, err := findValueIndex(hv, rowValueName)
	if err != nil {
		log.Printf("%v", err)
	}
	targetValueIndex, err := findValueIndex(hv, targetValueName)
	if err != nil {
		log.Printf("%v", err)
	}
	for _, values := range hv.Values {
		if values[rowValueIndex] == rowValue {
			value = values[targetValueIndex]
			break
		}
	}
	return
}

// GetValuesFromColumn returns all values in specified valueIndex as a string (comma separated list)
func (r *RulesEngineContext) GetValuesFromColumn(reportName string, tableName string, valueIndex int64) (values string) {
	var reportData *Report
	for _, rd := range r.reportsData {
		if rd.InternalName == reportName {
			reportData = rd
			break
		}
	}
	if reportData == nil {
		log.Printf("report specified in rule not found: %s", reportName)
		return
	}
	table := reportData.findTable(tableName)
	if table == nil {
		log.Printf("table specified in rule not found: %s", tableName)
		return
	}
	hv := &table.AllHostValues[r.sourceIdx]
	if int64(len(hv.Values)) > valueIndex {
		values = strings.Join(hv.Values[0], ",")
	}
	return
}

// GetValueAsInt returns an integer value from a table
func (r *RulesEngineContext) GetValueAsInt(reportName string, tableName string, valueName string) (value int) {
	v := r.GetValue(reportName, tableName, valueName)
	re := regexp.MustCompile(`.*?(\d*)`)
	match := re.FindStringSubmatch(v)
	var num string
	if match != nil {
		num = match[1]
	}
	value, err := strconv.Atoi(num)
	if err != nil {
		log.Printf("failed to convert string to int: %s", v)
	}
	return
}

// GetValueAsFloat returns a float64 value from a table
// if value doesn't contain a float, result will be 0
func (r *RulesEngineContext) GetValueAsFloat(reportName string, tableName string, valueName string) (value float64) {
	v := r.GetValue(reportName, tableName, valueName)
	if v == "" {
		return
	}
	re := regexp.MustCompile(`.*?(\d*\.\d*).*`)
	match := re.FindStringSubmatch(v)
	var num string
	if match != nil {
		num = match[1]
	}
	value, err := strconv.ParseFloat(num, 64)
	if err != nil {
		log.Printf("failed to convert string to float: %s", v)
	}
	return
}

// GetValueFromColumnAsFloat returns a float64 value from a table
// if column value doesn't contain a float, result will be 0
func (r *RulesEngineContext) GetValueFromColumnAsFloat(reportName, tableName, rowValueName, rowValue, targetValueName string) (value float64) {
	v := r.GetValueFromColumn(reportName, tableName, rowValueName, rowValue, targetValueName)
	re := regexp.MustCompile(`.*?(\d*\.\d*).*`)
	match := re.FindStringSubmatch(v)
	var num string
	if match != nil {
		num = match[1]
	}
	value, err := strconv.ParseFloat(num, 64)
	if err != nil {
		log.Printf("failed to convert string to float: %s", v)
	}
	return
}

// CompareVersions -- compares two version strings
// Note: both input versions need to be of the same format
// Supported formats:
// - single integer, ex. 10
// - two integers, ex. 10.7
// - three integers, ex. 10.7.33
// - three integers and a alpha character, ex. 1.1.1m  (OpenSSL version format)
// returns 0 if x == y, -1 if x < y, 1 if x > y....and -2 if error
func (r *RulesEngineContext) CompareVersions(x, y string) int {
	var res []*regexp.Regexp
	res = append(res, regexp.MustCompile(`([0-9]+)\.([0-9]+)\.([0-9]+)([a-z])`))
	res = append(res, regexp.MustCompile(`([0-9]+)\.([0-9]+)\.([0-9]+)`))
	res = append(res, regexp.MustCompile(`([0-9]+)\.([0-9]+)`))
	res = append(res, regexp.MustCompile(`([0-9]+)`))

	var xMatch, yMatch []string
	for _, re := range res {
		xMatch = re.FindStringSubmatch(x)
		yMatch = re.FindStringSubmatch(y)
		if len(xMatch) != len(yMatch) {
			return -2 // inconsistent format
		}
		if xMatch != nil {
			break // found a matching format
		}
	}
	if xMatch == nil {
		return -2 // unsupported format
	}
	for i := 1; i <= len(xMatch)-1; i++ {
		if i == 4 { // special case for openssl 1.1.1e style format
			if xMatch[i] < yMatch[i] {
				return -1
			} else if xMatch[i] > yMatch[i] {
				return 1
			}
			continue
		}
		xVal, _ := strconv.Atoi(xMatch[i])
		yVal, _ := strconv.Atoi(yMatch[i])
		if xVal < yVal {
			return -1
		} else if xVal > yVal {
			return 1
		}
	}
	return 0 // they are the same version
}

// CompareMicroarchitecture -- comparison of CPU micro-architectures
// returns 0 if x == y, -1 if x < y, 1 if x > y....and -2 if error
func (r *RulesEngineContext) CompareMicroarchitecture(x, y string) int {
	uArchs := map[string]int{
		"HSX": 1,
		"BDX": 2,
		"SKX": 3,
		"CLX": 4,
		"ICX": 5,
		"SPR": 6,
		"EMR": 7,
	}
	var xArch, yArch int
	var ok bool
	if xArch, ok = uArchs[x]; !ok {
		return -2
	}
	if yArch, ok = uArchs[y]; !ok {
		return -2
	}
	if xArch < yArch {
		return -1
	}
	if xArch > yArch {
		return 1
	}
	return 0 // equal
}

// AddInsight -- appends an insight to the table
func (r *RulesEngineContext) AddInsight(justification string, recommendation string) {
	r.insightTable.AllHostValues[r.sourceIdx].Values = append(
		r.insightTable.AllHostValues[r.sourceIdx].Values,
		[]string{recommendation, justification},
	)
}
