/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
/* Defines the reports (e.g., Full, Brief, etc.) */

package main

import (
	"log"

	"github.com/intel/svr-info/internal/cpudb"
)

// Report ... all sources & tables that define a report
type Report struct {
	InternalName string // the value set here needs to remain consistent for users who parse the json report
	Sources      []*Source
	Tables       []*Table
}

// NewConfigurationReport -- includes all verbose tables
func NewConfigurationReport(sources []*Source, CPUdb cpudb.CPUDB) (report *Report) {
	report = &Report{
		InternalName: "Configuration",
		Sources:      sources,
		Tables:       []*Table{},
	}

	report.Tables = append(report.Tables,
		[]*Table{
			newHostTable(sources, System),
			newSystemTable(sources, System),
			newBaseboardTable(sources, System),
			newChassisTable(sources, System),
			newPCIeSlotsTable(sources, System),

			newBIOSTable(sources, Software),
			newOperatingSystemTable(sources, Software),
			newSoftwareTable(sources, Software),

			newCPUTable(sources, CPUdb, CPUCategory),
			newISATable(sources, CPUCategory),
			newAcceleratorTable(sources, CPUCategory),

			newPowerTable(sources, Power),
			newUncoreTable(sources, CPUdb, Power),
			newEfficiencyLatencyControlTable(sources, Power),
		}...,
	)

	tableDIMM := newDIMMTable(sources, Memory)
	tableDIMMPopulation := newDIMMPopulationTable(sources, tableDIMM, CPUdb, Memory)

	report.Tables = append(report.Tables,
		[]*Table{
			newMemoryTable(sources, tableDIMM, tableDIMMPopulation, Memory),
			tableDIMMPopulation,
			tableDIMM,

			newNICTable(sources, Network),
			newNetworkIRQTable(sources, Network),

			newDiskTable(sources, Storage),
			newFilesystemTable(sources, Storage),

			newGPUTable(sources, GPU),
			newGaudiTable(sources, GPU),

			newCXLDeviceTable(sources, CXL),

			newVulnerabilityTable(sources, Security),

			newProcessTable(sources, Status),
			newSensorTable(sources, Status),
			newChassisStatusTable(sources, Status),
			newSystemEventLogTable(sources, Status),
			newKernelLogTable(sources, Status),
			newPMUTable(sources, Status),
			newSvrinfoTable(sources, Status),
		}...,
	)
	// TODO: remove check when code is stable
	for _, table := range report.Tables {
		check(table, sources)
	}
	return
}

func NewBriefReport(sources []*Source, fullReport *Report, CPUdb cpudb.CPUDB) (report *Report) {
	report = &Report{
		InternalName: "Brief",
		Sources:      sources,
		Tables:       []*Table{},
	}
	tableDiskSummary := newDiskSummaryTable(fullReport.findTable("Disk"), Storage)
	tableNicSummary := newNICSummaryTable(fullReport.findTable("NIC"), Network)
	tableAcceleratorSummary := newAcceleratorSummaryTable(fullReport.findTable("Accelerator"), CPUCategory)
	tableEfficiencyLatencyControlSummary := newEfficiencyLatencyControlSummaryTable(fullReport.findTable("Efficiency Latency Control"), Power)
	report.Tables = append(report.Tables,
		[]*Table{
			fullReport.findTable("Host"),
			newSystemSummaryTable(fullReport.findTable("System"), System),
			newBaseboardSummaryTable(fullReport.findTable("Baseboard"), System),
			newChassisSummaryTable(fullReport.findTable("Chassis"), System),
			newCPUBriefTable(fullReport.findTable("CPU"), CPUCategory),
			tableAcceleratorSummary,
			newMemoryBriefTable(fullReport.findTable("Memory"), Memory),
			tableNicSummary,
			tableDiskSummary,
			newBIOSSummaryTable(fullReport.findTable("BIOS"), Software),
			newOperatingSystemBriefTable(fullReport.findTable("Operating System"), Software),
			fullReport.findTable("Power"),
			tableEfficiencyLatencyControlSummary,
			newVulnerabilitySummaryTable(fullReport.findTable("Vulnerability"), Security),
			newMarketingClaimTable(fullReport, tableNicSummary, tableDiskSummary, NoCategory),
		}...,
	)
	// TODO: remove check when code is stable
	for _, table := range report.Tables {
		check(table, sources)
	}
	return
}

