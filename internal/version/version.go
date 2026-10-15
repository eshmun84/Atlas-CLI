package version

// Version is the Atlas CLI source version string for the current
// development/dogfooding baseline. Version selection and bump belong to a
// later release-hardening block — do not treat this string as a frozen
// product identity.
//
// Override at build time with:
//
//	go build -ldflags "-X github.com/eshmun84/Atlas-CLI/internal/version.Version=x.y.z"
//	make build VERSION=0.1.0
//
// Do not tag, publish, or create a GitHub release without explicit human approval.
var Version = "0.1.0"
