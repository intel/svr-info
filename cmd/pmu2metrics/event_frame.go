/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
//
// Linux perf event output, i.e., from 'perf stat' parsing and processing helper functions
//
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/intel/svr-info/internal/util"
	"golang.org/x/exp/slices"
)

// EventGroup represents a group of perf events and their values
type EventGroup struct {
	EventValues map[string]float64 // event name -> event value
	GroupID     int
	Percentage  float64
}

// EventFrame represents the list of EventGroups collected with a specific timestamp
// and sometimes present cgroup
type EventFrame struct {
	EventGroups []EventGroup
	Timestamp   float64
	Socket      string
	CPU         string
	Cgroup      string
}

// Event represents the structure of an event output by perf stat...with
// a few exceptions
type Event struct {
	Interval     float64 `json:"interval"`
	CPU          string  `json:"cpu"`
	CounterValue string  `json:"counter-value"`
	Unit         string  `json:"unit"`
	Cgroup       string  `json:"cgroup"`
	Event        string  `json:"event"`
	EventRuntime int     `json:"event-runtime"`
	PcntRunning  float64 `json:"pcnt-running"`
	Value        float64 // parsed value
	Group        int     // event group index
	Socket       string  // only relevant if granularity is socket
}

// GetEventFrames organizes raw events received from perf into one or more frames (groups of events) that
// will be used for calculating metrics.
//
// The raw events received from perf will differ based on the scope of collection. Current options
// are system-wide, process, cgroup(s). Cgroup scoped data is received intermixed, i.e., multiple
// cgroups' data is represented in the rawEvents list. Process scoped data is received for only
// one process at a time.
//
// The frames produced will differ based on the intended metric granularity. Current options are
// system, socket, cpu (thread/logical CPU), but only when in system scope. Process and cgroup scope
// only support system-level granularity.
func GetEventFrames(rawEvents [][]byte, eventGroupDefinitions []GroupDefinition, scope Scope, granularity Granularity, metadata Metadata) (eventFrames []EventFrame, err error) {
	// parse raw events into list of Event
	var allEvents []Event
	if allEvents, err = parseEvents(rawEvents, eventGroupDefinitions); err != nil {
		return
	}
	// coalesce events to one or more lists based on scope and granularity
	var coalescedEvents [][]Event
	if coalescedEvents, err = coalesceEvents(allEvents, scope, granularity, metadata); err != nil {
		return
	}
	// create one EventFrame per list of Events
	for _, events := range coalescedEvents {
		// organize events into groups
		group := EventGroup{EventValues: make(map[string]float64)}
		var lastGroupID int
		var eventFrame EventFrame
		for eventIdx, event := range events {
			if eventIdx == 0 {
				lastGroupID = event.Group
				eventFrame.Timestamp = event.Interval
				if gCmdLineArgs.granularity == GranularityCPU {
					eventFrame.CPU = event.CPU
				} else if gCmdLineArgs.granularity == GranularitySocket {
					eventFrame.Socket = event.Socket
				}
				if gCmdLineArgs.scope == ScopeCgroup {
					eventFrame.Cgroup = event.Cgroup
				}
			}
			if event.Group != lastGroupID {
				eventFrame.EventGroups = append(eventFrame.EventGroups, group)
				group = EventGroup{EventValues: make(map[string]float64)}
				lastGroupID = event.Group
			}
			group.GroupID = event.Group
			group.Percentage = event.PcntRunning
			group.EventValues[event.Event] = event.Value
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

// parseEvents parses the raw event data into a list of Event
func parseEvents(rawEvents [][]byte, eventGroupDefinitions []GroupDefinition) (events []Event, err error) {
	events = make([]Event, 0, len(rawEvents))
	groupIdx := 0
	eventIdx := -1
	previousEvent := ""
	for _, rawEvent := range rawEvents {
		var event Event
		if event, err = parseEventJSON(rawEvent); err != nil {
			err = fmt.Errorf("failed to parse perf event: %v", err)
			return
		}
		if event.Event != previousEvent {
			eventIdx++
			previousEvent = event.Event
		}
		if eventIdx == len(eventGroupDefinitions[groupIdx]) { // last event in group
			groupIdx++
			if groupIdx == len(eventGroupDefinitions) {
				if gCmdLineArgs.scope == ScopeCgroup {
					groupIdx = 0
				} else {
					err = fmt.Errorf("event group definitions not aligning with raw events")
					return
				}
			}
			eventIdx = 0
		}
		event.Group = groupIdx
		events = append(events, event)
	}
	return
}

// coalesceEvents separates the events into a number of event lists by granularity and scope
func coalesceEvents(allEvents []Event, scope Scope, granularity Granularity, metadata Metadata) (coalescedEvents [][]Event, err error) {
	if scope == ScopeSystem {
		if granularity == GranularitySystem {
			coalescedEvents = append(coalescedEvents, allEvents)
			return
		} else if granularity == GranularitySocket {
			// one list of Events per Socket
			newEvents := make([][]Event, metadata.SocketCount)
			for i := 0; i < metadata.SocketCount; i++ {
				newEvents[i] = make([]Event, 0, len(allEvents)/metadata.SocketCount)
			}
			// merge
			prevSocket := -1
			var socket int
			var newEvent Event
			for i, event := range allEvents {
				var cpu int
				if cpu, err = strconv.Atoi(event.CPU); err != nil {
					return
				}
				socket = metadata.CPUSocketMap[cpu]
				if socket != prevSocket {
					if i != 0 {
						newEvents[prevSocket] = append(newEvents[prevSocket], newEvent)
					}
					prevSocket = socket
					newEvent = event
					newEvent.Socket = fmt.Sprintf("%d", socket)
					continue
				}
				newEvent.Value += event.Value
			}
			newEvents[socket] = append(newEvents[socket], newEvent)
			coalescedEvents = append(coalescedEvents, newEvents...)
			return
		} else if granularity == GranularityCPU {
			// create one list of Events per CPU
			numCPUs := metadata.SocketCount * metadata.CoresPerSocket * metadata.ThreadsPerCore
			newEvents := make([][]Event, numCPUs)
			for i := 0; i < numCPUs; i++ {
				newEvents[i] = make([]Event, 0, len(allEvents)/numCPUs)
			}
			for _, event := range allEvents {
				var cpu int
				if cpu, err = strconv.Atoi(event.CPU); err != nil {
					return
				}
				newEvents[cpu] = append(newEvents[cpu], event)
			}
			coalescedEvents = append(coalescedEvents, newEvents...)
		} else {
			err = fmt.Errorf("unsupported granularity: %d", granularity)
			return
		}
	} else if scope == ScopeProcess {
		coalescedEvents = append(coalescedEvents, allEvents)
		return
	} else if scope == ScopeCgroup {
		// expand events list to one list per cgroup
		var allCgroupEvents [][]Event
		var cgroups []string
		for _, event := range allEvents {
			var cgroupIdx int
			if cgroupIdx, err = util.StringIndexInList(event.Cgroup, cgroups); err != nil {
				cgroups = append(cgroups, event.Cgroup)
				cgroupIdx = len(cgroups) - 1
				allCgroupEvents = append(allCgroupEvents, []Event{})
			}
			allCgroupEvents[cgroupIdx] = append(allCgroupEvents[cgroupIdx], event)
		}
		coalescedEvents = append(coalescedEvents, allCgroupEvents...)
	} else {
		err = fmt.Errorf("unsupported scope: %d", scope)
		return
	}
	return
}

// collapseUncoreGroupsInFrame merges repeated (per-device) uncore groups into a single
// group by summing the values for events that only differ by device ID.
//
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
// For the example above, we will have this:
// 5.005032332,98,,UNC_CHA_TOR_INSERTS.IA_MISS_CRD,2806585867,25.00,,
// 5.005032332,5710,,UNC_CHA_TOR_INSERTS.IA_MISS_DRD_REMOTE,2806585867,25.00,,
// 5.005032332,2261557,,UNC_CHA_TOR_OCCUPANCY.IA_MISS_DRD_REMOTE,2806585867,25.00,,
// Note: uncore event names start with "UNC"
// Note: we assume that uncore events are not mixed into groups that have other event types, e.g., cpu events
func collapseUncoreGroupsInFrame(inFrame EventFrame) (outFrame EventFrame, err error) {
	outFrame = inFrame
	outFrame.EventGroups = []EventGroup{}
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

// isMatchingGroup - groups are considered matching if they include the same event names (ignoring .ID suffix)
func isMatchingGroup(groupA, groupB EventGroup) bool {
	if len(groupA.EventValues) != len(groupB.EventValues) {
		return false
	}
	aNames := make([]string, 0, len(groupA.EventValues))
	bNames := make([]string, 0, len(groupB.EventValues))
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

// collapseUncoreGroups collapses a list of groups into a single group
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

// parseEventJSON parses JSON formatted event into struct
// example: {"interval" : 5.005113019, "cpu": "0", "counter-value" : "22901873.000000", "unit" : "", "cgroup" : "...1cb2de.scope", "event" : "L1D.REPLACEMENT", "event-runtime" : 80081151765, "pcnt-running" : 6.00, "metric-value" : 0.000000, "metric-unit" : "(null)"}
func parseEventJSON(rawEvent []byte) (event Event, err error) {
	if err = json.Unmarshal(rawEvent, &event); err != nil {
		err = fmt.Errorf("unrecognized event format [%s]: %v", rawEvent, err)
		return
	}
	if event.Value, err = strconv.ParseFloat(event.CounterValue, 64); err != nil {
		event.Value = math.NaN()
		err = nil
		if gCmdLineArgs.verbose {
			log.Printf("failed to parse event value: %s", rawEvent)
		}
	}
	return
}
