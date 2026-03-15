package brew

// Update runs `brew update` to refresh the local Homebrew package database.
// Returns the command output.
func Update() (string, error) {
	return runCommand("brew", "update")
}
