/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
/* ReportGenerator is the interface required to be implemented by formatted reports, e.g. HTML, XLSX, etc. */

package main

type ReportGenerator interface {
	generate() (reportFilePath []string, err error)
}
