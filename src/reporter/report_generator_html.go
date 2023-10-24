/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	texttemplate "text/template"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v2"
	"intel.com/svr-info/pkg/cpu"
)

const (
	configurationDataIndex int = iota
	benchmarkDataIndex
	profileDataIndex
	analyzeDataIndex
	insightDataIndex
)

const noDataFound = "No data found."

type ReportGeneratorHTML struct {
	reports   []*Report
	outputDir string
	cpusInfo  *cpu.CPU
}

func newReportGeneratorHTML(outputDir string, cpusInfo *cpu.CPU, configurationData *Report, insightData *Report, profileData *Report, benchmarkData *Report, analyzeData *Report) (rpt *ReportGeneratorHTML) {
	rpt = &ReportGeneratorHTML{
		reports:   []*Report{configurationData, benchmarkData, profileData, analyzeData, insightData}, // order matches const indexes defined above
		outputDir: outputDir,
		cpusInfo:  cpusInfo,
	}
	return
}

// ReportGen - struct used within the HTML template
type ReportGen struct {
	HostIndices                      []int
	ConfigurationReport              *Report
	ConfigurationReportReferenceData []*HostReferenceData
	BenchmarkReport                  *Report
	BenchmarkReportReferenceData     []*HostReferenceData
	ProfileReport                    *Report
	ProfileReportReferenceData       []*HostReferenceData
	AnalyzeReport                    *Report
	AnalyzeReportReferenceData       []*HostReferenceData
	InsightsReport                   *Report
	InsightsReportReferenceData      []*HostReferenceData
	Version                          string
}

func newReportGen(reportsData []*Report, hostIndices []int, hostsReferenceData []*HostReferenceData) (gen *ReportGen) {
	gen = &ReportGen{
		HostIndices:                      hostIndices,
		ConfigurationReport:              reportsData[configurationDataIndex],
		ConfigurationReportReferenceData: []*HostReferenceData{},
		BenchmarkReport:                  reportsData[benchmarkDataIndex],
		BenchmarkReportReferenceData:     hostsReferenceData,
		ProfileReport:                    reportsData[profileDataIndex],
		ProfileReportReferenceData:       []*HostReferenceData{},
		AnalyzeReport:                    reportsData[analyzeDataIndex],
		AnalyzeReportReferenceData:       []*HostReferenceData{},
		InsightsReport:                   reportsData[insightDataIndex],
		InsightsReportReferenceData:      []*HostReferenceData{},
	}
	return
}

type HostReferenceData map[string]interface{}
type ReferenceData map[string]HostReferenceData

func newReferenceData() (data *ReferenceData) {
	refYaml, err := resources.ReadFile("resources/reference.yaml")
	if err != nil {
		log.Printf("Failed to read reference.yaml: %v.", err)
		return
	}
	data = &ReferenceData{}
	err = yaml.Unmarshal(refYaml, data)
	if err != nil {
		log.Printf("Failed to parse reference.yaml: %v.", err)
	}
	return
}

func (r *ReportGeneratorHTML) getRefLabel(hostIndex int) (refLabel string) {
	source := r.reports[0].Sources[hostIndex]
	family := source.valFromRegexSubmatch("lscpu", `^CPU family.*:\s*([0-9]+)$`)
	model := source.valFromRegexSubmatch("lscpu", `^Model.*:\s*([0-9]+)$`)
	stepping := source.valFromRegexSubmatch("lscpu", `^Stepping.*:\s*(.+)$`)
	sockets := source.valFromRegexSubmatch("lscpu", `^Socket\(.*:\s*(.+?)$`)
	capid4 := source.valFromRegexSubmatch("lspci bits", `^([0-9a-fA-F]+)`)
	devices := source.valFromRegexSubmatch("lspci devices", `^([0-9]+)`)
	uarch := getMicroArchitecture(r.cpusInfo, family, model, stepping, capid4, devices, sockets)
	if uarch == "" {
		log.Printf("Did not find a known architecture for %s:%s:%s", family, model, stepping)
		return
	}
	refLabel = fmt.Sprintf("%s_%s", uarch, sockets)
	return
}

func (r *ReportGeneratorHTML) loadHostReferenceData(hostIndex int, referenceData *ReferenceData) (data *HostReferenceData) {
	refLabel := r.getRefLabel(hostIndex)
	if refLabel == "" {
		log.Printf("No reference data found for host %d", hostIndex)
		return
	}
	for key, hostReferenceData := range *referenceData {
		if key == refLabel {
			data = &hostReferenceData
			break
		}
	}
	return
}

func (r *ReportGen) RenderMenuItems(reportData *Report) template.HTML {
	var out string
	category := NoCategory
	for _, table := range reportData.Tables {
		if table.Category != category {
			out += fmt.Sprintf(`<a href="#%s">%s</a>`, table.Name, TableCategoryLabels[table.Category])
			category = table.Category
		}
	}
	return template.HTML(out)
}

func renderHTMLTable(tableHeaders []string, tableValues [][]string, class string, valuesStyle [][]string) (out string) {
	if len(tableValues) > 0 {
		out += `<table class="` + class + `">`
		if len(tableHeaders) > 0 {
			out += `<thead>`
			out += `<tr>`
			for _, label := range tableHeaders {
				out += `<th>` + label + `</th>`
			}
			out += `</tr>`
			out += `</thead>`
		}
		out += `<tbody>`
		for rowIdx, rowValues := range tableValues {
			out += `<tr>`
			for colIdx, value := range rowValues {
				var style string
				if len(valuesStyle) > rowIdx && len(valuesStyle[rowIdx]) > colIdx {
					style = ` style="` + valuesStyle[rowIdx][colIdx] + `"`
				}
				out += `<td` + style + `>` + value + `</td>`
			}
			out += `</tr>`
		}
		out += `</tbody>`
		out += `</table>`
	} else {
		out += noDataFound
	}
	return
}

/* Single Value Table is rendered like this:
 *
 *				Hostname 1	|	Hostname 2	|	.....	|	Hostname N
 *	Valname 1	value			value			value		value
 *	Valname 2	value			value			value		value
 *	...			value			value			value		value
 *	Valname N	value			value			value		value
 */
