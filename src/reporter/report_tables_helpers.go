/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
)

func enabledIfVal(val string) string {
	if val != "" {
		return "Enabled"
	}
	return "Disabled"
}

func enabledIfValAndTrue(val string) string {
	if val == "true" {
		return "Enabled"
	}
	if val == "false" {
		return "Disabled"
	}
	return ""
}

func yesIfTrue(val string) string {
	if val == "true" {
		return "Yes"
	}
	return "No"
}

func getDIMMsSummary(tableDIMM *Table, sourceIdx int) (val string) {
	// counts of unique dimm types
	dimmTypeCount := make(map[string]int)
	for _, dimm := range tableDIMM.AllHostValues[sourceIdx].Values {
		dimmKey := dimm[TypeIdx] + ":" + dimm[SizeIdx] + ":" + dimm[SpeedIdx] + ":" + dimm[ConfiguredSpeedIdx]
		if count, ok := dimmTypeCount[dimmKey]; ok {
			dimmTypeCount[dimmKey] = count + 1
		} else {
			dimmTypeCount[dimmKey] = 1
		}
	}
	var summaries []string
	re := regexp.MustCompile(`(\d+)\s*(\w*)`)
	for dimmKey, count := range dimmTypeCount {
		fields := strings.Split(dimmKey, ":")
		match := re.FindStringSubmatch(fields[1]) // size field
		if match != nil {
			size, err := strconv.Atoi(match[1])
			if err != nil {
				log.Printf("Don't recognize DIMM size format: %s", fields[1])
				return
			}
			sum := count * size
			unit := match[2]
			dimmType := fields[0]
			speed := fields[2]
			configuredSpeed := fields[3]
			summary := fmt.Sprintf("%d%s (%dx%d%s %s %s [%s])", sum, unit, count, size, unit, dimmType, speed, configuredSpeed)
			summaries = append(summaries, summary)
		}
	}
	val = strings.Join(summaries, "; ")
	return
}

func getPopulatedMemoryChannels(tableDIMMPopulation *Table, sourceIdx int) string {
	channelsMap := make(map[string]bool)
	for _, dimm := range tableDIMMPopulation.AllHostValues[sourceIdx].Values {
		if !strings.Contains(dimm[SizeIdx], "No") {
			channelsMap[dimm[DerivedSocketIdx]+","+dimm[DerivedChannelIdx]] = true
		}
	}
	if len(channelsMap) > 0 {
		return fmt.Sprintf("%d", len(channelsMap))
	}
	return ""
}

