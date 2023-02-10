/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
/* reports are made up of tables, the Table data structure and some helpful functions are defined here */

package main

import "fmt"

// HostValues ... a single host's table values
type HostValues struct {
	Name       string // host's name
	ValueNames []string
	Values     [][]string //[record][field]
}

type TableCategory int

const (
	System TableCategory = iota
	Software
	CPU
	Power
	Memory
	Network
	Storage
	GPU
	CXL
	Security
	Status
	NoCategory
)

var TableCategoryLabels = []string{"System", "Software", "CPU", "Power", "Memory", "Network", "Storage", "GPU", "CXL", "Security", "Status"}

// Table ... all hosts
type Table struct {
	Name          string // table's name
	Category      TableCategory
	AllHostValues []HostValues
}

func (t *Table) getValue(sourceIdx int, valueName string) (value string, err error) {
	valueIndex, err := findValueIndex(&t.AllHostValues[sourceIdx], valueName)
	if err != nil {
		return
	}
	if len(t.AllHostValues[sourceIdx].Values) == 0 {
		err = fmt.Errorf("no values in table for this host")
		return
	}
	value = t.AllHostValues[sourceIdx].Values[0][valueIndex]
	return
}

// findValueIndex returns the index of the specified value name or error
func findValueIndex(srcHv *HostValues, valueName string) (index int, err error) {
	for i, valName := range srcHv.ValueNames {
		if valName == valueName {
			index = i
			return
		}
	}
	err = fmt.Errorf("value name not found: %s", valueName)
	return
}

// copy specified values from one table to another
func copyValues(src *Table, dst *Table, valueNames []string) {
	for _, srcHv := range src.AllHostValues {
		dstHv := HostValues{
			Name:       "",
			ValueNames: valueNames,
			Values:     [][]string{},
		}
		var valueIndices []int
		for _, valueName := range valueNames {
			idx, err := findValueIndex(&srcHv, valueName)
			if err == nil {
				valueIndices = append(valueIndices, idx)
			}
		}
		for srcRecordIndex, srcRecord := range srcHv.Values {
			dstHv.Values = append(dstHv.Values, []string{})
			for valueIndex := range valueNames {
				dstHv.Values[srcRecordIndex] = append(dstHv.Values[srcRecordIndex], srcRecord[valueIndices[valueIndex]])
			}
		}
		dst.AllHostValues = append(dst.AllHostValues, dstHv)
	}
}
