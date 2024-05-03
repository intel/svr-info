#!/usr/bin/env python3

# check_events.py metric.json events.txt
# prints:
#  a list of events used in the metrics file that are not in the events file
#  a list of events in the events file but not used in the metrics file

import sys
import json

def get_event(line):
    if line != "" and not line.startswith("#"):
        if line.find("name=") >= 0:
            x = line[line.find("name=")+5:]
            x = x[x.find("'")+1:]
            x = x[0:x.find("'")]
            if x.find(":") > 0:
                x = x[0:x.find(":")]
        else:
            x = line[0:-2]
    else:
        x = None
    return x

if __name__ == "__main__":
    metrics_file = sys.argv[1]
    events_file = sys.argv[2]

    with open(metrics_file, "r") as f:
        metrics = json.load(f)
    metric_list = {}
    used_events = {} # event: count
    for m in metrics:
        metric = m["name"]
        formula = m["expression"]
        m_events = []
        start_bracket = formula.find("[")
        while start_bracket >= 0:
            end_bracket = formula.find("]")
            event = formula[start_bracket+1:end_bracket]
            if event.find(":") > 0:
                event = event[0:event.find(":")]
            if not event.startswith("const_"):
                used_events[event] = used_events.get(event, 0) + 1
            m_events.append(event)
            formula = formula[end_bracket+1:]
            start_bracket = formula.find("[")
        metric_list[metric] = m_events

    event_list = []
    with open(events_file, "r") as f:
        for line in f:
            event = get_event(line)
            if event is not None and event != "" and event_list.count(event) == 0:
                event_list.append(event)

    missing_events = [x for x in used_events.keys() if not x in event_list]
    unused_events = [x for x in event_list if not x in used_events.keys()]
    missing_events_str = "\n".join(missing_events)
    unused_events_str = "\n".join(unused_events)
    print(f"Missing events: {missing_events_str}")
    print(f"Unused events: {unused_events_str}")