/*
Get DIMM socket and slot from Bank Locator or Locator field from dmidecode.
This method is inherently unreliable/incomplete as each OEM can set
these fields as they see fit.
Returns None when there's no match.
*/
func getDIMMSocketSlot(dimmType DIMMType, reBankLoc *regexp.Regexp, reLoc *regexp.Regexp, bankLocator string, locator string) (socket int, slot int, err error) {
	if dimmType == DIMMType0 {
		match := reLoc.FindStringSubmatch(locator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			slot, _ = strconv.Atoi(match[3])
		}
		return
	} else if dimmType == DIMMType1 {
		match := reLoc.FindStringSubmatch(locator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			slot, _ = strconv.Atoi(match[3])
			return
		}
	} else if dimmType == DIMMType2 {
		match := reLoc.FindStringSubmatch(locator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			slot, _ = strconv.Atoi(match[3])
			return
		}
	} else if dimmType == DIMMType3 {
		match := reBankLoc.FindStringSubmatch(bankLocator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			slot, _ = strconv.Atoi(match[3])
			return
		}
	} else if dimmType == DIMMType4 {
		match := reBankLoc.FindStringSubmatch(bankLocator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			slot, _ = strconv.Atoi(match[4])
			return
		}
	} else if dimmType == DIMMType5 {
		match := reBankLoc.FindStringSubmatch(bankLocator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			slot, _ = strconv.Atoi(match[3])
			return
		}
	} else if dimmType == DIMMType6 {
		match := reLoc.FindStringSubmatch(locator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			socket -= 1
			slot, _ = strconv.Atoi(match[3])
			slot -= 1
			return
		}
	} else if dimmType == DIMMType7 {
		match := reLoc.FindStringSubmatch(locator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			slot, _ = strconv.Atoi(match[3])
			slot -= 1
			return
		}
	} else if dimmType == DIMMType8 {
		match := reBankLoc.FindStringSubmatch(bankLocator)
		if match != nil {
			match2 := reLoc.FindStringSubmatch(locator)
			if match2 != nil {
				socket, _ = strconv.Atoi(match[1])
				socket -= 1
				slot, _ = strconv.Atoi(match2[2])
				slot -= 1
				return
			}
		}
	} else if dimmType == DIMMType9 {
		match := reLoc.FindStringSubmatch(locator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			slot, _ = strconv.Atoi(match[2])
			return
		}
	} else if dimmType == DIMMType10 {
		match := reBankLoc.FindStringSubmatch(bankLocator)
		if match != nil {
			socket = 0
			slot, _ = strconv.Atoi(match[2])
			return
		}
	} else if dimmType == DIMMType11 {
		match := reLoc.FindStringSubmatch(locator)
		if match != nil {
			socket = 0
			slot, _ = strconv.Atoi(match[2])
			return
		}
	} else if dimmType == DIMMType12 {
		match := reLoc.FindStringSubmatch(locator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			socket = socket - 1
			slot, _ = strconv.Atoi(match[3])
			slot = slot - 1
			return
		}
	} else if dimmType == DIMMType13 {
		match := reLoc.FindStringSubmatch(locator)
		if match != nil {
			socket, _ = strconv.Atoi(match[1])
			slot, _ = strconv.Atoi(match[3])
			slot = slot - 1
			return
		}
	}
	err = fmt.Errorf("unrecognized bank locator and/or locator in dimm info: %s %s", bankLocator, locator)
	return
}

type DIMMType int

const (
	DIMMTypeUNKNOWN          = -1
	DIMMType0       DIMMType = iota
	DIMMType1
	DIMMType2
	DIMMType3
	DIMMType4
	DIMMType5
	DIMMType6
	DIMMType7
	DIMMType8
	DIMMType9
	DIMMType10
	DIMMType11
	DIMMType12
	DIMMType13
)

