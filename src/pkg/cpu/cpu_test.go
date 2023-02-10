/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package cpu

import (
	"fmt"
	"testing"
)

func TestFindCPU(t *testing.T) {
	cpu, err := NewCPU([]string{"cpu_test.yaml", "cpu_test_2.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	// should fail
	_, err = cpu.GetMicroArchitecture("0", "0", "0")
	if err == nil {
		t.Fatal(err)
	}
	// should succeed
	uarch, err := cpu.GetMicroArchitecture("6", "85", "4") //SKX
	if err != nil {
		t.Fatal(err)
	}
	if uarch != "SKX" {
		t.Fatal(fmt.Errorf("Found the wrong CPU"))
	}
	// should succeed
	uarch, err = cpu.GetMicroArchitecture("6", "85", "7") //CLX
	if err != nil {
		t.Fatal(err)
	}
	if uarch != "CLX" {
		t.Fatal(fmt.Errorf("Found the wrong CPU"))
	}
	uarch, err = cpu.GetMicroArchitecture("6", "85", "6") //CLX
	if err != nil {
		t.Fatal(err)
	}
	if uarch != "CLX" {
		t.Fatal(fmt.Errorf("Found the wrong CPU"))
	}
	// should succeed
	uarch, err = cpu.GetMicroArchitecture("6", "108", "0") //ICX
	if err != nil {
		t.Fatal(err)
	}
	if uarch != "ICX" {
		t.Fatal(fmt.Errorf("Found the wrong CPU"))
	}
	uarch, err = cpu.GetMicroArchitecture("6", "71", "0") //BDW
	if err != nil {
		t.Fatal(err)
	}
	if uarch != "BDW" {
		t.Fatal(fmt.Errorf("Found the wrong CPU"))
	}

	// test the regex on model for HSW
	channels, err := cpu.GetMemoryChannels("6", "50", "0") //HSW
	if err != nil {
		t.Fatal(err)
	}
	if channels != 2 {
		t.Fatal(fmt.Errorf("Found the wrong CPU"))
	}
	uarch, err = cpu.GetMicroArchitecture("6", "69", "99") //HSW
	if err != nil {
		t.Fatal(err)
	}
	if uarch != "HSW" {
		t.Fatal(fmt.Errorf("Found the wrong CPU"))
	}
	uarch, err = cpu.GetMicroArchitecture("6", "70", "") //HSW
	if err != nil {
		t.Fatal(err)
	}
	if uarch != "HSW" {
		t.Fatal(fmt.Errorf("Found the wrong CPU"))
	}
	uarch, err = cpu.GetMicroArchitecture("0", "1", "r3p1") //
	if err != nil {
		t.Fatal(err)
	}
	if uarch != "Neoverse N1" {
		t.Fatal(fmt.Errorf("Found the wrong CPU"))
	}
}
