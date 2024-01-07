/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"

	"github.com/Knetic/govaluate"
	mapset "github.com/deckarep/golang-set/v2"
)

type Metric struct {
	Name   string
	Value  float64
	Cgroup string
}

// lock to protect metric variable map that holds the event group where a variable value will be retrieved
var metricVariablesLock = sync.RWMutex{}

// for each variable in a metric, set the best group from which to get its value
func loadMetricBestGroups(metric MetricDefinition, frame EventFrame) (err error) {
	// one thread at a time through this function, since it updates the metric variables map and this only needs to be done one time
	metricVariablesLock.Lock()
	defer metricVariablesLock.Unlock()
	// only load event groups one time for each metric
	loadGroups := false
	for variableName := range metric.Variables {
		if metric.Variables[variableName] == -1 { // group not yet set
			loadGroups = true
			break
		}
		if metric.Variables[variableName] == -2 { // tried previously and failed, don't try again
			err = fmt.Errorf("metric variable group assignment previously failed, skipping: %s", variableName)
			return
		}
	}
	if !loadGroups {
		return // nothing to do, already loaded
	}
	allVariableNames := mapset.NewSetFromMapKeys(metric.Variables)
	remainingVariableNames := allVariableNames.Clone()
	for {
		if remainingVariableNames.Cardinality() == 0 { // found matches for all
			break
		}
		// find group with the greatest number of event names that match the remaining variable names
		bestGroupIdx := -1
		bestMatches := 0
		var matchedNames mapset.Set[string] // := mapset.NewSet([]string{}...)
		for groupIdx, group := range frame.EventGroups {
			groupEventNames := mapset.NewSetFromMapKeys(group.EventValues)
			intersection := remainingVariableNames.Intersect(groupEventNames)
			if intersection.Cardinality() > bestMatches {
				bestGroupIdx = groupIdx
				bestMatches = intersection.Cardinality()
				matchedNames = intersection.Clone()
				if bestMatches == remainingVariableNames.Cardinality() {
					break
				}
			}
		}
		if bestGroupIdx == -1 { // no matches
			for _, variableName := range remainingVariableNames.ToSlice() {
				metric.Variables[variableName] = -2 // we tried and failed
			}
			err = fmt.Errorf("metric variables (%s) not found for metric: %s", strings.Join(remainingVariableNames.ToSlice(), ", "), metric.Name)
			break
		}
		// for each of the matched names, set the value and the group from which to retrieve the value next time
		for _, name := range matchedNames.ToSlice() {
			metric.Variables[name] = bestGroupIdx
		}
		remainingVariableNames = remainingVariableNames.Difference(matchedNames)
	}
	return
}

// get the variable values that will be used to evaluate the metric's expression
func getExpressionVariableValues(metric MetricDefinition, frame EventFrame, previousTimestamp float64, metadata Metadata) (variables map[string]interface{}, err error) {
	variables = make(map[string]interface{})
	if err = loadMetricBestGroups(metric, frame); err != nil {
		err = fmt.Errorf("at least one of the variables couldn't be assigned to a group: %v", err)
		return
	}
	// set the variable values to be used in the expression evaluation
	for variableName := range metric.Variables {
		if metric.Variables[variableName] == -2 {
			err = fmt.Errorf("variable value set to -2 (shouldn't happen): %s", variableName)
			return
		}
		// set the variable value to the event value divided by the perf collection time to normalize the value to 1 second
		if len(frame.EventGroups) <= metric.Variables[variableName] {
			err = fmt.Errorf("event groups have changed")
			return
		}
		variables[variableName] = frame.EventGroups[metric.Variables[variableName]].EventValues[variableName] / (frame.Timestamp - previousTimestamp)
		// adjust cstate_core/c6-residency value if hyperthreading is enabled
		// why here? so we don't have to change the perfmon metric formula
		if metadata.ThreadsPerCore > 1 && variableName == "cstate_core/c6-residency/" {
			variables[variableName] = variables[variableName].(float64) * float64(metadata.ThreadsPerCore)
		}
	}
	return
}

// define functions that can be called in metric expressions
func getEvaluatorFunctions() (functions map[string]govaluate.ExpressionFunction) {
	functions = make(map[string]govaluate.ExpressionFunction)
	functions["max"] = func(args ...interface{}) (interface{}, error) {
		var leftVal float64
		var rightVal float64
		switch t := args[0].(type) {
		case int:
			leftVal = float64(t)
		case float64:
			leftVal = t
		}
		switch t := args[1].(type) {
		case int:
			rightVal = float64(t)
		case float64:
			rightVal = t
		}
		return max(leftVal, rightVal), nil
	}
	functions["min"] = func(args ...interface{}) (interface{}, error) {
		var leftVal float64
		var rightVal float64
		switch t := args[0].(type) {
		case int:
			leftVal = float64(t)
		case float64:
			leftVal = t
		}
		switch t := args[1].(type) {
		case int:
			rightVal = float64(t)
		case float64:
			rightVal = t
		}
		return min(leftVal, rightVal), nil
	}
	return
}

// function to call evaluator so that we can catch panics that come from the evaluator
func evaluateExpression(metric MetricDefinition, variables map[string]interface{}) (result interface{}, err error) {
	defer func() {
		if errx := recover(); errx != nil {
			err = errx.(error)
		}
	}()
	if result, err = metric.Evaluable.Evaluate(variables); err != nil {
		err = fmt.Errorf("%v : %s : %s", err, metric.Name, metric.Expression)
	}
	return
}

func processEvents(perfEvents [][]byte, metricDefinitions []MetricDefinition, previousTimestamp float64, metadata Metadata) (metrics []Metric, timeStamp float64, err error) {
	var eventFrames []EventFrame
	if eventFrames, err = GetEventFrames(perfEvents); err != nil { // arrange the events into groups
		err = fmt.Errorf("failed to put perf events into groups: %v", err)
		return
	}
	for _, eventFrame := range eventFrames {
		timeStamp = eventFrame.Timestamp
		// produce metrics from event groups
		for _, metricDef := range metricDefinitions {
			metric := Metric{Name: metricDef.Name, Value: math.NaN(), Cgroup: eventFrame.Cgroup}
			var variables map[string]interface{}
			if variables, err = getExpressionVariableValues(metricDef, eventFrame, previousTimestamp, metadata); err != nil {
				if gCmdLineArgs.verbose {
					log.Printf("failed to get expression variable values: %v", err)
				}
				err = nil
			} else {
				var result interface{}
				if result, err = evaluateExpression(metricDef, variables); err != nil {
					if gCmdLineArgs.verbose {
						log.Printf("failed to evaluate expression: %v", err)
					}
					err = nil
				} else {
					metric.Value = result.(float64)
				}
			}
			metrics = append(metrics, metric)
			if gCmdLineArgs.veryVerbose {
				var prettyVars []string
				for variableName := range variables {
					prettyVars = append(prettyVars, fmt.Sprintf("%s=%f", variableName, variables[variableName]))
				}
				log.Printf("%s : %s : %s", metricDef.Name, metricDef.Expression, strings.Join(prettyVars, ", "))
			}
		}
	}
	return
}