func (r *ReportGen) renderSingleValueTable(table *Table, refData []*HostReferenceData) (out string) {
	var tableHeaders []string
	var tableValues [][]string
	var tableValueStyles [][]string

	// only include column headers if there is more than one host or a single host with reference data
	hostnameHeader := len(r.HostIndices) > 1
	if !hostnameHeader {
		if len(refData) > 0 {
			for _, ref := range refData {
				if _, ok := (*ref)[table.Name]; ok {
					hostnameHeader = true
				}
			}
		}
	}
	if hostnameHeader {
		// header in first column is blank
		tableHeaders = append(tableHeaders, "")
		// include only the hosts in HostIndices
		for _, hostIndex := range r.HostIndices {
			tableHeaders = append(tableHeaders, table.AllHostValues[hostIndex].Name)
		}
		// add a column for each reference
		for _, ref := range refData {
			if _, ok := (*ref)[table.Name]; ok {
				tableHeaders = append(tableHeaders, (*ref)["Hostref"].(map[interface{}]interface{})["Name"].(string))
			}
		}
	}

	// we will not be in this function unless all host values' have the same value names,
	// so use the value names from the first host
	for valueIndex, valueName := range table.AllHostValues[r.HostIndices[0]].ValueNames {
		var rowValues []string
		// first column in row is the value name
		rowValues = append(rowValues, valueName)
		// include only the hosts in HostIndices
		for _, hostIndex := range r.HostIndices {
			hv := table.AllHostValues[hostIndex]
			// if have the value
			if len(hv.Values) > 0 && len(hv.Values[0]) > valueIndex {
				rowValues = append(rowValues, hv.Values[0][valueIndex])
			} else { // value is missing
				rowValues = append(rowValues, "")
			}
		}
		// if reference data is available, add it to the table
		for _, ref := range refData {
			if refData, ok := (*ref)[table.Name]; ok {
				if _, ok := refData.(map[interface{}]interface{})[valueName]; ok {
					rowValues = append(rowValues, refData.(map[interface{}]interface{})[valueName].(string))
				} else {
					rowValues = append(rowValues, "")
				}
			}
		}
		tableValues = append(tableValues, rowValues)
		tableValueStyles = append(tableValueStyles, []string{"font-weight:bold"})
	}
	// if all host data fields are empty string, then don't render the table
	haveData := false
	for _, rowValues := range tableValues {
		for col, val := range rowValues {
			if val != "" && col != 0 {
				if col <= len(r.HostIndices) { // only host data, not reference
					haveData = true
				}
				break
			}
		}
		if haveData {
			break
		}
	}
	if !haveData {
		tableValues = [][]string{} // this will cause renderHTMLTable to indicate "No data found."
	}
	out += renderHTMLTable(tableHeaders, tableValues, "pure-table pure-table-striped", tableValueStyles)
	return
}

/* Multi Value Table is rendered like this:
 *
 *	Hostname 1
 *	Valname 1	|	Valname 2	|	......	|	Valname N
 *	value			value			value		value
 *	value			value			value		value
 *	value			value			value		value
 *	value			value			value		value
 *
 * Hostname 2
 *	Valname 1	|	Valname 2	|	......	|	Valname N
 *	value			value			value		value
 *	value			value			value		value
 *	value			value			value		value
 *	value			value			value		value
 */
func (r *ReportGen) renderMultiValueTable(table *Table, refData []*HostReferenceData) (out string) {
	// include only the host in HostIndices
	for _, hostIndex := range r.HostIndices {
		// hostname above table if more than one hostname
		if len(r.HostIndices) > 1 {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		out += renderHTMLTable(
			table.AllHostValues[hostIndex].ValueNames,
			table.AllHostValues[hostIndex].Values,
			"pure-table pure-table-striped",
			[][]string{},
		)
	}
	return
}

const datasetTemplate = `
{
	label: '{{.Label}}',
	data: [{{.Data}}],
	backgroundColor: '{{.Color}}',
	borderColor: '{{.Color}}',
	borderWidth: 1,
	showLine: true
}
`
const scatterChartTemplate = `<div class="chart-container" style="max-width: 900px">
<canvas id="{{.ID}}"></canvas>
</div>
<script>
new Chart(document.getElementById('{{.ID}}'), {
    type: 'scatter',
    data: {
        datasets: [{{.Datasets}}]
    },
    options: {
        aspectRatio: {{.AspectRatio}},
        scales: {
            x: {
                beginAtZero: false,
                title: {
                    text: "{{.XaxisText}}",
                    display: true
                }
            },
            y: {
                beginAtZero: {{.YaxisZero}},
                title: {
                    text: "{{.YaxisText}}",
                    display: true
                },
            }
        },
        plugins: {
            title: {
                text: "{{.TitleText}}",
                display: {{.DisplayTitle}},
                font: {
                    size: 18
                }
            },
            tooltip: {
                callbacks: {
                    label: function(ctx) {
                        return ctx.dataset.label + " (" + ctx.parsed.x + ", " + ctx.parsed.y + ")";
                    }
                }
            },
            legend: {
                display: {{.DisplayLegend}}
            }
        }
    }
});
</script>
`

type scatterChartTemplateStruct struct {
	ID            string
	Datasets      string
	XaxisText     string
	YaxisText     string
	TitleText     string
	DisplayTitle  string
	DisplayLegend string
	AspectRatio   string
	YaxisZero     string
}

func (r *ReportGen) renderFrequencyChart(table *Table, refData []*HostReferenceData) (out string) {
	// one chart per host
	for _, hostIndex := range r.HostIndices {
		// add hostname only if more than one host or a single host with reference data
		hostnameHeader := len(r.HostIndices) > 1
		if hostnameHeader {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		hv := table.AllHostValues[hostIndex]
		// need at least one set of values
		if len(hv.Values) > 0 {
			var datasets []string
			// spec
			formattedPoints := []string{}
			for _, point := range table.AllHostValues[hostIndex].Values {
				if point[1] != "" {
					formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %s, y: %s}", point[0], point[1]))
				}
			}
			if len(formattedPoints) > 0 {
				specValues := strings.Join(formattedPoints, ",")
				dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
				buf := new(bytes.Buffer)
				err := dst.Execute(buf, struct {
					Label string
					Data  string
					Color string
				}{
					Label: "spec",
					Data:  specValues,
					Color: getColor(0),
				})
				if err != nil {
					return
				}
				datasets = append(datasets, buf.String())
			}
			// measured
			formattedPoints = []string{}
			for _, point := range table.AllHostValues[hostIndex].Values {
				if point[2] != "" {
					formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %s, y: %s}", point[0], point[2]))
				}
			}
			if len(formattedPoints) > 0 {
				measuredValues := strings.Join(formattedPoints, ",")
				dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
				buf := new(bytes.Buffer)
				err := dst.Execute(buf, struct {
					Label string
					Data  string
					Color string
				}{
					Label: "measured",
					Data:  measuredValues,
					Color: getColor(1),
				})
				if err != nil {
					return
				}
				datasets = append(datasets, buf.String())
			}
			if len(datasets) > 0 {
				sct := texttemplate.Must(texttemplate.New("scatterChartTemplate").Parse(scatterChartTemplate))
				buf := new(bytes.Buffer)
				err := sct.Execute(buf, scatterChartTemplateStruct{
					ID:            "scatterchart" + fmt.Sprintf("%d", hostIndex),
					Datasets:      strings.Join(datasets, ","),
					XaxisText:     "Core Count",
					YaxisText:     "Frequency (GHz)",
					TitleText:     "",
					DisplayTitle:  "false",
					DisplayLegend: "true",
					AspectRatio:   "4",
					YaxisZero:     "false",
				})
				if err != nil {
					return
				}
				out += buf.String()
				out += "\n"
				if len(datasets) > 1 {
					out += r.renderFrequencyTable(table, hostIndex)
				}
			} else {
				out += noDataFound
			}
		} else {
			out += noDataFound
		}
	}
	return
}