func getDIMMParseInfo(bankLocator string, locator string) (dimmType DIMMType, reBankLoc *regexp.Regexp, reLoc *regexp.Regexp) {
	dimmType = DIMMTypeUNKNOWN
	// Inspur ICX 2s system
	// Needs to be before next regex pattern to differentiate
	reLoc = regexp.MustCompile(`CPU([0-9])_C([0-9])D([0-9])`)
	if reLoc.FindStringSubmatch(locator) != nil {
		dimmType = DIMMType0
		return
	}
	reLoc = regexp.MustCompile(`CPU([0-9])_([A-Z])([0-9])`)
	if reLoc.FindStringSubmatch(locator) != nil {
		dimmType = DIMMType1
		return
	}
	reLoc = regexp.MustCompile(`CPU([0-9])_MC._DIMM_([A-Z])([0-9])`)
	if reLoc.FindStringSubmatch(locator) != nil {
		dimmType = DIMMType2
		return
	}
	reBankLoc = regexp.MustCompile(`NODE ([0-9]) CHANNEL ([0-9]) DIMM ([0-9])`)
	if reBankLoc.FindStringSubmatch(bankLocator) != nil {
		dimmType = DIMMType3
		return
	}
	/* Added for SuperMicro X13DET-B (SPR). Must be before Type4 because Type4 matches, but data in BankLoc is invalid.
	 * Locator: P1-DIMMA1
	 * Locator: P1-DIMMB1
	 * Locator: P1-DIMMC1
	 * ...
	 * Locator: P2-DIMMA1
	 * ...
	 * Note: also matches SuperMicro X11DPT-B (CLX)
	 */
	reLoc = regexp.MustCompile(`P([1,2])-DIMM([A-L])([1,2])`)
	if reLoc.FindStringSubmatch(locator) != nil {
		dimmType = DIMMType12
		return
	}
	reBankLoc = regexp.MustCompile(`P([0-9])_Node([0-9])_Channel([0-9])_Dimm([0-9])`)
	if reBankLoc.FindStringSubmatch(bankLocator) != nil {
		dimmType = DIMMType4
		return
	}
	reBankLoc = regexp.MustCompile(`_Node([0-9])_Channel([0-9])_Dimm([0-9])`)
	if reBankLoc.FindStringSubmatch(bankLocator) != nil {
		dimmType = DIMMType5
		return
	}
	/* SKX SDP
	 * Locator: CPU1_DIMM_A1, Bank Locator: NODE 1
	 * Locator: CPU1_DIMM_A2, Bank Locator: NODE 1
	 */
	reLoc = regexp.MustCompile(`CPU([1-4])_DIMM_([A-Z])([1-2])`)
	if reLoc.FindStringSubmatch(locator) != nil {
		reBankLoc = regexp.MustCompile(`NODE ([1-8])`)
		if reBankLoc.FindStringSubmatch(bankLocator) != nil {
			dimmType = DIMMType6
			return
		}
	}
	/* ICX SDP
	 * Locator: CPU0_DIMM_A1, Bank Locator: NODE 0
	 * Locator: CPU0_DIMM_A2, Bank Locator: NODE 0
	 */
	reLoc = regexp.MustCompile(`CPU([0-7])_DIMM_([A-Z])([1-2])`)
	if reLoc.FindStringSubmatch(locator) != nil {
		reBankLoc = regexp.MustCompile(`NODE ([0-9]+)`)
		if reBankLoc.FindStringSubmatch(bankLocator) != nil {
			dimmType = DIMMType7
			return
		}
	}
	reBankLoc = regexp.MustCompile(`NODE ([1-9]\d*)`)
	if reBankLoc.FindStringSubmatch(bankLocator) != nil {
		reLoc = regexp.MustCompile(`DIMM_([A-Z])([1-9]\d*)`)
		if reLoc.FindStringSubmatch(locator) != nil {
			dimmType = DIMMType8
			return
		}
	}
	/* GIGABYTE MILAN
	 * Locator: DIMM_P0_A0, Bank Locator: BANK 0
	 * Locator: DIMM_P0_A1, Bank Locator: BANK 1
	 * Locator: DIMM_P0_B0, Bank Locator: BANK 0
	 * ...
	 * Locator: DIMM_P1_I0, Bank Locator: BANK 0
	 */
	reLoc = regexp.MustCompile(`DIMM_P([0-1])_[A-Z]([0-1])`)
	if reLoc.FindStringSubmatch(locator) != nil {
		dimmType = DIMMType9
		return
	}
	/* my NUC
	 * Locator: SODIMM0, Bank Locator: CHANNEL A DIMM0
	 * Locator: SODIMM1, Bank Locator: CHANNEL B DIMM0
	 */
	reBankLoc = regexp.MustCompile(`CHANNEL ([A-D]) DIMM([0-9])`)
	if reBankLoc.FindStringSubmatch(bankLocator) != nil {
		dimmType = DIMMType10
		return
	}
	/* Alder Lake Client Desktop
	 * Locator: Controller0-ChannelA-DIMM0, Bank Locator: BANK 0
	 * Locator: Controller1-ChannelA-DIMM0, Bank Locator: BANK 0
	 */
	reLoc = regexp.MustCompile(`Controller([0-1]).*DIMM([0-1])`)
	if reLoc.FindStringSubmatch(locator) != nil {
		dimmType = DIMMType11
		return
	}
	/* BIRCHSTREAM
	 * LOCATOR      BANK LOCATOR
	 * CPU0_DIMM_A1 BANK 0
	 * CPU0_DIMM_A2 BANK 0
	 * CPU0_DIMM_B1 BANK 1
	 * CPU0_DIMM_B2 BANK 1
	 * ...
	 * CPU0_DIMM_H2 BANK 7
	 */
	reLoc = regexp.MustCompile(`CPU([\d])_DIMM_([A-H])([1-2])`)
	if reLoc.FindStringSubmatch(locator) != nil {
		dimmType = DIMMType13
		return
	}
	return
}

