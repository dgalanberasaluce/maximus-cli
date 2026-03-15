package brew

// Remove runs `brew bundle cleanup --force` to uninstall packages that are
// installed on the system but NOT listed in the Brewfile.
//
// WARNING: This operation is destructive and will remove packages permanently.
// Returns the command output.
func Remove(brewfilePath string) (string, error) {
	return runCommand("brew", "bundle", "cleanup", "--force", "--file", brewfilePath)
}
