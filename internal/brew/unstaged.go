package brew

// Unstaged returns a list of packages that are installed on the system but
// not listed in the Brewfile (dry-run equivalent of `brew bundle cleanup`).
//
// The function returns each line of output from the dry-run cleanup command,
// which lists packages that would be removed.
func Unstaged(brewfilePath string) (string, error) {
	return runCommand("brew", "bundle", "cleanup", "--file", brewfilePath)
}
