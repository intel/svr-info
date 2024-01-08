/*
Package msr implements functions to read MSRs.
*/
/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package msr

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

type MSR struct {
	fileNames    []string // all msr file names
	pkgFileNames []string // one file name per package (CPU/Socket)
	fileStyleNew bool     // new style if true, old style if false
	lowBit       int      // low bit in requested bit range
	highBit      int      // high bit in requested bit range
}

func NewMSR() (msr *MSR, err error) {
	msr = &MSR{
		lowBit:  0,
		highBit: 63,
	}
	err = msr.init()
	return
}

func (msr *MSR) init() (err error) {
	_, err = os.Stat("/dev/cpu/cpu0/msr")
	if err == nil {
		msr.fileStyleNew = false
		msr.fileNames, err = filepath.Glob("/dev/cpu/cpu*/msr")
		if err != nil {
			return
		}
	} else {
		_, err = os.Stat("/dev/cpu/0/msr")
		if err == nil {
			msr.fileStyleNew = true
			msr.fileNames, err = filepath.Glob("/dev/cpu/*/msr")
			if err != nil {
				return
			}
		} else {
			err = fmt.Errorf("could not find the MSR files in /dev/cpu (maybe you need a sudo modprobe msr)")
			return
		}
	}
	// determine which MSR files to use for packages
	// don't return an error if this fails, we can't get the PPID on all platforms
	var vals []uint64
	for _, fileName := range msr.fileNames {
		var val uint64
		val, e := msr.read(0x4F, fileName, 8) // use PPID reg since it will be unique per package
		if e != nil {
			return
		}
		haveIt := false
		for _, v := range vals {
			if v == val {
				haveIt = true
				break
			}
		}
		if !haveIt {
			msr.pkgFileNames = append(msr.pkgFileNames, fileName)
			vals = append(vals, val)
		}
	}
	return
}

// returns filenames for specified core and scope
// core == -1 indicates all cores
// packageScope arg ignored if specific core is requested
func (msr *MSR) getMSRFileNames(core int, packageScope bool) (fileNames []string) {
	// all cores
	if core == -1 {
		if packageScope {
			fileNames = msr.pkgFileNames
		} else {
			fileNames = msr.fileNames
		}
	} else { // specific core
		if msr.fileStyleNew {
			fileNames = append(fileNames, fmt.Sprintf("/dev/cpu/%d/msr", core))
		} else {
			fileNames = append(fileNames, fmt.Sprintf("/dev/cpu/cpu%d/msr", core))
		}
	}
	return
}

func maskUint64(highBit int, lowBit int, val uint64) (v uint64) {
	bits := highBit - lowBit + 1
	if bits < 64 {
		val >>= uint64(lowBit)
		val &= (uint64(1) << bits) - 1
	}
	v = val
	return
}

func (msr *MSR) read(reg uint64, fileName string, bytes int) (val uint64, err error) {
	f, err := os.Open(fileName)
	if err != nil {
		return
	}
	defer f.Close()
	buf := make([]byte, bytes)
	read, err := f.ReadAt(buf, int64(reg))
	if err != nil {
		return
	}
	if read != bytes {
		err = fmt.Errorf("didn't read intended number of bytes")
		return
	}
	val = uint64(binary.LittleEndian.Uint64(buf))
	val = maskUint64(msr.highBit, msr.lowBit, val)
	return
}

func (msr *MSR) write(reg uint64, fileName string, bytes int, val uint64) (err error) {
	f, err := os.OpenFile(fileName, os.O_RDWR, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	buf := make([]byte, 0, 8)
	buf = binary.LittleEndian.AppendUint64(buf, val)
	written, err := f.WriteAt(buf, int64(reg))
	if err != nil {
		return
	}
	if written != bytes {
		err = fmt.Errorf("didn't write intended number of bytes: %d,%d", bytes, written)
		return
	}
	return
}

// SetBitRange filters bits for subsequent calls to Read* functions
func (msr *MSR) SetBitRange(highBit int, lowBit int) (err error) {
	if lowBit >= highBit {
		err = fmt.Errorf("lowBit must be less than highBit")
		return
	}
	if lowBit < 0 || lowBit > 62 {
		err = fmt.Errorf("lowBit must be a value between 0 and 62 (inclusive)")
		return
	}
	if highBit < 1 || highBit > 63 {
		err = fmt.Errorf("highBit must be a value between 1 and 63 (inclusive)")
		return
	}
	msr.lowBit = lowBit
	msr.highBit = highBit
	return
}

// ReadAll returns the register value for all cores
func (msr *MSR) ReadAll(reg uint64) (out []uint64, err error) {
	fileNames := msr.getMSRFileNames(-1, false)
	for _, fileName := range fileNames {
		var val uint64
		val, err = msr.read(reg, fileName, 8)
		if err != nil {
			return
		}
		out = append(out, val)
	}
	return
}

// WriteAll writes the given value to all cores at the given register offset
func (msr *MSR) WriteAll(reg uint64, val uint64) (err error) {
	fileNames := msr.getMSRFileNames(-1, false)
	for _, fileName := range fileNames {
		var val uint64
		err = msr.write(reg, fileName, 8, val)
		if err != nil {
			return
		}
	}
	return
}

// ReadOne returns the register value for the specified core
func (msr *MSR) ReadOne(reg uint64, core int) (out uint64, err error) {
	fileNames := msr.getMSRFileNames(core, false)
	if len(fileNames) != 1 {
		err = fmt.Errorf("did not find filenames for msr,core: %d, %d", reg, core)
		return
	}
	out, err = msr.read(reg, fileNames[0], 8)
	return
}

// WriteOne writes the given value to the the specified core at the given register offset
func (msr *MSR) WriteOne(reg uint64, core int, val uint64) (err error) {
	fileNames := msr.getMSRFileNames(core, false)
	if len(fileNames) != 1 {
		err = fmt.Errorf("did not find filenames for msr,core: %d, %d", reg, core)
		return
	}
	err = msr.write(reg, fileNames[0], 8, val)
	return
}

// ReadPackages returns the specified register value for each package (CPU/Socket)
func (msr *MSR) ReadPackages(reg uint64) (out []uint64, err error) {
	fileNames := msr.getMSRFileNames(-1, true)
	if len(fileNames) == 0 {
		err = fmt.Errorf("unable to identify msr files for package")
		return
	}
	for _, fileName := range fileNames {
		var val uint64
		val, err = msr.read(reg, fileName, 8)
		if err != nil {
			return
		}
		out = append(out, val)
	}
	return
}
