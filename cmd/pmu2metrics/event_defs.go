/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
//
// helper functions for parsing and interpreting the architecture-specific perf event definition files
//
package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
)

// EventDefinition represents a single perf event
type EventDefinition struct {
	Raw    string
	Name   string
	Device string
}

// GroupDefinition represents a group of perf events
type GroupDefinition []EventDefinition

// LoadEventGroups reads the events defined in the architecture specific event definition file, then
// expands them to include the per-device uncore events
func LoadEventGroups(eventDefinitionOverridePath string, metadata Metadata) (groups []GroupDefinition, err error) {
	var file fs.File
	if eventDefinitionOverridePath != "" {
		if file, err = os.Open(eventDefinitionOverridePath); err != nil {
			return
		}
	} else {
		if file, err = resources.Open(filepath.Join("resources", fmt.Sprintf("%s_events.txt", metadata.Microarchitecture))); err != nil {
			return
		}
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	uncollectableEvents := mapset.NewSet[string]()
	var group GroupDefinition
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		var event EventDefinition
		if event, err = parseEventDefinition(line[:len(line)-1]); err != nil {
			return
		}
		var collectable bool
		if collectable, err = isCollectableEvent(event, metadata); err != nil {
			return
		}
		if collectable {
			group = append(group, event)
		} else {
			uncollectableEvents.Add(event.Name)
		}
		if line[len(line)-1] == ';' {
			// end of group detected
			if len(group) > 0 {
				groups = append(groups, group)
			} else if gCmdLineArgs.verbose {
				log.Printf("No collectable events in group ending with %s", line)
			}
			group = GroupDefinition{} // clear the list
		}
	}
	if err = scanner.Err(); err != nil {
		return
	}
	// expand uncore groups for all uncore devices
	groups, err = expandUncoreGroups(groups, metadata)
	// "fixed" PMU counters are not supported on (most) IaaS VMs, so we add a separate group
	if !isUncoreSupported(metadata) {
		group = GroupDefinition{EventDefinition{Raw: "cpu-cycles"}, EventDefinition{Raw: "instructions"}}
		if metadata.RefCyclesSupported {
			group = append(group, EventDefinition{Raw: "ref-cycles"})
		}
		groups = append(groups, group)
		group = GroupDefinition{EventDefinition{Raw: "cpu-cycles:k"}, EventDefinition{Raw: "instructions"}}
		if metadata.RefCyclesSupported {
			group = append(group, EventDefinition{Raw: "ref-cycles:k"})
		}
		groups = append(groups, group)

	}
	if uncollectableEvents.Cardinality() != 0 && gCmdLineArgs.verbose {
		log.Printf("Uncollectable events: %s", uncollectableEvents)
	}
	return
}

// isUncoreSupported confirms if platform has exposed uncore devices
func isUncoreSupported(metadata Metadata) (supported bool) {
	supported = false
	for uncoreDeviceName := range metadata.DeviceIDs {
		if uncoreDeviceName == "cha" { // could be any uncore device
			supported = true
			break
		}
	}
	return
}

// isCollectableEvent confirms if given event can be collected on the platform
func isCollectableEvent(event EventDefinition, metadata Metadata) (collectable bool, err error) {
	collectable = true
	// TMA
	if !metadata.TMASupported && (event.Name == "TOPDOWN.SLOTS" || strings.HasPrefix(event.Name, "PERF_METRICS.")) {
		collectable = false
		return
	}
	// short-circuit for cpu events
	if event.Device == "cpu" && !strings.HasPrefix(event.Name, "OCR") {
		return
	}
	// short-circuit off-core response events
	if event.Device == "cpu" &&
		strings.HasPrefix(event.Name, "OCR") &&
		isUncoreSupported(metadata) &&
		!(gCmdLineArgs.scope == ScopeProcess) &&
		!(gCmdLineArgs.scope == ScopeCgroup) {
		return
	}
	// exclude uncore events when
	// - their corresponding device is not found
	// - not in system-wide collection scope
	if event.Device != "cpu" && event.Device != "" {
		if gCmdLineArgs.scope == ScopeProcess || gCmdLineArgs.scope == ScopeCgroup {
			collectable = false
			return
		}
		deviceExists := false
		for uncoreDeviceName := range metadata.DeviceIDs {
			if event.Device == uncoreDeviceName {
				deviceExists = true
				break
			}
		}
		if !deviceExists {
			collectable = false
		} else if !strings.Contains(event.Raw, "umask") && !strings.Contains(event.Raw, "event") {
			collectable = false
		}
		return
	}
	// if we got this far, event.Device is empty
	// is ref-cycles supported?
	if !metadata.RefCyclesSupported && strings.Contains(event.Name, "ref-cycles") {
		collectable = false
		return
	}
	// no uncore means we're on a VM where cpu fixed cycles are likely not supported
	if strings.Contains(event.Name, "cpu-cycles") && !isUncoreSupported(metadata) {
		collectable = false
		return
	}
	// no cstate and power events when collecting at process or cgroup scope
	if (gCmdLineArgs.scope == ScopeProcess || gCmdLineArgs.scope == ScopeCgroup) &&
		(strings.Contains(event.Name, "cstate_") || strings.Contains(event.Name, "power/energy")) {
		collectable = false
		return
	}
	// finally, if it isn't in the perf list output, it isn't collectable
	name := strings.Split(event.Name, ":")[0]
	collectable = strings.Contains(metadata.PerfSupportedEvents, name)
	return
}

