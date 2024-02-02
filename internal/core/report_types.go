/*
Package core includes internal shared code.
*/
/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package core

import (
	"fmt"
	"strings"
)

var ReportTypes = []string{"html", "json", "xlsx", "txt", "all"}

func IsValidReportType(input string) (valid bool) {
	for _, validType := range ReportTypes {
		if input == validType {
			return true
		}
	}
	return false
}

func GetReportTypes(input string) (reportTypes []string, err error) {
	reportTypes = strings.Split(input, ",")
	if len(reportTypes) == 1 && reportTypes[0] == "all" {
		reportTypes = ReportTypes[:len(ReportTypes)-1]
		return
	}
	for _, reportType := range reportTypes {
		if !IsValidReportType(reportType) {
			err = fmt.Errorf("invalid report type: %s", reportType)
			return
		}
	}
	return
}
