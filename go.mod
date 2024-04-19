module github.com/intel/svr-info

go 1.21

replace github.com/intel/svr-info/internal/core => ./internal/core

replace github.com/intel/svr-info/internal/cpu => ./internal/cpu

replace github.com/intel/svr-info/internal/msr => ./internal/msr

replace github.com/intel/svr-info/internal/progress => ./internal/progress

replace github.com/intel/svr-info/internal/target => ./internal/target

replace github.com/intel/svr-info/internal/commandfile => ./internal/commandfile

replace github.com/intel/svr-info/internal/util => ./internal/util

require (
	github.com/Knetic/govaluate v3.0.0+incompatible
	github.com/deckarep/golang-set/v2 v2.6.0
	github.com/google/go-cmp v0.6.0
	github.com/hyperjumptech/grule-rule-engine v1.15.0
	github.com/intel/svr-info/internal/commandfile v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/core v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/cpu v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/msr v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/progress v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/target v0.0.0-00010101000000-000000000000
	github.com/intel/svr-info/internal/util v0.0.0-00010101000000-000000000000
	github.com/xuri/excelize/v2 v2.8.0
	golang.org/x/exp v0.0.0-20240119083558-1b970713d09a
	golang.org/x/term v0.18.0
	golang.org/x/text v0.14.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20230828082145-3c4c8a2d2371 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr v1.4.10 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/cloudflare/circl v1.3.7 // indirect
	github.com/creasty/defaults v1.7.0 // indirect
	github.com/cyphar/filepath-securejoin v0.2.4 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.5.0 // indirect
	github.com/go-git/go-git/v5 v5.11.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/richardlehane/mscfb v1.0.4 // indirect
	github.com/richardlehane/msoleps v1.0.3 // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/skeema/knownhosts v1.2.1 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xuri/efp v0.0.0-20230802181842-ad255f2331ca // indirect
	github.com/xuri/nfp v0.0.0-20230819163627-dc951e3ffe1a // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.25.0 // indirect
	golang.org/x/crypto v0.21.0 // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/net v0.23.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	golang.org/x/tools v0.17.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
)