func deriveDIMMInfoOther(dimms *[][]string, numSockets int, channelsPerSocket int) (err error) {
	previousSocket, channel := -1, 0
	if len(*dimms) == 0 {
		err = fmt.Errorf("no DIMMs")
		return
	}
	dimmType, reBankLoc, reLoc := getDIMMParseInfo((*dimms)[0][BankLocatorIdx], (*dimms)[0][LocatorIdx])
	if dimmType == DIMMTypeUNKNOWN {
		err = fmt.Errorf("unknown DIMM identification format")
		return
	}
	for _, dimm := range *dimms {
		var socket, slot int
		socket, slot, err = getDIMMSocketSlot(dimmType, reBankLoc, reLoc, dimm[BankLocatorIdx], dimm[LocatorIdx])
		if err != nil {
			log.Printf("Couldn't extract socket and slot from DIMM info: %v", err)
			return
		}
		if socket > previousSocket {
			channel = 0
		} else if previousSocket == socket && slot == 0 {
			channel++
		}
		// sanity check
		if channel >= channelsPerSocket {
			err = fmt.Errorf("invalid interpretation of DIMM data")
			return
		}
		previousSocket = socket
		dimm[DerivedSocketIdx] = fmt.Sprintf("%d", socket)
		dimm[DerivedChannelIdx] = fmt.Sprintf("%d", channel)
		dimm[DerivedSlotIdx] = fmt.Sprintf("%d", slot)
	}
	return
}

/* as seen on 2 socket HPE systems...2 slots per channel
* Locator field has these: PROC 1 DIMM 1, PROC 1 DIMM 2, etc...
* DIMM/slot numbering on board follows logic shown below
 */
func deriveDIMMInfoHPE(dimms *[][]string, numSockets int, channelsPerSocket int) (err error) {
	slotsPerChannel := len(*dimms) / (numSockets * channelsPerSocket)
	re := regexp.MustCompile(`PROC ([1-9]\d*) DIMM ([1-9]\d*)`)
	for _, dimm := range *dimms {
		if !strings.Contains(dimm[BankLocatorIdx], "Not Specified") {
			err = fmt.Errorf("doesn't conform to expected HPE Bank Locator format: %s", dimm[BankLocatorIdx])
			return
		}
		match := re.FindStringSubmatch(dimm[LocatorIdx])
		if match == nil {
			err = fmt.Errorf("doesn't conform to expected HPE Locator format: %s", dimm[LocatorIdx])
			return
		}
		socket, _ := strconv.Atoi(match[1])
		socket -= 1
		dimm[DerivedSocketIdx] = fmt.Sprintf("%d", socket)
		dimmNum, _ := strconv.Atoi(match[2])
		channel := (dimmNum - 1) / slotsPerChannel
		dimm[DerivedChannelIdx] = fmt.Sprintf("%d", channel)
		var slot int
		if (dimmNum < channelsPerSocket && dimmNum%2 != 0) || (dimmNum > channelsPerSocket && dimmNum%2 == 0) {
			slot = 0
		} else {
			slot = 1
		}
		dimm[DerivedSlotIdx] = fmt.Sprintf("%d", slot)
	}
	return
}

/* as seen on 2 socket Dell systems...
* "Bank Locator" for all DIMMs is "Not Specified" and "Locator" is A1-A12 and B1-B12.
* A1 and A7 are channel 0, A2 and A8 are channel 1, etc.
 */
func deriveDIMMInfoDell(dimms *[][]string, numSockets int, channelsPerSocket int) (err error) {
	re := regexp.MustCompile(`([ABCD])([1-9]\d*)`)
	for _, dimm := range *dimms {
		if !strings.Contains(dimm[BankLocatorIdx], "Not Specified") {
			err = fmt.Errorf("doesn't conform to expected Dell Bank Locator format")
			return
		}
		match := re.FindStringSubmatch(dimm[LocatorIdx])
		if match == nil {
			err = fmt.Errorf("doesn't conform to expected Dell Locator format")
			return
		}
		alpha := match[1]
		var numeric int
		numeric, err = strconv.Atoi(match[2])
		if err != nil {
			err = fmt.Errorf("doesn't conform to expected Dell Locator numeric format")
			return
		}
		// Socket
		// A = 0, B = 1, C = 2, D = 3
		dimm[DerivedSocketIdx] = fmt.Sprintf("%d", int(alpha[0])-int('A'))
		// Slot
		if numeric <= channelsPerSocket {
			dimm[DerivedSlotIdx] = "0"
		} else {
			dimm[DerivedSlotIdx] = "1"
		}
		// Channel
		if numeric <= channelsPerSocket {
			dimm[DerivedChannelIdx] = fmt.Sprintf("%d", numeric-1)
		} else {
			dimm[DerivedChannelIdx] = fmt.Sprintf("%d", numeric-(channelsPerSocket+1))
		}
	}
	return
}

