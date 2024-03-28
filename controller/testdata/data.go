package testdata

import _ "embed"

var (
	//go:embed live-deployment.yaml
	LiveDeploymentYaml string

	//go:embed target-deployment.yaml
	TargetDeploymentYaml string

	//go:embed target-deployment-new-entries.yaml
	TargetDeploymentNewEntries string

	//go:embed diff-cache.yaml
	DiffCacheYaml string

	//go:embed live-httpproxy.yaml
	LiveHTTPProxy string

	//go:embed target-httpproxy.yaml
	TargetHTTPProxy string
)