func (r *ReportGen) renderFrequencyTable(table *Table, hostIndex int) (out string) {
	hv := table.AllHostValues[hostIndex]
	var rows [][]string
	headers := []string{""}
	for i := 0; i < len(hv.Values); i++ {
		headers = append(headers, fmt.Sprintf("%d", i+1))
	}
	specRow := []string{"spec"}
	measRow := []string{"measured"}
	for _, vals := range hv.Values {
		specRow = append(specRow, vals[1])
		measRow = append(measRow, vals[2])
	}
	rows = append(rows, specRow)
	rows = append(rows, measRow)
	valuesStyles := [][]string{}
	valuesStyles = append(valuesStyles, []string{"font-weight:bold"})
	valuesStyles = append(valuesStyles, []string{"font-weight:bold"})
	out = renderHTMLTable(headers, rows, "pure-table pure-table-striped", valuesStyles)
	return
}

func (r *ReportGen) renderAverageCPUUtilizationChart(table *Table, refData []*HostReferenceData) (out string) {
	// one chart per host
	for _, hostIndex := range r.HostIndices {
		// add hostname only if more than one host or a single host with reference data
		hostnameHeader := len(r.HostIndices) > 1
		if hostnameHeader {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		hv := table.AllHostValues[hostIndex]
		// need at least one set of values
		if len(hv.Values) > 0 {
			var datasets []string
			for statIdx, stat := range hv.ValueNames { // 1 data set per stat, e.g., %usr, %nice, etc.
				if statIdx == 0 { // skip Time value
					continue
				}
				formattedPoints := []string{}
				for pointIdx, point := range table.AllHostValues[hostIndex].Values {
					formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %d, y: %s}", pointIdx, point[statIdx]))
				}
				if len(formattedPoints) > 0 {
					specValues := strings.Join(formattedPoints, ",")
					dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
					buf := new(bytes.Buffer)
					err := dst.Execute(buf, struct {
						Label string
						Data  string
						Color string
					}{
						Label: stat,
						Data:  specValues,
						Color: getColor(statIdx - 1),
					})
					if err != nil {
						return
					}
					datasets = append(datasets, buf.String())
				}
			}
			if len(datasets) > 0 {
				sct := texttemplate.Must(texttemplate.New("scatterChartTemplate").Parse(scatterChartTemplate))
				buf := new(bytes.Buffer)
				err := sct.Execute(buf, scatterChartTemplateStruct{
					ID:            "sysutil" + fmt.Sprintf("%d", hostIndex),
					Datasets:      strings.Join(datasets, ","),
					XaxisText:     "Time/Samples",
					YaxisText:     "% Utilization",
					TitleText:     "",
					DisplayTitle:  "false",
					DisplayLegend: "true",
					AspectRatio:   "2",
					YaxisZero:     "true",
				})
				if err != nil {
					return
				}
				out += buf.String()
				out += "\n"
			} else {
				out += noDataFound
			}
		} else {
			out += noDataFound
		}
	}
	return
}

func (r *ReportGen) renderCPUUtilizationChart(table *Table, refData []*HostReferenceData) (out string) {
	// one chart per host
	for _, hostIndex := range r.HostIndices {
		// add hostname only if more than one host or a single host with reference data
		hostnameHeader := len(r.HostIndices) > 1
		if hostnameHeader {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		hv := table.AllHostValues[hostIndex]
		// need at least one set of values
		if len(hv.Values) > 0 {
			var datasets []string
			cpuBusyStats := make(map[int][]float64)
			for _, point := range table.AllHostValues[hostIndex].Values {
				idle, err := strconv.ParseFloat(point[len(point)-1], 64)
				if err != nil {
					continue
				}
				busy := 100.0 - idle
				cpu, err := strconv.Atoi(point[1])
				if err != nil {
					continue
				}
				if _, ok := cpuBusyStats[cpu]; !ok {
					cpuBusyStats[cpu] = []float64{}
				}
				cpuBusyStats[cpu] = append(cpuBusyStats[cpu], busy)
			}
			var keys []int
			for cpu := range cpuBusyStats {
				keys = append(keys, cpu)
			}
			sort.Ints(keys)
			for cpu := range keys {
				stats := cpuBusyStats[cpu]
				formattedPoints := []string{}
				for statIdx, stat := range stats {
					formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %d, y: %0.2f}", statIdx, stat))
				}
				if len(formattedPoints) > 0 {
					specValues := strings.Join(formattedPoints, ",")
					dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
					buf := new(bytes.Buffer)
					err := dst.Execute(buf, struct {
						Label string
						Data  string
						Color string
					}{
						Label: fmt.Sprintf("CPU %d", cpu),
						Data:  specValues,
						Color: getColor(cpu),
					})
					if err != nil {
						return
					}
					datasets = append(datasets, buf.String())
				}
			}
			if len(datasets) > 0 {
				sct := texttemplate.Must(texttemplate.New("scatterChartTemplate").Parse(scatterChartTemplate))
				buf := new(bytes.Buffer)
				err := sct.Execute(buf, scatterChartTemplateStruct{
					ID:            "cpuutil" + fmt.Sprintf("%d", hostIndex),
					Datasets:      strings.Join(datasets, ","),
					XaxisText:     "Time/Samples",
					YaxisText:     "% Utilization",
					TitleText:     "",
					DisplayTitle:  "false",
					DisplayLegend: "false",
					AspectRatio:   "2",
					YaxisZero:     "true",
				})
				if err != nil {
					return
				}
				out += buf.String()
				out += "\n"
			} else {
				out += noDataFound
			}
		} else {
			out += noDataFound
		}
	}
	return
}

