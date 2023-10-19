package main

import (
	"encoding/json"
	"fmt"
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
	Name                string                         `json:"name"`
	Expression          string                         `json:"expression"`
	Variables           map[string]int                 // parsed from Expression for efficiency, int represents group index
	EvaluatorExpression *govaluate.EvaluableExpression // parse each metric one time
}

// transform if/else to ternary conditional (? :) so expression evaluator can handle it
// simple:
// from: <expression 1> if <condition> else <expression 2>
// to:   <condition> ? <expression 1> : <expression 2>
// less simple:
// from: <expression 0> ((<expression 1>) if <condition> else (<expression 2>)) <expression 3>
// to:   <expression 0> (<condition> ? (<expression 1>) : <expression 2) <expression 3>
func transformConditional(in string) (out string, err error) {
	var idxIf, idxElse, idxExpression1, idxExpression3 int
	if idxIf = strings.Index(in, "if"); idxIf == -1 {
		out = in
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
	return
}

// load metrics from file
func loadMetricDefinitions(metricDefinitionOverridePath string, groupDefinitions []GroupDefinition, metadata Metadata) (metrics []MetricDefinition, err error) {
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
	if err = json.Unmarshal(bytes, &metrics); err != nil {
		return
	}
	for metricIdx := range metrics {
		// transform if/else to ?/:
		if metrics[metricIdx].Expression, err = transformConditional(metrics[metricIdx].Expression); err != nil {
			return
		}
		// replace constants with their values
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[SYSTEM_TSC_FREQ]", fmt.Sprintf("%f", float64(metadata.TSCFrequencyHz)))
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[TSC]", fmt.Sprintf("%f", float64(metadata.TSC)))
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[CORES_PER_SOCKET]", fmt.Sprintf("%f", float64(metadata.CoresPerSocket)))
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[CHAS_PER_SOCKET]", fmt.Sprintf("%f", float64(metadata.DeviceCounts["cha"])))
		metrics[metricIdx].Expression = strings.ReplaceAll(metrics[metricIdx].Expression, "[SOCKET_COUNT]", fmt.Sprintf("%f", float64(metadata.SocketCount)))
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
	}
	return
}
