/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
//
// functions to create summary (mean,min,max,stddev) metrics from metrics CSV
//
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/intel/svr-info/internal/util"
)

// PostProcess - generates formatted output from a CSV file containing metric values. Format
// options are 'html' and 'csv'.
func PostProcess(csvInputPath string, format Summary) (out string, err error) {
	var metrics []metricsFromCSV
	if metrics, err = newMetricsFromCSV(csvInputPath); err != nil {
		return
	}
	if format == SummaryHTML {
		if len(metrics) > 1 {
			err = fmt.Errorf("html format supported only for a single set of data, e.g., system scope and granularity, a single PID, or a single CID")
			return
		}
		out, err = metrics[0].getHTML()
		return
	} else if format == SummaryCSV {
		for i, m := range metrics {
			var oneOut string
			if oneOut, err = m.getCSV(i == 0); err != nil {
				return
			}
			out += oneOut
		}
		return
	}
	err = fmt.Errorf("unsupported post-processing format: %d", format)
	return
}

type metricStats struct {
	mean   float64
	min    float64
	max    float64
	stddev float64
}

type row struct {
	timestamp float64
	socket    string
	cpu       string
	pid       string
	cmd       string
	cgroup    string
	metrics   map[string]float64
}

// newRow loads a row structure with given fields and field names
func newRow(fields []string, names []string) (r row, err error) {
	r.metrics = make(map[string]float64)
	for fIdx, field := range fields {
		if fIdx == Timestamp {
			var ts float64
			if ts, err = strconv.ParseFloat(field, 64); err != nil {
				return
			}
			r.timestamp = ts
		} else if fIdx == Socket {
			r.socket = field
		} else if fIdx == CPU {
			r.cpu = field
		} else if fIdx == Pid {
			r.pid = field
		} else if fIdx == Cmd {
			r.cmd = field
		} else if fIdx == Cgroup {
			r.cgroup = field
		} else {
			// metrics
			var v float64
			if field != "" {
				if v, err = strconv.ParseFloat(field, 64); err != nil {
					return
				}
			} else {
				v = math.NaN()
			}
			r.metrics[names[fIdx-FirstMetric]] = v
		}
	}
	return
}

const (
	Timestamp int = iota
	Socket
	CPU
	Pid
	Cmd
	Cgroup
	FirstMetric
)

type metricsFromCSV struct {
	names        []string
	rows         []row
	groupByField string
	groupByValue string
}

// newMetricsFromCSV - loads data from CSV. Returns a list of metrics, one per
// scope unit or granularity unit, e.g., one per socket, or one per PID
func newMetricsFromCSV(csvPath string) (metrics []metricsFromCSV, err error) {
	var file *os.File
	if file, err = os.Open(csvPath); err != nil {
		return
	}
	reader := csv.NewReader(file)
	groupByField := -1
	var groupByValues []string
	var metricNames []string
	var nonMetricNames []string
	for idx := 0; true; idx++ {
		var fields []string
		if fields, err = reader.Read(); err != nil {
			if err != io.EOF {
				return
			}
			err = nil
		}
		if fields == nil {
			// no more rows
			break
		}
		if idx == 0 {
			// headers
			for fIdx, field := range fields {
				if fIdx < FirstMetric {
					nonMetricNames = append(nonMetricNames, field)
				} else {
					metricNames = append(metricNames, field)
				}
			}
			continue
		}
		// Determine the scope and granularity of the captured data by looking
		// at the first row of values. If none of these are set, then it's
		// system scope and system granularity
		if idx == 1 {
			if fields[Socket] != "" {
				groupByField = Socket
			} else if fields[CPU] != "" {
				groupByField = CPU
			} else if fields[Pid] != "" {
				groupByField = Pid
			} else if fields[Cgroup] != "" {
				groupByField = Cgroup
			}
		}
		// Load row into a row structure
		var r row
		if r, err = newRow(fields, metricNames); err != nil {
			return
		}
		// put the row into the associated list based on groupByField
		if groupByField == -1 { // system scope/granularity
			if len(metrics) == 0 {
				metrics = append(metrics, metricsFromCSV{})
				metrics[0].names = metricNames
			}
			metrics[0].rows = append(metrics[0].rows, r)
		} else {
			groupByValue := fields[groupByField]
			var listIdx int
			if listIdx, err = util.StringIndexInList(groupByValue, groupByValues); err != nil {
				groupByValues = append(groupByValues, groupByValue)
				metrics = append(metrics, metricsFromCSV{})
				listIdx = len(metrics) - 1
				metrics[listIdx].names = metricNames
				if groupByField == Socket {
					metrics[listIdx].groupByField = nonMetricNames[Socket]
				} else if groupByField == CPU {
					metrics[listIdx].groupByField = nonMetricNames[CPU]
				} else if groupByField == Pid {
					metrics[listIdx].groupByField = nonMetricNames[Pid]
				} else if groupByField == Cgroup {
					metrics[listIdx].groupByField = nonMetricNames[Cgroup]
				}
				metrics[listIdx].groupByValue = groupByValue
			}
			metrics[listIdx].rows = append(metrics[listIdx].rows, r)
		}
	}
	return
}

