module intel.com/svr-info/collector/v2

go 1.20

require (
	gopkg.in/yaml.v2 v2.4.0
	intel.com/svr-info/pkg/commandfile v0.0.0-00010101000000-000000000000
	intel.com/svr-info/pkg/core v0.0.0-00010101000000-000000000000
	intel.com/svr-info/pkg/target v0.0.0-00010101000000-000000000000
)

require (
	github.com/creasty/defaults v1.6.0 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
)

replace intel.com/svr-info/pkg/core => ../pkg/core

replace intel.com/svr-info/pkg/cpu => ../pkg/cpu

replace intel.com/svr-info/pkg/msr => ../pkg/msr

replace intel.com/svr-info/pkg/progress => ../pkg/progress

replace intel.com/svr-info/pkg/target => ../pkg/target

replace intel.com/svr-info/pkg/commandfile => ../pkg/commandfile
