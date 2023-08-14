/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package msr

import (
	"fmt"
	"testing"
)

func TestNewMSR(t *testing.T) {
	_, err := NewMSR()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetBitRange(t *testing.T) {
	msr, err := NewMSR()
	if err != nil {
		t.Fatal(err)
	}
	err = msr.SetBitRange(0, 1)
	if err == nil {
		t.Fatal("highBit < lowBit - should have failed")
	}
	err = msr.SetBitRange(64, 0)
	if err == nil {
		t.Fatal("highBit > 63 - should have failed")
	}
	err = msr.SetBitRange(63, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = msr.SetBitRange(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = msr.SetBitRange(63, 62)
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadOne(t *testing.T) {
	msr, err := NewMSR()
	if err != nil {
		t.Fatal(err)
	}
	// this one should work
	fullVal, err := msr.ReadOne(0x1B0, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = msr.SetBitRange(4, 0)
	if err != nil {
		t.Fatal(err)
	}
	partialVal, err := msr.ReadOne(0x1B0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fullVal == partialVal {
		t.Fatal(fmt.Errorf("values should not match"))
	}
}

func TestWriteOne(t *testing.T) {
	msr, err := NewMSR()
	if err != nil {
		t.Fatal(err)
	}
	err = msr.WriteOne(0xB0, 0, 0x80000694)
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadAll(t *testing.T) {
	msr, err := NewMSR()
	if err != nil {
		t.Fatal(err)
	}
	// this one should work
	_, err = msr.ReadAll(0x1B0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadPackage(t *testing.T) {
	msr, err := NewMSR()
	if err != nil {
		t.Fatal(err)
	}
	// this one should work
	_, err = msr.ReadPackages(0x1B0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMaskUint64(t *testing.T) {
	var inputVal uint64 = 0xffffffff
	outputVal := maskUint64(63, 0, inputVal)
	if outputVal != inputVal {
		t.Fatal("should match")
	}
	outputVal = maskUint64(3, 0, inputVal)
	if outputVal != 0xf {
		t.Fatal("should match")
	}

	inputVal = 0x7857000158488
	outputVal = maskUint64(14, 0, inputVal)
	if outputVal != 0x488 {
		t.Fatal("should match")
	}
}