func (r *ReportGen) renderIRQRateChart(table *Table, refData []*HostReferenceData) (out string) {
	// one chart per host
	for _, hostIndex := range r.HostIndices {
		// add hostname only if more than one host or a single host with reference data
		hostnameHeader := len(r.HostIndices) > 1
		if hostnameHeader {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		hv := table.AllHostValues[hostIndex]
		// need at least one set of values
		if len(hv.Values) > 0 {
			var datasets []string
			for statIdx, stat := range hv.ValueNames { // 1 data set per stat, e.g., %usr, %nice, etc.
				if statIdx < 2 { // skip Time and CPU values
					continue
				}
				formattedPoints := []string{}
				// collapse per-CPU samples into a total per stat
				timeStamp := table.AllHostValues[hostIndex].Values[0][0] // timestamp off of first sample
				total := 0.0
				xVal := 0
				for _, point := range table.AllHostValues[hostIndex].Values {
					statVal, err := strconv.ParseFloat(point[statIdx], 64)
					if err != nil {
						continue
					}
					total += statVal
					if timeStamp != point[0] {
						formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %d, y: %0.2f}", xVal, total))
						timeStamp = point[0]
						total = 0.0
						xVal += 1
					}
				}
				if len(formattedPoints) > 0 {
					specValues := strings.Join(formattedPoints, ",")
					dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
					buf := new(bytes.Buffer)
					err := dst.Execute(buf, struct {
						Label string
						Data  string
						Color string
					}{
						Label: stat,
						Data:  specValues,
						Color: getColor(statIdx - 1),
					})
					if err != nil {
						return
					}
					datasets = append(datasets, buf.String())
				}
			}
			if len(datasets) > 0 {
				sct := texttemplate.Must(texttemplate.New("scatterChartTemplate").Parse(scatterChartTemplate))
				buf := new(bytes.Buffer)
				err := sct.Execute(buf, scatterChartTemplateStruct{
					ID:            "irqrate" + fmt.Sprintf("%d", hostIndex),
					Datasets:      strings.Join(datasets, ","),
					XaxisText:     "Time/Samples",
					YaxisText:     "IRQ/s",
					TitleText:     "",
					DisplayTitle:  "false",
					DisplayLegend: "true",
					AspectRatio:   "2",
					YaxisZero:     "true",
				})
				if err != nil {
					return
				}
				out += buf.String()
				out += "\n"
			} else {
				out += noDataFound
			}
		} else {
			out += noDataFound
		}
	}
	return
}

func (r *ReportGen) renderDriveStatsChart(table *Table, refData []*HostReferenceData) (out string) {
	// one chart per host drive
	for _, hostIndex := range r.HostIndices {
		// add hostname only if more than one host or a single host with reference data
		hostnameHeader := len(r.HostIndices) > 1
		if hostnameHeader {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		hv := table.AllHostValues[hostIndex]
		// need at least one set of values
		if len(hv.Values) > 0 {
			driveStats := make(map[string][][]string)
			for _, point := range table.AllHostValues[hostIndex].Values {
				drive := point[0]
				if _, ok := driveStats[drive]; !ok {
					driveStats[drive] = [][]string{}
				}
				driveStats[drive] = append(driveStats[drive], point[1:])
			}
			var keys []string
			for drive := range driveStats {
				keys = append(keys, drive)
			}
			sort.Strings(keys)
			for _, drive := range keys {
				var datasets []string
				dstats := driveStats[drive]
				for valIdx := 0; valIdx < len(dstats[0]); valIdx++ { // 1 dataset per stat type, e.g., tps, kB_read/s
					formattedPoints := []string{}
					for statIdx, stat := range dstats {
						formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %d, y: %s}", statIdx, stat[valIdx]))
					}
					if len(formattedPoints) > 0 {
						specValues := strings.Join(formattedPoints, ",")
						dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
						buf := new(bytes.Buffer)
						err := dst.Execute(buf, struct {
							Label string
							Data  string
							Color string
						}{
							Label: hv.ValueNames[valIdx+1],
							Data:  specValues,
							Color: getColor(valIdx),
						})
						if err != nil {
							return
						}
						datasets = append(datasets, buf.String())
					}
				}
				if len(datasets) > 0 {
					sct := texttemplate.Must(texttemplate.New("scatterChartTemplate").Parse(scatterChartTemplate))
					buf := new(bytes.Buffer)
					err := sct.Execute(buf, scatterChartTemplateStruct{
						ID:            "drivestats" + fmt.Sprintf("%d%s", hostIndex, drive),
						Datasets:      strings.Join(datasets, ","),
						XaxisText:     "Time/Samples",
						YaxisText:     "",
						TitleText:     drive,
						DisplayTitle:  "true",
						DisplayLegend: "true",
						AspectRatio:   "2",
						YaxisZero:     "true",
					})
					if err != nil {
						return
					}
					out += buf.String()
					out += "\n"
				} else {
					out += noDataFound
				}
			}
		} else {
			out += noDataFound
		}
	}
	return
}

func (r *ReportGen) renderNetworkStatsChart(table *Table, refData []*HostReferenceData) (out string) {
	// one chart per host nic
	for _, hostIndex := range r.HostIndices {
		// add hostname only if more than one host or a single host with reference data
		hostnameHeader := len(r.HostIndices) > 1
		if hostnameHeader {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		hv := table.AllHostValues[hostIndex]
		// need at least one set of values
		if len(hv.Values) > 0 {
			netStats := make(map[string][][]string)
			for _, point := range table.AllHostValues[hostIndex].Values {
				net := point[1]
				if _, ok := netStats[net]; !ok {
					netStats[net] = [][]string{}
				}
				netStats[net] = append(netStats[net], point[2:])
			}
			var keys []string
			for drive := range netStats {
				keys = append(keys, drive)
			}
			sort.Strings(keys)
			for _, drive := range keys {
				var datasets []string
				dstats := netStats[drive]
				for valIdx := 0; valIdx < len(dstats[0]); valIdx++ { // 1 dataset per stat type, e.g., rxpck/s, txpck/s
					formattedPoints := []string{}
					for statIdx, stat := range dstats {
						formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %d, y: %s}", statIdx, stat[valIdx]))
					}
					if len(formattedPoints) > 0 {
						specValues := strings.Join(formattedPoints, ",")
						dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
						buf := new(bytes.Buffer)
						err := dst.Execute(buf, struct {
							Label string
							Data  string
							Color string
						}{
							Label: hv.ValueNames[valIdx+2],
							Data:  specValues,
							Color: getColor(valIdx),
						})
						if err != nil {
							return
						}
						datasets = append(datasets, buf.String())
					}
				}
				if len(datasets) > 0 {
					sct := texttemplate.Must(texttemplate.New("scatterChartTemplate").Parse(scatterChartTemplate))
					buf := new(bytes.Buffer)
					err := sct.Execute(buf, scatterChartTemplateStruct{
						ID:            "netstats" + fmt.Sprintf("%d%s", hostIndex, drive),
						Datasets:      strings.Join(datasets, ","),
						XaxisText:     "Time/Samples",
						YaxisText:     "",
						TitleText:     drive,
						DisplayTitle:  "true",
						DisplayLegend: "true",
						AspectRatio:   "2",
						YaxisZero:     "true",
					})
					if err != nil {
						return
					}
					out += buf.String()
					out += "\n"
				} else {
					out += noDataFound
				}
			}
		} else {
			out += noDataFound
		}
	}
	return
}

