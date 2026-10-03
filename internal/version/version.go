package version

// Version is the Atlas CLI version string.
// Override at build time with:
//
//	go build -ldflags "-X github.com/eshmun84/Atlas-CLI/internal/version.Version=x.y.z"
var Version = "0.1.0"
