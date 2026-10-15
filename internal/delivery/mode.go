package delivery

// Mode is the Atlas source-control platform preference (historical YAML key
// family: source_control / "Delivery" UI section).
// String values match persisted .atlas/config.yaml source_control.mode.
//
// This package records policy preference only. Atlas Core does not create
// remotes, push, open PRs, merge, tag, or publish repository releases.
// Those version-control remote operations belong to developer ↔ agent
// workflows (future governed skills may assist). This boundary does not
// apply to Docker, Kubernetes, deployment, CI/CD, or infrastructure.
type Mode = string

const (
	ModeNone           Mode = "none"
	ModeGitLocal       Mode = "git_local"
	ModeGitGitHub      Mode = "git_github"       // hosting preference; not a Core remote-ops engine
	ModeGitGitLab      Mode = "git_gitlab"       // reserved; not selectable in current UI
	ModeGitBitbucket   Mode = "git_bitbucket"    // reserved; not selectable in current UI
	ModeGitAzureDevOps Mode = "git_azure_devops" // reserved; not selectable in current UI
)

// SelectableModes are platforms exposed in Init/Configure today.
var SelectableModes = []Mode{
	ModeNone,
	ModeGitLocal,
	ModeGitGitHub,
}

// DefaultMode returns the Init draft seed for source_control.mode before any
// persisted document is applied.
//
//	explicit Atlas config (ApplyProjectDocument) → DefaultMode(git) → none
func DefaultMode(gitRepoDetected bool) Mode {
	if gitRepoDetected {
		return ModeGitLocal
	}
	return ModeNone
}