func (r *ReportGen) renderMemoryStatsChart(table *Table, refData []*HostReferenceData) (out string) {
	// one chart per host
	for _, hostIndex := range r.HostIndices {
		// add hostname only if more than one host or a single host with reference data
		hostnameHeader := len(r.HostIndices) > 1
		if hostnameHeader {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		hv := table.AllHostValues[hostIndex]
		// need at least one set of values
		if len(hv.Values) > 0 {
			var datasets []string
			for statIdx, stat := range hv.ValueNames { // 1 data set per stat, e.g., %usr, %nice, etc.
				if statIdx == 0 { // skip Time value
					continue
				}
				formattedPoints := []string{}
				for pointIdx, point := range table.AllHostValues[hostIndex].Values {
					formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %d, y: %s}", pointIdx, point[statIdx]))
				}
				if len(formattedPoints) > 0 {
					specValues := strings.Join(formattedPoints, ",")
					dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
					buf := new(bytes.Buffer)
					err := dst.Execute(buf, struct {
						Label string
						Data  string
						Color string
					}{
						Label: stat,
						Data:  specValues,
						Color: getColor(statIdx - 1),
					})
					if err != nil {
						return
					}
					datasets = append(datasets, buf.String())
				}
			}
			if len(datasets) > 0 {
				sct := texttemplate.Must(texttemplate.New("scatterChartTemplate").Parse(scatterChartTemplate))
				buf := new(bytes.Buffer)
				err := sct.Execute(buf, scatterChartTemplateStruct{
					ID:            "memstat" + fmt.Sprintf("%d", hostIndex),
					Datasets:      strings.Join(datasets, ","),
					XaxisText:     "Time/Samples",
					YaxisText:     "kilobytes",
					TitleText:     "",
					DisplayTitle:  "false",
					DisplayLegend: "true",
					AspectRatio:   "2",
					YaxisZero:     "true",
				})
				if err != nil {
					return
				}
				out += buf.String()
				out += "\n"
			} else {
				out += noDataFound
			}
		} else {
			out += noDataFound
		}
	}
	return
}

func (r *ReportGen) renderPowerStatsChart(table *Table, refData []*HostReferenceData) (out string) {
	// one chart per host
	for _, hostIndex := range r.HostIndices {
		// add hostname only if more than one host or a single host with reference data
		hostnameHeader := len(r.HostIndices) > 1
		if hostnameHeader {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		hv := table.AllHostValues[hostIndex]
		// need at least one set of values
		if len(hv.Values) > 0 {
			var datasets []string
			for statIdx, stat := range hv.ValueNames { // 1 data set per stat, e.g., Package, DRAM
				formattedPoints := []string{}
				for pointIdx, point := range table.AllHostValues[hostIndex].Values {
					formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %d, y: %s}", pointIdx, point[statIdx]))
				}
				if len(formattedPoints) > 0 {
					specValues := strings.Join(formattedPoints, ",")
					dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
					buf := new(bytes.Buffer)
					err := dst.Execute(buf, struct {
						Label string
						Data  string
						Color string
					}{
						Label: stat,
						Data:  specValues,
						Color: getColor(statIdx),
					})
					if err != nil {
						return
					}
					datasets = append(datasets, buf.String())
				}
			}
			if len(datasets) > 0 {
				sct := texttemplate.Must(texttemplate.New("scatterChartTemplate").Parse(scatterChartTemplate))
				buf := new(bytes.Buffer)
				err := sct.Execute(buf, scatterChartTemplateStruct{
					ID:            "powerstat" + fmt.Sprintf("%d", hostIndex),
					Datasets:      strings.Join(datasets, ","),
					XaxisText:     "Time/Samples",
					YaxisText:     "Watts",
					TitleText:     "",
					DisplayTitle:  "false",
					DisplayLegend: "true",
					AspectRatio:   "2",
					YaxisZero:     "true",
				})
				if err != nil {
					return
				}
				out += buf.String()
				out += "\n"
			} else {
				out += noDataFound
			}
		} else {
			out += noDataFound
		}
	}
	return
}

const flameGraphTemplate = `
<div id="chart{{.ID}}"></div>
<script type="text/javascript">
  var chart{{.ID}} = flamegraph()
    .width(900)
	.cellHeight(18)
    .inverted(true)
	.minFrameSize(1);
  d3.select("#chart{{.ID}}")
    .datum({{.Data}})
    .call(chart{{.ID}});
</script>
`

type flameGraphTemplateStruct struct {
	ID   string
	Data string
}

// Folded data conversion adapted from https://github.com/spiermar/burn
// Copyright © 2017 Martin Spier <spiermar@gmail.com>
// Apache License, Version 2.0
func reverse(strings []string) {
	for i, j := 0, len(strings)-1; i < j; i, j = i+1, j-1 {
		strings[i], strings[j] = strings[j], strings[i]
	}
}

type Node struct {
	Name     string
	Value    int
	Children map[string]*Node
}

func (n *Node) Add(stackPtr *[]string, index int, value int) {
	n.Value += value
	if index >= 0 {
		head := (*stackPtr)[index]
		childPtr, ok := n.Children[head]
		if !ok {
			childPtr = &(Node{head, 0, make(map[string]*Node)})
			n.Children[head] = childPtr
		}
		childPtr.Add(stackPtr, index-1, value)
	}
}

func (n *Node) MarshalJSON() ([]byte, error) {
	v := make([]Node, 0, len(n.Children))
	for _, value := range n.Children {
		v = append(v, *value)
	}

	return json.Marshal(&struct {
		Name     string `json:"name"`
		Value    int    `json:"value"`
		Children []Node `json:"children"`
	}{
		Name:     n.Name,
		Value:    n.Value,
		Children: v,
	})
}