func NewInsightsReport(sources []*Source, configReport, briefReport, profileReport, benchmarkReport *Report, analyzeReport *Report, CPUdb cpudb.CPUDB) (report *Report) {
	report = &Report{
		InternalName: "Recommendations",
		Sources:      sources,
		Tables:       []*Table{},
	}
	report.Tables = append(report.Tables,
		[]*Table{
			newInsightTable(configReport, briefReport, profileReport, benchmarkReport, analyzeReport),
		}...,
	)
	// TODO: remove check when code is stable
	for _, table := range report.Tables {
		check(table, sources)
	}
	return
}

func NewProfileReport(sources []*Source) (report *Report) {
	report = &Report{
		InternalName: "Profile",
		Sources:      sources,
		Tables:       []*Table{},
	}
	averageCPUUtilizationTable := newAverageCPUUtilizationTable(sources, NoCategory)
	CPUUtilizationTable := newCPUUtilizationTable(sources, NoCategory)
	IRQRateTable := newIRQRateTable(sources, NoCategory)
	driveStatsTable := newDriveStatsTable(sources, NoCategory)
	netStatsTable := newNetworkStatsTable(sources, NoCategory)
	memStatsTable := newMemoryStatsTable(sources, NoCategory)
	PMUMetricsTable := newPMUMetricsTable(sources, NoCategory)
	powerStatsTable := newPowerStatsTable(sources, NoCategory)
	summaryTable := newProfileSummaryTable(sources, NoCategory, averageCPUUtilizationTable, driveStatsTable, netStatsTable, memStatsTable, PMUMetricsTable, powerStatsTable)
	report.Tables = append(report.Tables,
		[]*Table{
			summaryTable,
			averageCPUUtilizationTable,
			CPUUtilizationTable,
			powerStatsTable,
			IRQRateTable,
			driveStatsTable,
			netStatsTable,
			memStatsTable,
			PMUMetricsTable,
		}...,
	)
	// TODO: remove check when code is stable
	for _, table := range report.Tables {
		check(table, sources)
	}
	return
}

func NewAnalyzeReport(sources []*Source) (report *Report) {
	report = &Report{
		InternalName: "Analyze",
		Sources:      sources,
		Tables:       []*Table{},
	}
	report.Tables = append(report.Tables,
		[]*Table{
			newCodePathTable(sources, NoCategory),
		}...,
	)
	// TODO: remove check when code is stable
	for _, table := range report.Tables {
		check(table, sources)
	}
	return
}

func NewBenchmarkReport(sources []*Source, CPUdb cpudb.CPUDB) (report *Report) {
	report = &Report{
		InternalName: "Performance",
		Sources:      sources,
		Tables:       []*Table{},
	}
	tableMemBandwidthLatency := newMemoryBandwidthLatencyTable(sources, NoCategory)
	report.Tables = append(report.Tables,
		[]*Table{
			newBenchmarkSummaryTable(sources, tableMemBandwidthLatency, NoCategory),
			newFrequencyTable(sources, CPUdb, NoCategory),
			tableMemBandwidthLatency,
			newMemoryNUMABandwidthTable(sources, NoCategory),
		}...,
	)
	// TODO: remove check when code is stable
	for _, table := range report.Tables {
		check(table, sources)
	}
	return
}

/*
A function that creates and returns a table must return a valid table.
A valid table is defined as follows:
  - Table.Name is set to a non-empty string
  - Table.AllHostValues length is equal to number of Source
  - HostValues.HostName is set to a non-empty string
  - HostValues.Values[] lengths are equal to the number of HostValues.ValueNames or zero
*/
func check(table *Table, sources []*Source) {
	if table.Name == "" {
		log.Panic("table name not set")
	}
	if len(table.AllHostValues) != len(sources) {
		log.Panic("len of host values != len sources: " + table.Name)
	}
	for _, hv := range table.AllHostValues {
		if hv.Name == "" {
			log.Panic("host name not set: " + table.Name)
		}
		for _, record := range hv.Values {
			if len(record) != len(hv.ValueNames) && len(record) != 0 {
				log.Panic("# of values doesn't match # of value names: " + table.Name)
			}
		}
	}
}

func (r *Report) findTable(name string) (table *Table) {
	for _, t := range r.Tables {
		if t.Name == name {
			table = t
			return
		}
	}
	return
}
