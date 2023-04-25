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
	"sort"
	"time"

	"golang.org/x/term"
)

var spinChars []string = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

type MultiSpinnerUpdateFunc func(string, string) error

type spinnerState struct {
	status      string
	statusIsNew bool
	spinIndex   int
}

type MultiSpinner struct {
	spinners map[string]*spinnerState
	ticker   *time.Ticker
	done     chan bool
	spinning bool
}

func NewMultiSpinner() *MultiSpinner {
	ms := MultiSpinner{}
	ms.spinners = make(map[string]*spinnerState)
	ms.done = make(chan bool)
	return &ms
}

func (ms *MultiSpinner) AddSpinner(label string) (err error) {
	if _, ok := ms.spinners[label]; ok {
		err = fmt.Errorf("spinner with label %s already exists", label)
		return
	}
	ms.spinners[label] = &spinnerState{"?", false, 0}
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
	if spinner, ok := ms.spinners[label]; ok {
		if status != spinner.status {
			spinner.statusIsNew = true
			spinner.status = status
		}
	} else {
		err = fmt.Errorf("did not find spinner with label %s", label)
		return
	}
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
	var spinnerLabels []string
	for k := range ms.spinners {
		spinnerLabels = append(spinnerLabels, k)
	}
	sort.Strings(spinnerLabels)
	for _, label := range spinnerLabels {
		spinner := ms.spinners[label]
		if !term.IsTerminal(int(os.Stderr.Fd())) && !spinner.statusIsNew {
			return
		}
		fmt.Fprintf(os.Stderr, "%-20s  %s  %-40s\n", label, spinChars[spinner.spinIndex], spinner.status)
		spinner.statusIsNew = false
		spinner.spinIndex += 1
		if spinner.spinIndex >= len(spinChars) {
			spinner.spinIndex = 0
		}
	}
	if goUp && term.IsTerminal(int(os.Stderr.Fd())) {
		for range ms.spinners {
			fmt.Fprintf(os.Stderr, "\x1b[1A")
		}
	}
}