func convertFoldedToJSON(folded string) (out string, err error) {
	rootNode := Node{Name: "root", Value: 0, Children: make(map[string]*Node)}
	scanner := bufio.NewScanner(strings.NewReader(folded))
	for scanner.Scan() {
		line := scanner.Text()
		sep := strings.LastIndex(line, " ")
		s := line[:sep]
		v := line[sep+1:]
		stack := strings.Split(s, ";")
		reverse(stack)
		var i int
		i, err = strconv.Atoi(v)
		if err != nil {
			return
		}
		rootNode.Add(&stack, len(stack)-1, i)
	}
	outbytes, err := rootNode.MarshalJSON()
	out = string(outbytes)
	return
}

func renderFlameGraph(header string, hv *HostValues, field string, hostIndex int) (out string) {
	out += fmt.Sprintf("<h2>%s</h2>\n", header)
	fieldIdx, err := findValueIndex(hv, field)
	if err != nil {
		log.Panicf("didn't find expected field (%s) in table: %v", field, err)
	}
	folded := hv.Values[0][fieldIdx]
	if folded == "" {
		out += noDataFound
		return
	}
	jsonStacks, err := convertFoldedToJSON(folded)
	if err != nil {
		log.Printf("failed to convert folded data: %v", err)
		out += "Error."
		return
	}
	fg := texttemplate.Must(texttemplate.New("flameGraphTemplate").Parse(flameGraphTemplate))
	buf := new(bytes.Buffer)
	err = fg.Execute(buf, flameGraphTemplateStruct{
		ID:   fmt.Sprintf("%d%s", hostIndex, header),
		Data: jsonStacks,
	})
	if err != nil {
		log.Printf("failed to render flame graph template: %v", err)
		out += "Error."
		return
	}
	out += buf.String()
	out += "\n"
	return
}

func (r *ReportGen) renderCodePathFrequency(table *Table) (out string) {
	for _, hostIndex := range r.HostIndices {
		// add hostname only if more than one host or a single host with reference data
		hostnameHeader := len(r.HostIndices) > 1
		if hostnameHeader {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		hv := table.AllHostValues[hostIndex]
		if len(hv.Values) > 0 {
			out += renderFlameGraph("System", &hv, "System Paths", hostIndex)
			out += renderFlameGraph("Java", &hv, "Java Paths", hostIndex)
		} else {
			out += noDataFound
		}
	}
	return
}

func getColor(idx int) string {
	// color-blind safe palette from here: http://mkweb.bcgsc.ca/colorblind/palettes.mhtml#page-container
	colors := []string{"#9F0162", "#009F81", "#FF5AAF", "#00FCCF", "#8400CD", "#008DF9", "#00C2F9", "#FFB2FD", "#A40122", "#E20134", "#FF6E3A", "#FFC33B"}
	return colors[idx%len(colors)]
}

func (r *ReportGen) renderBandwidthLatencyChart(table *Table, refData []*HostReferenceData) (out string) {
	var datasets []string
	colorIdx := 0
	for _, hostIndex := range r.HostIndices {
		hv := table.AllHostValues[hostIndex]
		formattedPoints := []string{}
		for _, point := range table.AllHostValues[hostIndex].Values {
			if point[1] != "" {
				formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %s, y: %s}", point[1], point[0]))
			}
		}
		if len(formattedPoints) > 0 {
			data := strings.Join(formattedPoints, ",")
			dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
			buf := new(bytes.Buffer)
			err := dst.Execute(buf, struct {
				Label string
				Data  string
				Color string
			}{
				Label: hv.Name,
				Data:  data,
				Color: getColor(colorIdx),
			})
			if err != nil {
				return
			}
			datasets = append(datasets, buf.String())
			colorIdx++
		}
	}
	if len(refData) > 0 && len(datasets) > 0 {
		for _, ref := range refData {
			if _, ok := (*ref)[table.Name]; ok {
				hostname := (*ref)["Hostref"].(map[interface{}]interface{})["Name"].(string)
				formattedPoints := []string{}
				for _, point := range (*ref)[table.Name].([]interface{}) {
					latency := point.([]interface{})[0].(float64)
					bandwidth := point.([]interface{})[1].(float64) / 1000
					formattedPoints = append(formattedPoints, fmt.Sprintf("{x: %.0f, y: %.0f}", bandwidth, latency))
				}
				if len(formattedPoints) > 0 {
					data := strings.Join(formattedPoints, ",")
					dst := texttemplate.Must(texttemplate.New("datasetTemplate").Parse(datasetTemplate))
					buf := new(bytes.Buffer)
					err := dst.Execute(buf, struct {
						Label string
						Data  string
						Color string
					}{
						Label: hostname,
						Data:  data,
						Color: getColor(colorIdx),
					})
					if err != nil {
						return
					}
					datasets = append(datasets, buf.String())
					colorIdx++
				}
			}
		}
	}
	if len(datasets) > 0 {
		sct := texttemplate.Must(texttemplate.New("scatterChartTemplate").Parse(scatterChartTemplate))
		buf := new(bytes.Buffer)
		err := sct.Execute(buf, scatterChartTemplateStruct{
			ID:            "memBandwdithLatency",
			Datasets:      strings.Join(datasets, ","),
			XaxisText:     "Bandwidth (GB/s)",
			YaxisText:     "Latency (ns)",
			TitleText:     "",
			DisplayTitle:  "false",
			DisplayLegend: "true",
			AspectRatio:   "2",
			YaxisZero:     "true",
		})
		if err != nil {
			return
		}
		out += buf.String()
		out += "\n"
	} else {
		out += noDataFound
		out += "<br>Using the OSS release of svr-info? Memory benchmarks require Intel Memory Latency Checker (MLC) be downloaded, extracted, and the Linux binary placed in the svr-info/extras directory. See the repo README for additional information."
	}
	return
}

/* A NUMA Bandwidth table is rendered like this:
 *
 *   Hostname 1
 *   Node   |   0   |   1   |  ...  |   N
 *    0        val     val     val     val
 *    1        val     val     val     val
 *   ...       val     val     val     val
 *    N        val     val     val     val
 *
 *   Hostname 2
 *   ...
 */
