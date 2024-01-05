/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"golang.org/x/exp/slices"
)

type EventGroup struct {
	EventValues map[string]float64 // event name -> event value
	GroupID     int
	Percentage  float64
}

// EventFrame -- the list of EventGroups collected with a specific timestamp
type EventFrame struct {
	EventGroups []EventGroup
	Timestamp   float64
	Cgroup      string
}

// groups are considered matching if includes the same event names (ignoring .ID suffix)
func isMatchingGroup(groupA, groupB EventGroup) bool {
	if len(groupA.EventValues) != len(groupB.EventValues) {
		return false
	}
	var aNames, bNames []string
	for eventAName := range groupA.EventValues {
		parts := strings.Split(eventAName, ".")
		newName := strings.Join(parts[:len(parts)-1], ".")
		aNames = append(aNames, newName)
	}
	for eventBName := range groupB.EventValues {
		parts := strings.Split(eventBName, ".")
		newName := strings.Join(parts[:len(parts)-1], ".")
		bNames = append(bNames, newName)
	}
	slices.Sort(aNames)
	slices.Sort(bNames)
	for nameIdx, name := range aNames {
		if name != bNames[nameIdx] {
			return false
		}
	}
	return true
}

func collapseUncoreGroups(inGroups []EventGroup, firstIdx int, count int) (outGroup EventGroup, err error) {
	outGroup.GroupID = inGroups[firstIdx].GroupID
	outGroup.Percentage = inGroups[firstIdx].Percentage
	outGroup.EventValues = make(map[string]float64)
	for i := firstIdx; i <= firstIdx+count; i++ {
		for name, value := range inGroups[i].EventValues {
			parts := strings.Split(name, ".")
			newName := strings.Join(parts[:len(parts)-1], ".")
			if _, ok := outGroup.EventValues[newName]; !ok {
				outGroup.EventValues[newName] = 0
			}
			outGroup.EventValues[newName] += value
		}
	}
	return
}

// collapseUncoreGroupsInFrame
// uncore events are received in repeated perf groups like this:
// group:
// 5.005032332,49,,UNC_CHA_TOR_INSERTS.IA_MISS_CRD.0,2806917160,25.00,,
// 5.005032332,2720,,UNC_CHA_TOR_INSERTS.IA_MISS_DRD_REMOTE.0,2806917160,25.00,,
// 5.005032332,1061494,,UNC_CHA_TOR_OCCUPANCY.IA_MISS_DRD_REMOTE.0,2806917160,25.00,,
// group:
// 5.005032332,49,,UNC_CHA_TOR_INSERTS.IA_MISS_CRD.1,2806585867,25.00,,
// 5.005032332,2990,,UNC_CHA_TOR_INSERTS.IA_MISS_DRD_REMOTE.1,2806585867,25.00,,
// 5.005032332,1200063,,UNC_CHA_TOR_OCCUPANCY.IA_MISS_DRD_REMOTE.1,2806585867,25.00,,
//
// We need to merge the repeated groups into a single group by sum-ing the values for
// events that only differ by the device ID, e.g., 1, 2, 3, appended to the end of the
// event name, and remove the .<device_id> from the end of the name in the new group
// For the example above, we will have this:
// 5.005032332,98,,UNC_CHA_TOR_INSERTS.IA_MISS_CRD,2806585867,25.00,,
// 5.005032332,5710,,UNC_CHA_TOR_INSERTS.IA_MISS_DRD_REMOTE,2806585867,25.00,,
// 5.005032332,2261557,,UNC_CHA_TOR_OCCUPANCY.IA_MISS_DRD_REMOTE,2806585867,25.00,,
// Note: uncore event names start with "UNC"
// Note: we assume that uncore events are not mixed into groups that have other event types, e.g., cpu events
func collapseUncoreGroupsInFrame(inFrame EventFrame) (outFrame EventFrame, err error) {
	outFrame.Timestamp = inFrame.Timestamp
	outFrame.Cgroup = inFrame.Cgroup
	var idxUncoreMatches []int
	for inGroupIdx, inGroup := range inFrame.EventGroups {
		// skip groups that have been collapsed
		if slices.Contains(idxUncoreMatches, inGroupIdx) {
			continue
		}
		idxUncoreMatches = []int{}
		foundUncore := false
		for eventName := range inGroup.EventValues {
			// only check the first entry
			if strings.HasPrefix(eventName, "UNC") {
				foundUncore = true
			}
			break
		}
		if foundUncore {
			// we need to know how many of the following groups (if any) match the current group
			// so they can be merged together into a single group
			for i := inGroupIdx + 1; i < len(inFrame.EventGroups); i++ {
				if isMatchingGroup(inGroup, inFrame.EventGroups[i]) {
					// keep track of the groups that match so we can skip processing them since
					// they will be merged into a single group
					idxUncoreMatches = append(idxUncoreMatches, i)
				} else {
					break
				}
			}
			var outGroup EventGroup
			if outGroup, err = collapseUncoreGroups(inFrame.EventGroups, inGroupIdx, len(idxUncoreMatches)); err != nil {
				return
			}
			outFrame.EventGroups = append(outFrame.EventGroups, outGroup)
		} else {
			outFrame.EventGroups = append(outFrame.EventGroups, inGroup)
		}
	}
	return
}

