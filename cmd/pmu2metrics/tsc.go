/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
//
// Time Stamp Counter helper functions.
//
package main

import "time"

// GetTSCFreqMHz - gets the TSC frequency
func GetTSCFreqMHz() (freqMHz int) {
	start := GetTSCStart()
	time.Sleep(time.Millisecond * 1000)
	end := GetTSCEnd()
	freqMHz = int(end-start) / 1000000
	return
}

func GetTSCStart() uint64

func GetTSCEnd() uint64