/* as seen on Amazon EC2 bare-metal systems...
 * 		BANK LOC		LOCATOR
 * c5.metal
 * 		NODE 1			DIMM_A0
 * 		NODE 1			DIMM_A1
 * 		...
 * 		NODE 2			DIMM_G0
 * 		NODE 2			DIMM_G1
 * 		...								<<< there's no 'I'
 * 		NODE 2			DIMM_M0
 * 		NODE 2			DIMM_M1
 *
 * c6i.metal
 * 		NODE 0			CPU0 Channel0 DIMM0
 * 		NODE 0			CPU0 Channel0 DIMM1
 * 		NODE 0			CPU0 Channel1 DIMM0
 * 		NODE 0			CPU0 Channel1 DIMM1
 * 		...
 * 		NODE 7			CPU1 Channel7 DIMM0
 * 		NODE 7			CPU1 Channel7 DIMM1
 */
func deriveDIMMInfoEC2(dimms *[][]string, numSockets int, channelsPerSocket int) (err error) {
	c5bankLocRe := regexp.MustCompile(`NODE\s+([1-9])`)
	c5locRe := regexp.MustCompile(`DIMM_(.)(.)`)
	c6ibankLocRe := regexp.MustCompile(`NODE\s+(\d+)`)
	c6ilocRe := regexp.MustCompile(`CPU(\d+)\s+Channel(\d+)\s+DIMM(\d+)`)
	for _, dimm := range *dimms {
		// try c5.metal format
		bankLocMatch := c5bankLocRe.FindStringSubmatch(dimm[BankLocatorIdx])
		locMatch := c5locRe.FindStringSubmatch(dimm[LocatorIdx])
		if locMatch != nil && bankLocMatch != nil {
			var socket, channel, slot int
			socket, _ = strconv.Atoi(bankLocMatch[1])
			socket -= 1
			if int(locMatch[1][0]) < int('I') { // there is no 'I'
				channel = (int(locMatch[1][0]) - int('A')) % channelsPerSocket
			} else if int(locMatch[1][0]) > int('I') {
				channel = (int(locMatch[1][0]) - int('B')) % channelsPerSocket
			} else {
				err = fmt.Errorf("doesn't conform to expected EC2 format")
				return
			}
			slot, _ = strconv.Atoi(locMatch[2])
			dimm[DerivedSocketIdx] = fmt.Sprintf("%d", socket)
			dimm[DerivedChannelIdx] = fmt.Sprintf("%d", channel)
			dimm[DerivedSlotIdx] = fmt.Sprintf("%d", slot)
			continue
		}
		// try c6i.metal format
		bankLocMatch = c6ibankLocRe.FindStringSubmatch(dimm[BankLocatorIdx])
		locMatch = c6ilocRe.FindStringSubmatch(dimm[LocatorIdx])
		if locMatch != nil && bankLocMatch != nil {
			var socket, channel, slot int
			socket, _ = strconv.Atoi(locMatch[1])
			channel, _ = strconv.Atoi(locMatch[2])
			slot, _ = strconv.Atoi(locMatch[3])
			dimm[DerivedSocketIdx] = fmt.Sprintf("%d", socket)
			dimm[DerivedChannelIdx] = fmt.Sprintf("%d", channel)
			dimm[DerivedSlotIdx] = fmt.Sprintf("%d", slot)
			continue
		}
		err = fmt.Errorf("doesn't conform to expected EC2 format")
		return
	}
	return
}

