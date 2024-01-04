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
)

type metricStats struct {
	mean   float64
	min    float64
	max    float64
	stddev float64
}

type row struct {
	timestamp float64
	pid       string
	cmd       string
	cgroup    string
	metrics   map[string]float64
}

const (
	Timestamp int = iota
	Pid
	Cmd
	Cgroup
	FirstMetric
)

type metricsFromCSV struct {
	names []string
	rows  []row
}

func newMetricsFromCSV(csvPath string, pid string, cgroup string) (metrics metricsFromCSV, err error) {
	var file *os.File
	if file, err = os.Open(csvPath); err != nil {
		return
	}
	reader := csv.NewReader(file)
	firstMatchingCgroup := ""
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
				if fIdx < FirstMetric { // skip non-metrics
					continue
				}
				metrics.names = append(metrics.names, field)
			}
			continue
		}
		r := row{}
		r.metrics = make(map[string]float64)
		for fIdx, field := range fields {
			if fIdx == Timestamp {
				var ts float64
				if ts, err = strconv.ParseFloat(field, 64); err != nil {
					return
				}
				r.timestamp = ts
			} else if fIdx == Pid {
				r.pid = field
			} else if fIdx == Cmd {
				r.cmd = field
			} else if fIdx == Cgroup {
				r.cgroup = field
			} else {
				// metrics
				var v float64
				if v, err = strconv.ParseFloat(field, 64); err != nil {
					return
				}
				r.metrics[metrics.names[fIdx-FirstMetric]] = v
			}
		}
		if r.pid == pid && strings.Contains(r.cgroup, cgroup) {
			if firstMatchingCgroup == "" {
				firstMatchingCgroup = r.cgroup
			}
			if firstMatchingCgroup != r.cgroup {
				err = fmt.Errorf("provided cgroup matches more than one cgroup in CSV data, be more specific")
				return
			}
			metrics.rows = append(metrics.rows, r)
		}
	}
	return
}

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
			stddev = distanceSquaredSum / float64(count)
		}
		stats[metricName] = metricStats{mean: mean, min: min, max: max, stddev: stddev}
	}
	return
}

func (m *metricsFromCSV) generateHTML() (html string, err error) {
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

func (m *metricsFromCSV) generateCSV() (out string, err error) {
	var stats map[string]metricStats
	if stats, err = m.getStats(); err != nil {
		return
	}
	out = "metric,mean,min,max,stddev\n"
	for _, name := range m.names {
		out += fmt.Sprintf("%s,%f,%f,%f,%f\n", name, stats[name].mean, stats[name].min, stats[name].max, stats[name].stddev)
	}
	return
}

func postProcess(csvInputPath string, format string, pid string, cgroup string) (out string, err error) {
	var metrics metricsFromCSV
	if metrics, err = newMetricsFromCSV(csvInputPath, pid, cgroup); err != nil {
		return
	}
	if format == "" || strings.ToLower(format) == "html" {
		out, err = metrics.generateHTML()
	} else if strings.ToLower(format) == "csv" {
		out, err = metrics.generateCSV()
	} else {
		err = fmt.Errorf("unsupported post-processing format: %s", format)
	}
	return
}
