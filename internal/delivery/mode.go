package delivery

// Mode is the Atlas Delivery / source-control platform preference.
// String values match persisted .atlas/config.yaml source_control.mode.
type Mode = string

const (
	ModeNone           Mode = "none"
	ModeGitLocal       Mode = "git_local"
	ModeGitGitHub      Mode = "git_github"
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
