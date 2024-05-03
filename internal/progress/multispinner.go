/*
Package progress provides CLI progress bar options.
*/
/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package progress

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

var spinChars []string = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

type MultiSpinnerUpdateFunc func(string, string) error

type spinnerState struct {
	label       string
	status      string
	statusIsNew bool
	spinIndex   int
}

type MultiSpinner struct {
	spinners []spinnerState
	ticker   *time.Ticker
	done     chan bool
	spinning bool
}

func NewMultiSpinner() *MultiSpinner {
	ms := MultiSpinner{}
	ms.done = make(chan bool)
	return &ms
}

func (ms *MultiSpinner) AddSpinner(label string) (err error) {
	// make sure label is unique
	for _, spinner := range ms.spinners {
		if spinner.label == label {
			err = fmt.Errorf("spinner with label %s already exists", label)
			return
		}
	}
	ms.spinners = append(ms.spinners, spinnerState{label, "?", false, 0})
	return
}

func (ms *MultiSpinner) Start() {
	ms.ticker = time.NewTicker(250 * time.Millisecond)
	ms.spinning = true
	go ms.onTick()
}

func (ms *MultiSpinner) Finish() {
	if ms.spinning {
		ms.ticker.Stop()
		ms.done <- true
		ms.draw(false)
		ms.spinning = false
	}
}

func (ms *MultiSpinner) Status(label string, status string) (err error) {
	for spinnerIdx, spinner := range ms.spinners {
		if spinner.label == label {
			if status != spinner.status {
				ms.spinners[spinnerIdx].status = status
				ms.spinners[spinnerIdx].statusIsNew = true
			}
			return
		}
	}
	err = fmt.Errorf("did not find spinner with label %s", label)
	return
}

func (ms *MultiSpinner) onTick() {
	for {
		select {
		case <-ms.done:
			return
		case <-ms.ticker.C:
			ms.draw(true)
		}
	}
}

func (ms *MultiSpinner) draw(goUp bool) {
	for i, spinner := range ms.spinners {
		if !term.IsTerminal(int(os.Stderr.Fd())) && !spinner.statusIsNew {
			continue
		}
		fmt.Fprintf(os.Stderr, "%-20s  %s  %-40s\n", spinner.label, spinChars[spinner.spinIndex], spinner.status)
		ms.spinners[i].statusIsNew = false
		ms.spinners[i].spinIndex += 1
		if ms.spinners[i].spinIndex >= len(spinChars) {
			ms.spinners[i].spinIndex = 0
		}
	}
	if goUp && term.IsTerminal(int(os.Stderr.Fd())) {
		for range ms.spinners {
			fmt.Fprintf(os.Stderr, "\x1b[1A")
		}
	}
}
