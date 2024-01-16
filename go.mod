module github.com/intel/svr-info

go 1.21.5

require (
	github.com/Knetic/govaluate v3.0.0+incompatible
	github.com/deckarep/golang-set/v2 v2.6.0
	github.com/google/go-cmp v0.6.0
	github.com/hyperjumptech/grule-rule-engine v1.14.1
	github.com/intel/svr-info/internal/commandfile v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/core v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/cpu v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/msr v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/progress v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/target v0.0.0-00010101000000-000000000000
	github.com/xuri/excelize/v2 v2.8.1-0.20231031165218-b41a6cc3cd37
	golang.org/x/exp v0.0.0-20240112132812-db7319d0e0e3
	golang.org/x/term v0.16.0
	golang.org/x/text v0.14.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr v1.4.10 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/creasty/defaults v1.7.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/google/uuid v1.5.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/richardlehane/mscfb v1.0.4 // indirect
	github.com/richardlehane/msoleps v1.0.3 // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/src-d/gcfg v1.4.0 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xuri/efp v0.0.0-20231025114914-d1ff6096ae53 // indirect
	github.com/xuri/nfp v0.0.0-20230919160717-d98342af3f05 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/crypto v0.18.0 // indirect
	golang.org/x/net v0.20.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	gopkg.in/src-d/go-billy.v4 v4.3.2 // indirect
	gopkg.in/src-d/go-git.v4 v4.13.1 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
)

replace github.com/intel/svr-info/internal/core => ./internal/core

replace github.com/intel/svr-info/internal/cpu => ./internal/cpu

replace github.com/intel/svr-info/internal/msr => ./internal/msr

replace github.com/intel/svr-info/internal/progress => ./internal/progress

replace github.com/intel/svr-info/internal/target => ./internal/target

replace github.com/intel/svr-info/internal/commandfile => ./internal/commandfile
