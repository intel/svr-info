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
	pid       int
	// cid       int
	cmd     string
	metrics map[string]float64
}

const (
	timestamp int = iota
	pid
	//	cid
	cmd
	firstMetric
)

type metricsFromCSV struct {
	names []string
	rows  []row
}

func newMetricsFromCSV(csvPath string) (metrics metricsFromCSV, err error) {
	var file *os.File
	if file, err = os.Open(csvPath); err != nil {
		return
	}
	reader := csv.NewReader(file)
	var fields []string
	for idx := 0; true; idx++ {
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
				if fIdx < firstMetric { // skip non-metrics
					continue
				}
				metrics.names = append(metrics.names, field)
			}
			continue
		}
		r := row{}
		r.metrics = make(map[string]float64)
		for fIdx, field := range fields {
			if fIdx == timestamp {
				var ts float64
				if ts, err = strconv.ParseFloat(field, 64); err != nil {
					return
				}
				r.timestamp = ts
			} else if fIdx == pid {
				var p int
				if field != "" {
					if p, err = strconv.Atoi(field); err != nil {
						return
					}
				}
				r.pid = p
				// } else if fIdx == cid {
				// 	var c int
				// 	if field != "" {
				// 		if c, err = strconv.Atoi(field); err != nil {
				// 			return
				// 		}
				// 	}
				// 	m.cid = c
			} else if fIdx == cmd {
				r.cmd = field
			} else {
				// metrics
				var v float64
				if v, err = strconv.ParseFloat(field, 64); err != nil {
					return
				}
				r.metrics[metrics.names[fIdx-firstMetric]] = v
			}
		}
		metrics.rows = append(metrics.rows, r)
	}
	return
}

func (m *metricsFromCSV) separateByPID() (pidMetrics map[int]metricsFromCSV, err error) {
	pidMetrics = make(map[int]metricsFromCSV)
	for _, row := range m.rows {
		var entry metricsFromCSV
		var ok bool
		if entry, ok = pidMetrics[row.pid]; !ok {
			pidMetrics[row.pid] = metricsFromCSV{}
			entry = pidMetrics[row.pid]
			entry.names = m.names
		}
		entry.rows = append(entry.rows, row)
		pidMetrics[row.pid] = entry
	}
	return
}

func (m *metricsFromCSV) getStats() (stats map[string]metricStats, err error) {
	stats = make(map[string]metricStats)
	for _, metricName := range m.names {
		min := math.NaN()
		max := math.NaN()
		sum := math.NaN()
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
		}
		mean := sum / float64(len(m.rows))
		stddev := math.NaN()
		if !math.IsNaN(mean) {
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
			stddev = distanceSquaredSum / float64(len(m.rows))
		}
		stats[metricName] = metricStats{min: min, max: max, mean: mean, stddev: stddev}
	}
	return
}

func generateHTML(csvInputPath string, pid int) (html string, err error) {
	var metrics metricsFromCSV
	if metrics, err = newMetricsFromCSV(csvInputPath); err != nil {
		return
	}
	var allPIDMetrics map[int]metricsFromCSV
	if allPIDMetrics, err = metrics.separateByPID(); err != nil {
		return
	}
	var pidMetrics metricsFromCSV
	var ok bool
	if pidMetrics, ok = allPIDMetrics[pid]; !ok {
		if pid == 0 {
			err = fmt.Errorf("must specify a pid when post-processing data collected in process mode")
		} else {
			err = fmt.Errorf("specified pid (%d) not found in input file (%s)", pid, csvInputPath)
		}
		return
	}
	var stats map[string]metricStats
	if stats, err = pidMetrics.getStats(); err != nil {
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
	for _, m := range templateReplace {
		var series [][]float64
		var firstTimestamp float64
		for rIdx, row := range metrics.rows {
			if rIdx == 0 {
				firstTimestamp = row.timestamp
			}
			if math.IsNaN(row.metrics[m.metricName]) {
				continue
			}
			series = append(series, []float64{row.timestamp - firstTimestamp, row.metrics[m.metricName]})
		}
		var seriesBytes []byte
		if seriesBytes, err = json.Marshal(series); err != nil {
			return
		}
		html = strings.Replace(html, m.tmplVar, string(seriesBytes), -1)
	}
	// All Metrics Tab
	var metricHTMLStats [][]string
	for _, name := range metrics.names {
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

func generateCSV(csvInputPath string, pid int) (out string, err error) {
	var metrics metricsFromCSV
	if metrics, err = newMetricsFromCSV(csvInputPath); err != nil {
		return
	}
	var allPIDMetrics map[int]metricsFromCSV
	if allPIDMetrics, err = metrics.separateByPID(); err != nil {
		return
	}
	var pidMetrics metricsFromCSV
	var ok bool
	if pidMetrics, ok = allPIDMetrics[pid]; !ok {
		if pid == 0 {
			err = fmt.Errorf("must specify a pid when post-processing data collected in process mode")
		} else {
			err = fmt.Errorf("specified pid (%d) not found in input file (%s)", pid, csvInputPath)
		}
		return
	}
	var stats map[string]metricStats
	if stats, err = pidMetrics.getStats(); err != nil {
		return
	}
	out = "metric,mean,min,max,stddev\n"
	for _, name := range metrics.names {
		out += fmt.Sprintf("%s,%f,%f,%f,%f\n", name, stats[name].mean, stats[name].min, stats[name].max, stats[name].stddev)
	}
	return
}

func postProcess(csvInputPath string, format string, pid string) (out string, err error) {
	p := 0 // system
	if pid != "" {
		if p, err = strconv.Atoi(pid); err != nil {
			return
		}
	}
	if format == "" || strings.ToLower(format) == "html" {
		out, err = generateHTML(csvInputPath, p)
	} else if strings.ToLower(format) == "csv" {
		out, err = generateCSV(csvInputPath, p)
	} else {
		err = fmt.Errorf("unsupported post-processing format: %s", format)
	}
	return
}