// getStats - calculate summary stats (min, max, mean, stddev) for each metric
func (m *metricsFromCSV) getStats() (stats map[string]metricStats, err error) {
	stats = make(map[string]metricStats)
	for _, metricName := range m.names {
		min := math.NaN()
		max := math.NaN()
		mean := math.NaN()
		stddev := math.NaN()
		count := 0
		sum := 0.0
		for _, row := range m.rows {
			val := row.metrics[metricName]
			if math.IsNaN(val) {
				continue
			}
			if math.IsNaN(min) { // min was initialized to NaN
				// first non-NaN value, so initialize
				min = math.MaxFloat64
				max = 0
				sum = 0
			}
			if val < min {
				min = val
			}
			if val > max {
				max = val
			}
			sum += val
			count++
		}
		// must be at least one valid value for this metric to calculate mean and standard deviation
		if count > 0 {
			mean = sum / float64(count)
			distanceSquaredSum := 0.0
			for _, row := range m.rows {
				val := row.metrics[metricName]
				if math.IsNaN(val) {
					continue
				}
				distance := mean - val
				squared := distance * distance
				distanceSquaredSum += squared
			}
			stddev = math.Sqrt(distanceSquaredSum / float64(count))
		}
		stats[metricName] = metricStats{mean: mean, min: min, max: max, stddev: stddev}
	}
	return
}

