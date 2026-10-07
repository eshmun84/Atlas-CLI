package version

// Version is the Atlas CLI version string for the Alpha 2 release candidate.
// Override at build time with:
//
//	go build -ldflags "-X github.com/eshmun84/Atlas-CLI/internal/version.Version=x.y.z"
//	make build VERSION=0.1.0
//
// Do not tag, publish, or create a GitHub release without explicit human approval.
var Version = "0.1.0"
