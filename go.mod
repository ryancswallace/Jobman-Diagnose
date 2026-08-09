module github.com/ryancswallace/jobman-diagnose

go 1.26

require (
	github.com/ryancswallace/jobman v0.0.0
	go.yaml.in/yaml/v3 v3.0.5
)

// The evidence package is unreleased while the two repositories are developed
// together. Replace this with the first Jobman release containing diagnostic
// schema v1 before publishing a companion release.
replace github.com/ryancswallace/jobman => ../jobman