/* "1,3-5,8" -> [1,3,4,5,8] */
func expandCPUList(cpuList string) (cpus []int) {
	if cpuList != "" {
		tokens := strings.Split(cpuList, ",")
		for _, token := range tokens {
			if strings.Contains(token, "-") {
				subTokens := strings.Split(token, "-")
				if len(subTokens) == 2 {
					begin, errA := strconv.Atoi(subTokens[0])
					end, errB := strconv.Atoi(subTokens[1])
					if errA != nil || errB != nil {
						log.Printf("Failed to parse CPU affinity")
						return
					}
					for i := begin; i <= end; i++ {
						cpus = append(cpus, i)
					}
				}
			} else {
				cpu, err := strconv.Atoi(token)
				if err != nil {
					log.Printf("CPU isn't integer!")
					return
				}
				cpus = append(cpus, cpu)
			}
		}
	}
	return
}

func getCPUAveragePercentage(table *Table, sourceIndex int, fieldName string, inverse bool) (average string) {
	hostValues := &table.AllHostValues[sourceIndex]
	sum, _, err := getSumOfFields(hostValues, []string{fieldName}, "Time")
	if err != nil {
		log.Printf("failed to get sum of fields for CPU metrics: %v", err)
		return
	}
	if len(hostValues.Values) > 0 {
		averageFloat := sum / float64(len(hostValues.Values))
		if inverse {
			averageFloat = 100.0 - averageFloat
		}
		average = fmt.Sprintf("%0.2f", averageFloat)
	}
	return
}

func getMetricAverage(table *Table, sourceIndex int, fieldNames []string, separatorFieldName string) (average string) {
	hostValues := &table.AllHostValues[sourceIndex]
	sum, seps, err := getSumOfFields(hostValues, fieldNames, separatorFieldName)
	if err != nil {
		log.Printf("failed to get sum of fields for IO metrics: %v", err)
		return
	}
	if len(fieldNames) > 0 && seps > 0 {
		averageFloat := sum / float64(seps/len(fieldNames))
		average = fmt.Sprintf("%0.2f", averageFloat)
	}
	return
}

func getSumOfFields(hostValues *HostValues, fieldNames []string, separatorFieldName string) (sum float64, numSeparators int, err error) {
	prevSeparator := ""
	separatorIdx, err := findValueIndex(hostValues, separatorFieldName)
	if err != nil {
		return
	}
	for _, fieldName := range fieldNames {
		var fieldIdx int
		fieldIdx, err = findValueIndex(hostValues, fieldName)
		if err != nil {
			return
		}
		for _, entry := range hostValues.Values {
			valueStr := entry[fieldIdx]
			var valueFloat float64
			valueFloat, err = strconv.ParseFloat(valueStr, 64)
			if err != nil {
				return
			}
			separator := entry[separatorIdx]
			if separator != prevSeparator {
				numSeparators++
				prevSeparator = separator
			}
			sum += valueFloat
		}
	}
	return
}

func getInsightsRules() (rules []byte, err error) {
	rules, err = resources.ReadFile("resources/insights.grl")
	if err != nil {
		err = fmt.Errorf("failed to read insights.grl, %v", err)
		return
	}
	return
}

func getMicroArchitectureExt(model, sockets string, capid4 string, devices string) (uarch string, err error) {
	capid4Int, err := strconv.ParseInt(capid4, 16, 64)
	if err != nil {
		return
	}
	bits := (capid4Int >> 6) & 0b11
	if model == "143" { // SPR
		if bits == 3 {
			uarch = "SPR_XCC"
		} else if bits == 1 {
			uarch = "SPR_MCC"
		} else {
			uarch = "SPR_Unknown"
		}
	} else if model == "207" { /*EMR*/
		if bits == 3 {
			uarch = "EMR_XCC"
		} else if bits == 1 {
			uarch = "EMR_MCC"
		} else {
			uarch = "EMR_Unknown"
		}
	} else if model == "173" { /*GNR*/
		var devCount int
		devCount, err = strconv.Atoi(devices)
		if err != nil {
			return
		}
		var socketsCount int
		socketsCount, err = strconv.Atoi(sockets)
		if socketsCount == 0 || err != nil {
			return
		}
		ratio := devCount / socketsCount
		if ratio == 3 {
			uarch = "GNR_X1"
		} else if ratio == 4 {
			uarch = "GNR_X2"
		} else if ratio == 5 {
			uarch = "GNR_X3"
		} else {
			uarch = "GNR_Unknown"
		}
	}
	return
}
