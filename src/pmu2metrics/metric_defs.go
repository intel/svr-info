package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knetic/govaluate"
)

type Variable struct {
	Name          string
	EventGroupIdx int // initialized to -1 to indicate that a group has not yet been identified
}

type MetricDefinition struct {
	Name       string                         `json:"name"`
	Expression string                         `json:"expression"`
	Variables  map[string]int                 // parsed from Expression for efficiency, int represents group index
	Evaluable  *govaluate.EvaluableExpression // parse expression once, store here for use in metric evaluation
}

// transform if/else to ternary conditional (? :) so expression evaluator can handle it
// simple:
// from: <expression 1> if <condition> else <expression 2>
// to:   <condition> ? <expression 1> : <expression 2>
// less simple:
// from: <expression 0> ((<expression 1>) if <condition> else (<expression 2>)) <expression 3>
// to:   <expression 0> (<condition> ? (<expression 1>) : <expression 2) <expression 3>
func transformConditional(origIn string) (out string, err error) {
	numIfs := strings.Count(origIn, "if")
	if numIfs == 0 {
		out = origIn
		return
	}
	in := origIn
	for i := 0; i < numIfs; i++ {
		if i > 0 {
			in = out
		}
		var idxIf, idxElse, idxExpression1, idxExpression3 int
		if idxIf = strings.Index(in, "if"); idxIf == -1 {
			err = fmt.Errorf("didn't find expected if: %s", in)
			return
		}
		if idxElse = strings.Index(in, "else"); idxElse == -1 {
			err = fmt.Errorf("if without else in expression: %s", in)
			return
		}
		// find the beginning of expression 1 (also end of expression 0)
		var parens int
		for i := idxIf - 1; i >= 0; i-- {
			c := in[i]
			if c == ')' {
				parens += 1
			} else if c == '(' {
				parens -= 1
			} else {
				continue
			}
			if parens < 0 {
				idxExpression1 = i + 1
				break
			}
		}
		// find the end of expression 2 (also beginning of expression 3)
		parens = 0
		for i, c := range in[idxElse+5:] {
			if c == '(' {
				parens += 1
			} else if c == ')' {
				parens -= 1
			} else {
				continue
			}
			if parens < 0 {
				idxExpression3 = i + idxElse + 6
				break
			}
		}
		if idxExpression3 == 0 {
			idxExpression3 = len(in)
		}
		expression0 := in[:idxExpression1]
		expression1 := in[idxExpression1 : idxIf-1]
		condition := in[idxIf+3 : idxElse-1]
		expression2 := in[idxElse+5 : idxExpression3]
		expression3 := in[idxExpression3:]
		var space0, space3 string
		if expression0 != "" {
			space0 = " "
		}
		if expression3 != "" {
			space3 = " "
		}
		out = fmt.Sprintf("%s%s%s ? %s : %s%s%s", expression0, space0, condition, expression1, expression2, space3, expression3)
	}
	return
}

// true if string is in list of strings
func stringInList(s string, l []string) bool {
	for _, item := range l {
		if item == s {
			return true
		}
	}
	return false
}

// load metrics from file
func loadMetricDefinitions(metricDefinitionOverridePath string, selectedMetrics []string, metadata Metadata) (metrics []MetricDefinition, err error) {
	var bytes []byte
	if metricDefinitionOverridePath != "" {
		if bytes, err = os.ReadFile(metricDefinitionOverridePath); err != nil {
			return
		}
	} else {
		if bytes, err = resources.ReadFile(filepath.Join("resources", fmt.Sprintf("%s_metrics.json", metadata.Microarchitecture))); err != nil {
			return
		}
	}
	var metricsInFile []MetricDefinition
	if err = json.Unmarshal(bytes, &metricsInFile); err != nil {
		return
	}
	// remove "metric_" prefix from metric names
	for i := range metricsInFile {
		metricsInFile[i].Name = strings.TrimPrefix(metricsInFile[i].Name, "metric_")
	}
	// if a list of metric names provided, reduce list to match
	if len(selectedMetrics) > 0 {
		// confirm provided metric names are valid (included in metrics defined in file)
		for _, metricName := range selectedMetrics {
			found := false
			for _, metric := range metricsInFile {
				if metricName == metric.Name {
					found = true
					break
				}
			}
			if !found {
				err = fmt.Errorf("provided metric name not found: %s", metricName)
				return
			}
		}
		// build list of metrics based on provided list of metric names
		for _, metric := range metricsInFile {
			if !stringInList(metric.Name, selectedMetrics) {
				continue
			}
			metrics = append(metrics, metric)
		}
	} else {
		metrics = metricsInFile
	}
	return
}

func configureMetrics(metrics []MetricDefinition, evaluatorFunctions map[string]govaluate.ExpressionFunction, metadata Metadata) (err error) {
	// get constants as strings
	tscFreq := fmt.Sprintf("%f", float64(metadata.TSCFrequencyHz))
	tsc := fmt.Sprintf("%f", float64(metadata.TSC))
	coresPerSocket := fmt.Sprintf("%f", float64(metadata.CoresPerSocket))
	chasPerSocket := fmt.Sprintf("%f", float64(metadata.DeviceCounts["cha"]))
	socketCount := fmt.Sprintf("%f", float64(metadata.SocketCount))
	hyperThreadingOn := fmt.Sprintf("%t", metadata.ThreadsPerCore > 1)
	threadsPerCore := fmt.Sprintf("%f", float64(metadata.ThreadsPerCore))
	// configure each metric
	for metricIdx := range metrics {
		// transform if/else to ?/:
		var transformed string
		if transformed, err = transformConditional(metrics[metricIdx].Expression); err != nil {
			return
		}
		if transformed != metrics[metricIdx].Expression {
			if gCmdLineArgs.veryVerbose {
				log.Printf("transformed %s to %s", metrics[metricIdx].Name, transformed)
			}
			metrics[metricIdx].Expression = transformed
		}
		// replace constants with their values
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[SYSTEM_TSC_FREQ]", tscFreq)
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[TSC]", tsc)
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[CORES_PER_SOCKET]", coresPerSocket)
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[CHAS_PER_SOCKET]", chasPerSocket)
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[SOCKET_COUNT]", socketCount)
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[HYPERTHREADING_ON]", hyperThreadingOn)
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[const_thread_count]", threadsPerCore)
		// get a list of the variables in the expression
		metrics[metricIdx].Variables = make(map[string]int)
		expressionIdx := 0
		for {
			startVar := strings.IndexRune(metrics[metricIdx].Expression[expressionIdx:], '[')
			if startVar == -1 { // no more vars in this expression
				break
			}
			endVar := strings.IndexRune(metrics[metricIdx].Expression[expressionIdx:], ']')
			if endVar == -1 {
				err = fmt.Errorf("didn't find end of variable indicator (]) in expression: %s", metrics[metricIdx].Expression[expressionIdx:])
				return
			}
			// add the variable name to the map, set group index to -1 to indicate it has not yet been determined
			metrics[metricIdx].Variables[metrics[metricIdx].Expression[expressionIdx:][startVar+1:endVar]] = -1
			expressionIdx += endVar + 1
		}
		if metrics[metricIdx].Evaluable, err = govaluate.NewEvaluableExpressionWithFunctions(metrics[metricIdx].Expression, evaluatorFunctions); err != nil {
			log.Printf("%v : %s : %s", err, metrics[metricIdx].Name, metrics[metricIdx].Expression)
			return
		}
	}
	return
}
