/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package main

import "time"

func GetTSCStart() uint64

func GetTSCEnd() uint64

func GetTSCFreqMHz() (freqMHz int) {
	start := GetTSCStart()
	time.Sleep(time.Millisecond * 1000)
	end := GetTSCEnd()
	freqMHz = int(end-start) / 1000000
	return
}