// getHTML - generate a string containing HTML representing the metrics
func (m *metricsFromCSV) getHTML() (html string, err error) {
	var stats map[string]metricStats
	if stats, err = m.getStats(); err != nil {
		return
	}
	var htmlTemplate []byte
	if htmlTemplate, err = resources.ReadFile("resources/base.html"); err != nil {
		return
	}
	html = string(htmlTemplate)
	html = strings.Replace(html, "TRANSACTIONS", "false", 1) // no transactions for now
	// TMA Tab
	if !math.IsNaN(stats["TMA_Frontend_Bound(%)"].mean) { // do we have TMA?
		html = strings.Replace(html, "FRONTEND", fmt.Sprintf("%f", stats["TMA_Frontend_Bound(%)"].mean), -1)
		html = strings.Replace(html, "BACKEND", fmt.Sprintf("%f", stats["TMA_Backend_Bound(%)"].mean), -1)
		html = strings.Replace(html, "COREDATA", fmt.Sprintf("%f", stats["TMA_..Core_Bound(%)"].mean), -1)
		html = strings.Replace(html, "MEMORY", fmt.Sprintf("%f", stats["TMA_..Memory_Bound(%)"].mean), -1)
		html = strings.Replace(html, "BADSPECULATION", fmt.Sprintf("%f", stats["TMA_Bad_Speculation(%)"].mean), -1)
		html = strings.Replace(html, "RETIRING", fmt.Sprintf("%f", stats["TMA_Retiring(%)"].mean), -1)
	} else {
		html = strings.Replace(html, "FRONTEND", "0", -1)
		html = strings.Replace(html, "BACKEND", "0", -1)
		html = strings.Replace(html, "COREDATA", "0", -1)
		html = strings.Replace(html, "MEMORY", "0", -1)
		html = strings.Replace(html, "BADSPECULATION", "0", -1)
		html = strings.Replace(html, "RETIRING", "0", -1)
	}
	type tmplReplace struct {
		tmplVar    string
		metricName string
	}
	templateReplace := []tmplReplace{
		// CPU Tab
		{"CPUUTIL", "CPU utilization %"},
		{"CPIDATA", "CPI"},
		{"CPUFREQ", "CPU operating frequency (in GHz)"},
		// Memory Tab
		{"L1DATA", "L1D MPI (includes data+rfo w/ prefetches)"},
		{"L2DATA", "L2 MPI (includes code+data+rfo w/ prefetches)"},
		{"LLCDATA", "LLC data read MPI (demand+prefetch)"},
		{"READDATA", "memory bandwidth read (MB/sec)"},
		{"WRITEDATA", "memory bandwidth write (MB/sec)"},
		{"TOTALDATA", "memory bandwidth total (MB/sec)"},
		{"REMOTENUMA", "NUMA %_Reads addressed to remote DRAM"},
		// Power Tab
		{"PKGPOWER", "package power (watts)"},
		{"DRAMPOWER", "DRAM power (watts)"},
	}
	for _, tmpl := range templateReplace {
		var series [][]float64
		var firstTimestamp float64
		for rIdx, row := range m.rows {
			if rIdx == 0 {
				firstTimestamp = row.timestamp
			}
			if math.IsNaN(row.metrics[tmpl.metricName]) {
				continue
			}
			series = append(series, []float64{row.timestamp - firstTimestamp, row.metrics[tmpl.metricName]})
		}
		var seriesBytes []byte
		if seriesBytes, err = json.Marshal(series); err != nil {
			return
		}
		html = strings.Replace(html, tmpl.tmplVar, string(seriesBytes), -1)
	}
	// All Metrics Tab
	var metricHTMLStats [][]string
	for _, name := range m.names {
		metricHTMLStats = append(metricHTMLStats, []string{
			name,
			fmt.Sprintf("%f", stats[name].mean),
			fmt.Sprintf("%f", stats[name].min),
			fmt.Sprintf("%f", stats[name].max),
			fmt.Sprintf("%f", stats[name].stddev),
		})
	}
	var jsonMetricsBytes []byte
	if jsonMetricsBytes, err = json.Marshal(metricHTMLStats); err != nil {
		return
	}
	jsonMetrics := string(jsonMetricsBytes)
	html = strings.Replace(html, "ALLMETRICS", string(jsonMetrics), -1)
	return
}

// getCSV - generate CSV string representing the summary statistics of the metrics
func (m *metricsFromCSV) getCSV(includeFieldNames bool) (out string, err error) {
	var stats map[string]metricStats
	if stats, err = m.getStats(); err != nil {
		return
	}
	if includeFieldNames {
		out = "metric,mean,min,max,stddev\n"
		if m.groupByField != "" {
			out = m.groupByField + "," + out
		}
	}
	for _, name := range m.names {
		if m.groupByValue == "" {
			out += fmt.Sprintf("%s,%f,%f,%f,%f\n", name, stats[name].mean, stats[name].min, stats[name].max, stats[name].stddev)
		} else {
			out += fmt.Sprintf("%s,%s,%f,%f,%f,%f\n", m.groupByValue, name, stats[name].mean, stats[name].min, stats[name].max, stats[name].stddev)
		}
	}
	return
}
