package brew

// Upgrade runs `brew bundle install` to install/upgrade all packages defined
// in the Brewfile at the given path.
// Returns the command output.
func Upgrade(brewfilePath string) (string, error) {
	return runCommand("brew", "bundle", "install", "--file", brewfilePath)
}