type Event struct {
	Timestamp  float64 `json:"interval"`
	ValueStr   string  `json:"counter-value"`
	Value      float64
	Units      string  `json:"unit"`
	Cgroup     string  `json:"cgroup"`
	Name       string  `json:"event"`
	GroupID    int     `json:"event-runtime"`
	Percentage float64 `json:"pcnt-running"`
}

// parse JSON formatted event
// {"interval" : 5.005113019, "counter-value" : "22901873.000000", "unit" : "", "cgroup" : "...1cb2de.scope", "event" : "L1D.REPLACEMENT", "event-runtime" : 80081151765, "pcnt-running" : 6.00, "metric-value" : 0.000000, "metric-unit" : "(null)"}
func parseEventJSON(rawEvent []byte) (event Event, err error) {
	if err = json.Unmarshal(rawEvent, &event); err != nil {
		err = fmt.Errorf("unrecognized event format [%s]: %v", rawEvent, err)
		return
	}
	if event.Value, err = strconv.ParseFloat(event.ValueStr, 64); err != nil {
		event.Value = math.NaN()
		err = nil
		if gCmdLineArgs.verbose {
			log.Printf("failed to parse event value: %s", rawEvent)
		}
	}
	return
}

// organize events received from perf into groups where event values can be accessed by event name
func getEventFrames(rawEvents [][]byte) (eventFrames []EventFrame, err error) {
	// parse and separate events by cgroup (may be empty)
	cgroupEvents := make(map[string][]Event)
	for _, rawEvent := range rawEvents {
		var event Event
		if event, err = parseEventJSON(rawEvent); err != nil {
			err = fmt.Errorf("failed to parse perf event: %v", err)
			return
		}
		cgroupEvents[event.Cgroup] = append(cgroupEvents[event.Cgroup], event)
	}
	// one EventFrame per cgroup
	group := EventGroup{EventValues: make(map[string]float64)}
	for cgroup, events := range cgroupEvents {
		var lastGroupID int
		var eventFrame EventFrame
		for eventIdx, event := range events {
			if eventIdx == 0 {
				lastGroupID = event.GroupID
				eventFrame.Timestamp = event.Timestamp
				eventFrame.Cgroup = cgroup
			}
			if event.GroupID != lastGroupID {
				eventFrame.EventGroups = append(eventFrame.EventGroups, group)
				group = EventGroup{EventValues: make(map[string]float64)}
				lastGroupID = event.GroupID
			}
			group.GroupID = event.GroupID
			group.Percentage = event.Percentage
			group.EventValues[event.Name] = event.Value
		}
		// add the last group
		eventFrame.EventGroups = append(eventFrame.EventGroups, group)
		// TODO: can we collapse uncore groups as we're parsing (above)?
		if eventFrame, err = collapseUncoreGroupsInFrame(eventFrame); err != nil {
			return
		}
		eventFrames = append(eventFrames, eventFrame)
	}
	return
}
