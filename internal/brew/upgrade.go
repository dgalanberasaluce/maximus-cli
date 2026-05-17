package brew

// Upgrade runs `brew bundle install` to install/upgrade all packages defined
// in the Brewfile at the given path.
// Returns the command output.
func Upgrade(brewfilePath string) (string, error) {
	return runCommand("brew", "bundle", "install", "--file", brewfilePath)
}

// UpgradePackages runs `brew upgrade` on specific packages.
// Returns the command output.
func UpgradePackages(packages []string) (string, error) {
	args := append([]string{"upgrade"}, packages...)
	return runCommand("brew", args...)
}
