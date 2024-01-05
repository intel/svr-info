/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */

// Adapted from https://github.com/dterei/gotsc/blob/master/tsc_amd64.s
// Copyright 2016 David Terei.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

// func GetTSCStart() uint64
TEXT ·GetTSCStart(SB),NOSPLIT,$0-8
	CPUID
	RDTSC
	SHLQ	$32, DX
	ADDQ	DX, AX
	MOVQ	AX, ret+0(FP)
	RET

// func GetTSCEnd() uint64
TEXT ·GetTSCEnd(SB),NOSPLIT,$0-8
	BYTE	$0x0F // RDTSCP
	BYTE	$0x01
	BYTE	$0xF9
	SHLQ	$32, DX
	ADDQ	DX, AX
	MOVQ	AX, ret+0(FP)
	CPUID
	RET
