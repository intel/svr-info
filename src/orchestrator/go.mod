module intel.com/svr-info/orchestrator/v2

go 1.21

replace intel.com/svr-info/pkg/core => ../pkg/core

replace intel.com/svr-info/pkg/cpu => ../pkg/cpu

replace intel.com/svr-info/pkg/msr => ../pkg/msr

replace intel.com/svr-info/pkg/progress => ../pkg/progress

replace intel.com/svr-info/pkg/target => ../pkg/target

replace intel.com/svr-info/pkg/commandfile => ../pkg/commandfile

require (
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d
	golang.org/x/term v0.15.0
	gopkg.in/yaml.v2 v2.4.0
	intel.com/svr-info/pkg/commandfile v0.0.0-00010101000000-000000000000
	intel.com/svr-info/pkg/core v0.0.0-00010101000000-000000000000
	intel.com/svr-info/pkg/progress v0.0.0-00010101000000-000000000000
	intel.com/svr-info/pkg/target v0.0.0-00010101000000-000000000000
)

require (
	github.com/creasty/defaults v1.7.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	golang.org/x/sys v0.15.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
