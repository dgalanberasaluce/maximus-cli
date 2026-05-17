package home

import "strings"

// ToolMap maps common dotfile/dotfolder names (without path, e.g. ".gitconfig" or ".config") to their associated tool.
// For files or folders in $HOME, we will match against their base name.
var ToolMap = map[string]string{
	".gitconfig":      "Git",
	".gitignore":      "Git",
	".gitattributes":  "Git",
	".gitignore_global":"Git",
	".git":            "Git",
	".ssh":            "SSH",
	".bashrc":         "Bash",
	".bash_profile":   "Bash",
	".bash_history":   "Bash",
	".bash_logout":    "Bash",
	".inputrc":        "Readline",
	".zshrc":          "Zsh",
	".zprofile":       "Zsh",
	".zsh_history":    "Zsh",
	".zshenv":         "Zsh",
	".zlogout":        "Zsh",
	".oh-my-zsh":      "Oh My Zsh",
	".p10k.zsh":       "Powerlevel10k",
	".docker":         "Docker",
	".npm":            "npm",
	".npmrc":          "npm",
	".nvm":            "NVM",
	".node-gyp":       "Node-gyp",
	".yarn":           "Yarn",
	".yarnrc":         "Yarn",
	".config":         "XDG Config",
	".local":          "XDG Local",
	".cache":          "XDG Cache",
	".cargo":          "Rust/Cargo",
	".rustup":         "Rustup",
	".pyenv":          "Pyenv",
	".python_history":"Python",
	".conda":          "Conda",
	".pip":            "Pip",
	".aws":            "AWS CLI",
	".kube":           "Kubernetes",
	".minikube":       "Minikube",
	".docker-compose":"Docker Compose",
	".vim":            "Vim",
	".vimrc":          "Vim",
	".viminfo":        "Vim",
	".vscode":         "VS Code",
	".cursor":         "Cursor",
	".gnupg":          "GnuPG",
	".ssh-agent":      "SSH Agent",
	".colima":         "Colima",
	".orbstack":       "OrbStack",
	".ansible":        "Ansible",
	".terraform.d":    "Terraform",
	".gcloud":         "Google Cloud",
	".gem":            "RubyGems",
	".bundle":         "Bundler",
	".rvm":            "RVM",
	".rbenv":          "rbenv",
	".tmux.conf":      "tmux",
	".tmux":           "tmux",
	".sdkman":         "SDKMAN!",
	".gradle":         "Gradle",
	".m2":             "Maven",
	".curlrc":         "curl",
	".wgetrc":         "wget",
	".editorconfig":   "EditorConfig",
	".profile":        "System Profile",
	".hushlogin":      "System Login",
	".CFUserTextEncoding": "macOS System",
	".DS_Store":       "macOS Finder",
	".Trash":          "macOS Trash",
	".cups":           "CUPS Printing",
}

// InferTool matches a file/folder name against ToolMap.
// If it has a prefix like .config/ or similar, we handle it if needed.
// Since we only scan direct children of $HOME, name will just be the basename (e.g. ".gitconfig").
func InferTool(name string) string {
	if tool, found := ToolMap[name]; found {
		return tool
	}
	// Fallbacks / prefix-based heuristic
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, ".git"):
		return "Git"
	case strings.HasPrefix(lower, ".bash"):
		return "Bash"
	case strings.HasPrefix(lower, ".zsh"):
		return "Zsh"
	case strings.HasPrefix(lower, ".vim"):
		return "Vim"
	case strings.HasPrefix(lower, ".docker"):
		return "Docker"
	case strings.HasPrefix(lower, ".npm"):
		return "npm"
	case strings.HasPrefix(lower, ".yarn"):
		return "Yarn"
	case strings.HasPrefix(lower, ".cargo") || strings.HasPrefix(lower, ".rust"):
		return "Rust"
	case strings.HasPrefix(lower, ".aws"):
		return "AWS CLI"
	case strings.HasPrefix(lower, ".kube"):
		return "Kubernetes"
	case strings.HasPrefix(lower, ".terraform"):
		return "Terraform"
	case strings.HasPrefix(lower, ".gcloud"):
		return "Google Cloud"
	case strings.HasPrefix(lower, ".gem") || strings.HasPrefix(lower, ".bundle"):
		return "Ruby"
	case strings.HasPrefix(lower, ".python") || strings.HasPrefix(lower, ".py"):
		return "Python"
	case strings.HasPrefix(lower, ".tmux"):
		return "tmux"
	case strings.HasPrefix(lower, ".gradle") || strings.HasPrefix(lower, ".maven") || strings.HasPrefix(lower, ".m2"):
		return "Java"
	}
	return "—"
}
