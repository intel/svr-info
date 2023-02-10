# Collector
This program reads and executes commands (bash script) defined in YAML format and outputs results in JSON format.

Note: collector.log and collector.pid file will also be created at runtime in the same directory as the binary.

## Running
$ [SUDO_PASSWORD=*password*] ./collector < run_these_commands.yaml > command_results.json

## Input
Collector's input format (YAML) description.

Root level keys:
- Arguments
- Commands

Arguments are in key:value format. Valid keys are:
- **name**: typically the host name, will be the top-level key in the JSON output
- **bin_path**: path to dependencies of the commands, will be inserted at the beginning of the PATH environment variable
- **command_timeout**: maximum time to wait for any one command

Commands are list items.

Required command attributes:
- **label**: unique name for the command
- **command**: bash command or script

Optional command attributes:
- **superuser**: bool indicates need for elevated privilege (default: false)
- **run**: bool indicates if command will be run (default: true)
- **modprobe**: comma separated list of kernel modules required to run command
- **parallel**: bool indicates if command can be run in parallel with other commands (default: false)

Example:
```yaml
arguments:
    name: my_server
    bin_path: /home/me/my_bin_files
    command_timeout: 300
commands:
  - label: date
    command: date -u
  - label: hardware info
    command: lshw -businfo
    superuser: true
  - label: cpuid -1
    command: cpuid -1
    modprobe: cpuid
```

## Output
Collector's output format:
```JSON
{
  hostname {
    [
      {'label': "command label",
       'command': "full command",
       'superuser': "true" or "false",
       'stdout': "command output",
       'stderr': "",
       'exitstatus': "0"
      },
      ...
    ]
  }
}
```
