#!/usr/bin/env python3

import sys
import json


# translate perfmon metrics file to perfspect style metrics file
# inFile - perfmon_metrics.json file
# outFile - perfspect.json style file
def translate_perfmon_metrics_to_perfspect(inFile, outFile):
    varMap = {
        "[INST_RETIRED.ANY]": "[instructions]",
        "[CPU_CLK_UNHALTED.THREAD]": "[cpu-cycles]",
        "[CPU_CLK_UNHALTED.REF]": "[ref-cycles]",
        "[CPU_CLK_UNHALTED.REF_TSC]": "[ref-cycles]",
        "DURATIONTIMEINSECONDS": "1",
        "[DURATIONTIMEINMILLISECONDS]": "1000",
        "[TOPDOWN.SLOTS:perf_metrics]": "[TOPDOWN.SLOTS]",
        "[OFFCORE_REQUESTS_OUTSTANDING.ALL_DATA_RD:c4]": "[OFFCORE_REQUESTS_OUTSTANDING.DATA_RD:c4]",
    }

    with open(inFile, "r") as f:
        mf = json.load(f)

    if mf.get("Metrics") is None:
        print(f"ERROR: No metrics were found in {inFile}")
        return

    print(f"Metrics in {inFile}: {len(mf['Metrics'])}")
    vars = {}
    result = []
    for m in mf["Metrics"]:
        vars.clear()
        metric = {}
        metricName = m["LegacyName"]
        for e in m["Events"]:
            vars[e["Alias"]] = e["Name"]
        for c in m["Constants"]:
            vars[c["Alias"]] = c["Name"]
        formula = m["Formula"]
        newFormula = ""
        i = 0
        while i < len(formula):
            if formula[i].isalpha() or formula[i] == "_":
                x = formula[i]
                k = i + 1
                while k < len(formula) and (formula[k].isalpha() or formula[k] == "_"):
                    x += formula[k]
                    k += 1
                if vars.get(x) is not None:
                    newFormula = newFormula + "[" + vars[x] + "]"
                else:
                    newFormula = newFormula + formula[i:k]
                i = k
            else:
                newFormula += formula[i]
                i += 1
        metric["name"] = metricName
        for v in varMap:
            newFormula = newFormula.replace(v, varMap[v])
        metric["expression"] = newFormula
        result.append(metric)

    print(f"Generated metrics: {len(result)}")
    json_object = json.dumps(result, indent=4)
    with open(outFile, "w") as outfile:
        outfile.write(json_object)


# search metric mName in a list of metrics mList
# mList is dictionary
def find_metric(mList, mName):
    for m in mList:
        if m["name"] == mName:
            return m
    return None


# generate metrics file, based on perfmon metrics (allFile)
# usedFile - current perfspect metrics file
# if metric from usedFile not found in perfmon list,
# add it to filteredFile as is and add field "origin"="perfspect"
def generate_final_metrics_list(allFile, usedFile, filteredFile):
    with open(allFile, "r") as f:
        allMetrics = json.load(f)
    with open(usedFile, "r") as f:
        usedMetrics = json.load(f)
    result = []
    for m in usedMetrics:
        found = find_metric(allMetrics, m["name"])
        if found is not None:
            result.append(found)
        else:
            m["origin"] = "perfspect"
            result.append(m)

    print(f"PerfSpect metrics: {len(result)}")
    json_object = json.dumps(result, indent=4)
    with open(filteredFile, "w") as outfile:
        outfile.write(json_object)


# arg1 - perfmon json file
# arg2 - all perfmon metrics in "perfspect" style
# arg3 - old/current perfspect metrics file
# arg4 - final/new perfspect metrics file
if __name__ == "__main__":
    if len(sys.argv) < 3:
        sys.exit(1)

    translate_perfmon_metrics_to_perfspect(sys.argv[1], sys.argv[2])

    if len(sys.argv) == 5:
        generate_final_metrics_list(sys.argv[2], sys.argv[3], sys.argv[4])