// parseEventDefinition parses one line from the event definition file into a representative structure
func parseEventDefinition(line string) (eventDef EventDefinition, err error) {
	eventDef.Raw = line
	fields := strings.Split(line, ",")
	if len(fields) == 1 {
		eventDef.Name = fields[0]
	} else if len(fields) > 1 {
		nameField := fields[len(fields)-1]
		if nameField[:5] != "name=" {
			err = fmt.Errorf("unrecognized event format, name field not found: %s", line)
			return
		}
		eventDef.Name = nameField[6 : len(nameField)-2]
		eventDef.Device = strings.Split(fields[0], "/")[0]
	} else {
		err = fmt.Errorf("unrecognized event format: %s", line)
		return
	}
	return
}

// expandUncoreGroup expands a perf event group into a list of groups where each group is
// associated with an uncore device
func expandUncoreGroup(group GroupDefinition, ids []int, re *regexp.Regexp) (groups []GroupDefinition, err error) {
	for _, deviceID := range ids {
		var newGroup GroupDefinition
		for _, event := range group {
			match := re.FindStringSubmatch(event.Raw)
			if len(match) == 0 {
				err = fmt.Errorf("unexpected raw event format: %s", event.Raw)
				return
			}
			var newEvent EventDefinition
			newEvent.Name = fmt.Sprintf("%s.%d", match[4], deviceID)
			newEvent.Raw = fmt.Sprintf("uncore_%s_%d/event=%s,umask=%s,name='%s'/", match[1], deviceID, match[2], match[3], newEvent.Name)
			newEvent.Device = event.Device
			newGroup = append(newGroup, newEvent)
		}
		groups = append(groups, newGroup)
	}
	return
}

// expandUncoreGroups expands groups with uncore events to include events for all uncore devices
// assumes that uncore device events are in their own groups, not mixed with other device types
func expandUncoreGroups(groups []GroupDefinition, metadata Metadata) (expandedGroups []GroupDefinition, err error) {
	// example 1: cha/event=0x35,umask=0xc80ffe01,name='UNC_CHA_TOR_INSERTS.IA_MISS_CRD'/,
	// expand to: uncore_cha_0/event=0x35,umask=0xc80ffe01,name='UNC_CHA_TOR_INSERTS.IA_MISS_CRD.0'/,
	// example 2: cha/event=0x36,umask=0x21,config1=0x4043300000000,name='UNC_CHA_TOR_OCCUPANCY.IA_MISS.0x40433'/
	// expand to: uncore_cha_0/event=0x36,umask=0x21,config1=0x4043300000000,name='UNC_CHA_TOR_OCCUPANCY.IA_MISS.0x40433'/
	re := regexp.MustCompile(`(\w+)/event=(0x[0-9,a-f,A-F]+),umask=(0x[0-9,a-f,A-F]+.*),name='(.*)'`)
	for _, group := range groups {
		device := group[0].Device
		if device == "cha" || device == "upi" || device == "imc" || device == "iio" {
			var newGroups []GroupDefinition
			if len(metadata.DeviceIDs[device]) == 0 {
				if gCmdLineArgs.verbose {
					log.Printf("No uncore devices found for %s", device)
				}
				continue
			}
			if newGroups, err = expandUncoreGroup(group, metadata.DeviceIDs[device], re); err != nil {
				return
			}
			expandedGroups = append(expandedGroups, newGroups...)
		} else {
			expandedGroups = append(expandedGroups, group)
		}
	}
	return
}