func (r *ReportGen) renderNumaBandwidthTable(table *Table, refData []*HostReferenceData) (out string) {

	for _, hostIndex := range r.HostIndices {
		var tableHeaders []string
		var tableValues [][]string
		var tableValueStyles [][]string
		// add hostname only if more than one host or a single host with reference data
		hostnameHeader := len(r.HostIndices) > 1
		if !hostnameHeader {
			if len(refData) > 0 {
				for _, ref := range refData {
					if _, ok := (*ref)[table.Name]; ok {
						hostnameHeader = true
					}
				}
			}
		}
		tableHeaders = append(tableHeaders, "Node")
		for nodeIdx, node := range table.AllHostValues[hostIndex].Values {
			tableHeaders = append(tableHeaders, fmt.Sprintf("%d", nodeIdx))
			var rowValues []string
			rowValues = append(rowValues, fmt.Sprintf("%d", nodeIdx))
			bandwidths := strings.Split(node[1], ",")
			rowValues = append(rowValues, bandwidths...)
			tableValues = append(tableValues, rowValues)
			tableValueStyles = append(tableValueStyles, []string{"font-weight:bold"})
		}
		if hostnameHeader && (len(tableValues) > 0 || len(r.HostIndices) > 1) {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		out += renderHTMLTable(tableHeaders, tableValues, "pure-table pure-table-striped", tableValueStyles)
	}
	// if reference data is available, create a table for each reference data set
	/* Reference data format:
	   - - 67528.4 # 0, 0
	     - 30178.1 # 0, 1
	   - - 30177.9 # 1, 0
	     - 66665.4 # 1, 1
	*/
	// add ref data if host data tables rendered
	if strings.Contains(out, "</table>") {
		for _, ref := range refData {
			if _, ok := (*ref)[table.Name]; ok {
				var tableHeaders []string
				var tableValues [][]string
				var tableValueStyles [][]string
				out += `<h3>` + (*ref)["Hostref"].(map[interface{}]interface{})["Name"].(string) + `</h3>`
				tableHeaders = append(tableHeaders, "Node")
				for nodeIdx, node := range (*ref)[table.Name].([]interface{}) {
					tableHeaders = append(tableHeaders, fmt.Sprintf("%d", nodeIdx))
					var rowValues []string
					rowValues = append(rowValues, fmt.Sprintf("%d", nodeIdx))
					for _, bandwidth := range node.([]interface{}) {
						rowValues = append(rowValues, fmt.Sprintf("%.1f", bandwidth))
					}
					tableValues = append(tableValues, rowValues)
					tableValueStyles = append(tableValueStyles, []string{"font-weight:bold"})
				}
				out += renderHTMLTable(tableHeaders, tableValues, "pure-table pure-table-striped", tableValueStyles)
			}
		}
	}
	return
}

func dimmDetails(dimm []string) (details string) {
	if strings.Contains(dimm[SizeIdx], "No") {
		details = "No Module Installed"
	} else {
		// Intel PMEM modules may have serial number appended to end of part number...
		// strip that off so it doesn't mess with color selection later
		partNumber := dimm[PartIdx]
		if strings.Contains(dimm[DetailIdx], "Synchronous Non-Volatile") &&
			dimm[ManufacturerIdx] == "Intel" &&
			strings.HasSuffix(dimm[PartIdx], dimm[SerialIdx]) {
			partNumber = dimm[PartIdx][:len(dimm[PartIdx])-len(dimm[SerialIdx])]
		}
		details = dimm[SizeIdx] + " @" + dimm[ConfiguredSpeedIdx]
		details += " " + dimm[TypeIdx] + " " + dimm[DetailIdx]
		details += " " + dimm[ManufacturerIdx] + " " + partNumber
	}
	return
}

func (r *ReportGen) renderDIMMPopulationTable(table *Table, refData []*HostReferenceData) (out string) {
	htmlColors := []string{"lightgreen", "orange", "aqua", "lime", "yellow", "beige", "magenta", "violet", "salmon", "pink"}
	// a DIMM Population table for every host
	for _, hostIndex := range r.HostIndices {
		var slotColorIndices = make(map[string]int)
		// header if more than one host
		if len(r.HostIndices) > 1 {
			out += `<h3>` + table.AllHostValues[hostIndex].Name + `</h3>`
		}
		// socket -> channel -> slot -> dimm details
		var dimms = map[string]map[string]map[string]string{}
		for _, vals := range table.AllHostValues[hostIndex].Values {
			if _, ok := dimms[vals[DerivedSocketIdx]]; !ok {
				dimms[vals[DerivedSocketIdx]] = map[string]map[string]string{}
			}
			if _, ok := dimms[vals[DerivedSocketIdx]][vals[DerivedChannelIdx]]; !ok {
				dimms[vals[DerivedSocketIdx]][vals[DerivedChannelIdx]] = map[string]string{}
			}
			dimms[vals[DerivedSocketIdx]][vals[DerivedChannelIdx]][vals[DerivedSlotIdx]] = dimmDetails(vals)
		}
		var socketTableHeaders = []string{"Socket", ""}
		var socketTableValues [][]string
		var socketKeys []string
		for k := range dimms {
			socketKeys = append(socketKeys, k)
		}
		sort.Strings(socketKeys)
		for _, socket := range socketKeys {
			socketMap := dimms[socket]
			socketTableValues = append(socketTableValues, []string{})
			var channelTableHeaders = []string{"Channel", "Slots"}
			var channelTableValues [][]string
			var channelKeys []int
			for k := range socketMap {
				channel, _ := strconv.Atoi(k)
				channelKeys = append(channelKeys, channel)
			}
			sort.Ints(channelKeys)
			for _, channel := range channelKeys {
				channelMap := socketMap[strconv.Itoa(channel)]
				channelTableValues = append(channelTableValues, []string{})
				var slotTableHeaders []string
				var slotTableValues [][]string
				var slotTableValuesStyles [][]string
				var slotKeys []string
				for k := range channelMap {
					slotKeys = append(slotKeys, k)
				}
				sort.Strings(slotKeys)
				slotTableValues = append(slotTableValues, []string{})
				slotTableValuesStyles = append(slotTableValuesStyles, []string{})
				for _, slot := range slotKeys {
					dimmDetails := channelMap[slot]
					slotTableValues[0] = append(slotTableValues[0], dimmDetails)
					var slotColor string
					if dimmDetails == "No Module Installed" {
						slotColor = "background-color:silver"
					} else {
						if _, ok := slotColorIndices[dimmDetails]; !ok {
							slotColorIndices[dimmDetails] = int(math.Min(float64(len(slotColorIndices)), float64(len(htmlColors)-1)))
						}
						slotColor = "background-color:" + htmlColors[slotColorIndices[dimmDetails]]
					}
					slotTableValuesStyles[0] = append(slotTableValuesStyles[0], slotColor)
				}
				slotTable := renderHTMLTable(slotTableHeaders, slotTableValues, "pure-table pure-table-bordered", slotTableValuesStyles)
				// channel number
				channelTableValues[len(channelTableValues)-1] = append(channelTableValues[len(channelTableValues)-1], strconv.Itoa(channel))
				// slot table
				channelTableValues[len(channelTableValues)-1] = append(channelTableValues[len(channelTableValues)-1], slotTable)
				// style
			}
			channelTable := renderHTMLTable(channelTableHeaders, channelTableValues, "pure-table pure-table-bordered", [][]string{})
			// socket number
			socketTableValues[len(socketTableValues)-1] = append(socketTableValues[len(socketTableValues)-1], socket)
			// channel table
			socketTableValues[len(socketTableValues)-1] = append(socketTableValues[len(socketTableValues)-1], channelTable)
		}
		out += renderHTMLTable(socketTableHeaders, socketTableValues, "pure-table pure-table-bordered", [][]string{})
	}
	return
}

// if there's one value per value name
//
//	and
//
// if the value names are the same across hosts
func isSingleValueTable(table *Table) bool {
	var valueNames []string
	for _, hv := range table.AllHostValues {
		if len(hv.Values) > 1 {
			return false
		}
		if len(valueNames) == 0 {
			valueNames = hv.ValueNames
		}
		if len(hv.ValueNames) > 0 && !cmp.Equal(hv.ValueNames, valueNames) {
			return false
		}
	}
	return true
}

// HTMLEscapeTable - escape value names and values
func HTMLEscapeTable(table *Table) (safeTable Table) {
	safeTable.Name = table.Name
	safeTable.Category = table.Category
	for _, hv := range table.AllHostValues {
		var safeHv HostValues
		safeHv.Name = hv.Name
		for _, name := range hv.ValueNames {
			safeHv.ValueNames = append(safeHv.ValueNames, html.EscapeString(name))
		}
		for _, values := range hv.Values {
			var safeValues []string
			for _, value := range values {
				safeValues = append(safeValues, html.EscapeString(value))
			}
			safeHv.Values = append(safeHv.Values, safeValues)
		}
		safeTable.AllHostValues = append(safeTable.AllHostValues, safeHv)
	}
	return
}

func (r *ReportGen) RenderDataTable(unsafeTable *Table, refData []*HostReferenceData) template.HTML {
	t := HTMLEscapeTable(unsafeTable)
	table := &t
	out := fmt.Sprintf("<h2 id=%s>%s</h2>\n", "\""+table.Name+"\"", table.Name)
	if table.Name == "Core Frequency" {
		out += r.renderFrequencyChart(table, refData)
	} else if table.Name == "Memory Bandwidth and Latency" {
		out += r.renderBandwidthLatencyChart(table, refData)
	} else if table.Name == "Memory NUMA Bandwidth" {
		out += r.renderNumaBandwidthTable(table, refData)
	} else if table.Name == "DIMM Population" {
		out += r.renderDIMMPopulationTable(table, refData)
	} else if table.Name == "Average CPU Utilization" {
		out += r.renderAverageCPUUtilizationChart(table, refData)
	} else if table.Name == "CPU Utilization" {
		out += r.renderCPUUtilizationChart(table, refData)
	} else if table.Name == "IRQ Rate" {
		out += r.renderIRQRateChart(table, refData)
	} else if table.Name == "Drive Stats" {
		out += r.renderDriveStatsChart(table, refData)
	} else if table.Name == "Network Stats" {
		out += r.renderNetworkStatsChart(table, refData)
	} else if table.Name == "Memory Stats" {
		out += r.renderMemoryStatsChart(table, refData)
	} else if table.Name == "Code Path Frequency" {
		out += r.renderCodePathFrequency(table)
	} else if table.Name == "Power Stats" {
		out += r.renderPowerStatsChart(table, refData)
	} else if isSingleValueTable(table) {
		out += r.renderSingleValueTable(table, refData)
	} else {
		out += r.renderMultiValueTable(table, refData)
	}
	return template.HTML(out)
}

func (r *ReportGeneratorHTML) generate() (reportFilePaths []string, err error) {
	t, err := template.ParseFS(resources, "resources/report.html.tmpl")
	if err != nil {
		return
	}
	referenceData := newReferenceData()
	var hostnames []string
	for _, values := range r.reports[0].Tables[0].AllHostValues {
		hostnames = append(hostnames, values.Name)
	}
	// one HTML report for each host in reportData
	for hostIndex, hostname := range hostnames {
		// get the reference data for this host, if any
		var hostsReferenceData []*HostReferenceData
		hostReferenceData := r.loadHostReferenceData(hostIndex, referenceData)
		if hostReferenceData != nil {
			hostsReferenceData = append(hostsReferenceData, hostReferenceData)
		}
		fileName := hostname + ".html"
		reportFilePath := filepath.Join(r.outputDir, fileName)
		var f *os.File
		f, err = os.OpenFile(reportFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return
		}
		err = t.Execute(f, newReportGen(r.reports, []int{hostIndex}, hostsReferenceData))
		f.Close()
		if err != nil {
			return
		}
		reportFilePaths = append(reportFilePaths, reportFilePath)
	}
	// if more than one host, create a combined report
	if len(hostnames) > 1 {
		// get unique host reference data, if any
		var hostsReferenceData []*HostReferenceData
		for hostIndex := range hostnames {
			hostReferenceData := r.loadHostReferenceData(hostIndex, referenceData)
			if hostReferenceData != nil {
				// make sure we don't already have this one in the list
				alreadyHaveIt := false
				for _, ref := range hostsReferenceData {
					if (*hostReferenceData)["Hostref"].(map[interface{}]interface{})["Name"].(string) ==
						(*ref)["Hostref"].(map[interface{}]interface{})["Name"].(string) {
						alreadyHaveIt = true
						break
					}
				}
				if !alreadyHaveIt {
					hostsReferenceData = append(hostsReferenceData, hostReferenceData)
				}
			}
		}
		fileName := "all_hosts" + ".html"
		reportFilePath := filepath.Join(r.outputDir, fileName)
		var f *os.File
		f, err = os.OpenFile(reportFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return
		}
		var hostIndices []int
		for i := 0; i < len(hostnames); i++ {
			hostIndices = append(hostIndices, i)
		}
		err = t.Execute(f, newReportGen(r.reports, hostIndices, hostsReferenceData))
		f.Close()
		if err != nil {
			return
		}
		reportFilePaths = append(reportFilePaths, reportFilePath)
	}
	return
}
