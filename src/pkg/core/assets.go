/*
Package core includes internal shared code.
*/
/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package core

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	Orchestrator int = iota
	Amd64Collector
	Reporter
	Amd64Deps
	ReportsYaml
	MegadataYaml
	Sshpass
	ReferenceData
	HTMLReportTemplate
	CPUsYaml
	GPUsYaml
	AcceleratorsYaml
	Insights
	Arm64Collector
	Arm64Deps
	Burn
	numPaths // keep this at the end
)

type Assets [numPaths]string

func NewAssets() (assets *Assets, err error) {
	assets = &Assets{}

	assets[Orchestrator], err = FindAsset("orchestrator")
	if err != nil {
		return
	}
	assets[Reporter], err = FindAsset("reporter")
	if err != nil {
		return
	}
	assets[Amd64Collector], err = FindAsset("collector")
	if err != nil {
		return
	}
	assets[Arm64Collector], err = FindAsset("collector_arm64")
	if err != nil {
		return
	}
	assets[ReportsYaml], err = FindAsset("collector_reports.yaml.tmpl")
	if err != nil {
		return
	}
	assets[MegadataYaml], err = FindAsset("collector_megadata.yaml.tmpl")
	if err != nil {
		return
	}
	assets[Amd64Deps], err = FindAsset("collector_deps_amd64.tgz")
	if err != nil {
		return
	}
	assets[Arm64Deps], err = FindAsset("collector_deps_arm64.tgz")
	if err != nil {
		return
	}
	assets[Sshpass], err = FindAsset("sshpass")
	if err != nil {
		return
	}
	assets[ReferenceData], err = FindAsset("reference.yaml")
	if err != nil {
		return
	}
	assets[HTMLReportTemplate], err = FindAsset("report.html.tmpl")
	if err != nil {
		return
	}
	assets[CPUsYaml], err = FindAsset("cpus.yaml")
	if err != nil {
		return
	}
	assets[CPUsYaml], err = FindAsset("gpus.yaml")
	if err != nil {
		return
	}
	assets[CPUsYaml], err = FindAsset("accelerators.yaml")
	if err != nil {
		return
	}
	assets[Insights], err = FindAsset("insights.grl")
	if err != nil {
		return
	}
	assets[Burn], err = FindAsset("burn")
	if err != nil {
		return
	}
	return
}

func (assets *Assets) Verify() (match []string, nomatch []string, nodata []string, err error) {
	sums := make(map[string]string) // filename to md5 map
	// build map from file containing all md5 sums
	sumsFilepath, err := FindAsset("sums.md5")
	if err != nil {
		return
	}
	sumsFile, err := os.Open(sumsFilepath)
	if err != nil {
		return
	}
	defer sumsFile.Close()
	scanner := bufio.NewScanner(sumsFile)
	for scanner.Scan() {
		line := scanner.Text()
		re := regexp.MustCompile(`(\w+)\s+([\w./-]+)`)
		match := re.FindStringSubmatch(line)
		if len(match) == 3 {
			sums[path.Base(match[2])] = match[1]
		}
	}

	// find each asset in map, verify md5
	for _, asset := range assets {
		if asset == "" {
			continue
		}
		// calculate md5 of asset
		var assetFile *os.File
		assetFile, err = os.Open(asset)
		if err != nil {
			return
		}
		defer assetFile.Close()
		hash := md5.New()
		_, err = io.Copy(hash, assetFile)
		if err != nil {
			return
		}
		assetSum := fmt.Sprintf("%x", hash.Sum(nil))
		// categorize result
		if sum, ok := sums[path.Base(asset)]; ok {
			if sum != assetSum {
				nomatch = append(nomatch, asset)
			} else {
				match = append(match, asset)
			}
		} else {
			nodata = append(nodata, asset)
		}
	}
	return
}

func FindAsset(assetName string) (assetPath string, err error) {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	searchDirs := []string{
		/* for use during deployment */
		filepath.Join(exeDir, "..", "config"),
		filepath.Join(exeDir, "..", "tools"),
		/* for use during development */
		filepath.Join(exeDir, "..", "..", "config"),
		filepath.Join(exeDir, "..", "orchestrator"),
		filepath.Join(exeDir, "..", "collector"),
		filepath.Join(exeDir, "..", "reporter"),
		filepath.Join(exeDir, "..", "sshpass"),
		filepath.Join(exeDir, "..", "burn"),
	}
	for _, dir := range searchDirs {
		if dir == "." {
			assetPath = strings.Join([]string{dir, assetName}, string(filepath.Separator))
		} else {
			assetPath = filepath.Join(dir, assetName)
		}
		_, err = os.Stat(assetPath)
		if err == nil {
			return
		}
	}
	if err != nil {
		err = fmt.Errorf("could not find required asset (%s) relative to executable (%s)", assetName, exePath)
	}
	return
}
