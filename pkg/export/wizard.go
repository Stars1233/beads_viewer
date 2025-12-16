// Package export provides data export functionality for bv.
//
// This file implements the interactive deployment wizard for --pages flag.
// It guides users through exporting and deploying static sites to GitHub Pages.
package export

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WizardConfig holds configuration for the deployment wizard.
type WizardConfig struct {
	// Export options
	IncludeClosed bool   `json:"include_closed"`
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle,omitempty"`

	// Deployment target
	DeployTarget string `json:"deploy_target"` // "github", "cloudflare", "local"

	// GitHub options
	RepoName        string `json:"repo_name,omitempty"`
	RepoPrivate     bool   `json:"repo_private,omitempty"`
	RepoDescription string `json:"repo_description,omitempty"`

	// Cloudflare options
	CloudflareProject string `json:"cloudflare_project,omitempty"`
	CloudflareBranch  string `json:"cloudflare_branch,omitempty"`

	// Output path for bundle
	OutputPath string `json:"output_path,omitempty"`
}

// WizardResult contains the result of running the wizard.
type WizardResult struct {
	BundlePath   string
	RepoFullName string
	PagesURL     string
	DeployTarget string
	// Cloudflare-specific
	CloudflareProject string
	CloudflareURL     string
}

// Wizard handles the interactive deployment flow.
type Wizard struct {
	config     *WizardConfig
	reader     *bufio.Reader
	beadsPath  string
	bundlePath string
}

// NewWizard creates a new deployment wizard.
func NewWizard(beadsPath string) *Wizard {
	return &Wizard{
		config:    &WizardConfig{},
		reader:    bufio.NewReader(os.Stdin),
		beadsPath: beadsPath,
	}
}

// Run executes the interactive wizard flow.
func (w *Wizard) Run() (*WizardResult, error) {
	w.printBanner()

	// Step 1: Export configuration
	if err := w.collectExportOptions(); err != nil {
		return nil, err
	}

	// Step 2: Deployment target
	if err := w.collectDeployTarget(); err != nil {
		return nil, err
	}

	// Step 3: Target-specific configuration
	if err := w.collectTargetConfig(); err != nil {
		return nil, err
	}

	// Step 4: Prerequisites check
	if err := w.checkPrerequisites(); err != nil {
		return nil, err
	}

	// Step 5: Export bundle (handled externally by caller)
	// Return config for caller to perform export
	return &WizardResult{
		DeployTarget: w.config.DeployTarget,
	}, nil
}

// GetConfig returns the collected wizard configuration.
func (w *Wizard) GetConfig() *WizardConfig {
	return w.config
}

func (w *Wizard) printBanner() {
	fmt.Println("")
	fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║           bv → Static Site Deployment Wizard                     ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════════╣")
	fmt.Println("║  This wizard will:                                               ║")
	fmt.Println("║    1. Export your issues to a static HTML bundle                 ║")
	fmt.Println("║    2. Preview it locally                                         ║")
	fmt.Println("║    3. Deploy to GitHub Pages, Cloudflare Pages, or export only   ║")
	fmt.Println("║                                                                  ║")
	fmt.Println("║  Press Ctrl+C anytime to cancel                                  ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
	fmt.Println("")
}

func (w *Wizard) collectExportOptions() error {
	fmt.Println("Step 1: Export Configuration")
	fmt.Println("────────────────────────────")

	// Include closed issues?
	w.config.IncludeClosed = w.askYesNo("Include closed issues?", false)

	// Custom title
	defaultTitle := "Project Issues"
	w.config.Title = w.askString("Site title", defaultTitle)

	// Custom subtitle (optional)
	w.config.Subtitle = w.askString("Site subtitle (optional)", "")

	fmt.Println("")
	return nil
}

func (w *Wizard) collectDeployTarget() error {
	fmt.Println("Step 2: Deployment Target")
	fmt.Println("────────────────────────────")
	fmt.Println("Where do you want to deploy?")
	fmt.Println("  1. GitHub Pages (create/update repository)")
	fmt.Println("  2. Cloudflare Pages (requires wrangler CLI)")
	fmt.Println("  3. Export locally only")
	fmt.Println("")

	choice := w.askChoice("Choice", []string{"1", "2", "3"}, "1")

	switch choice {
	case "1":
		w.config.DeployTarget = "github"
	case "2":
		w.config.DeployTarget = "cloudflare"
	case "3":
		w.config.DeployTarget = "local"
	default:
		w.config.DeployTarget = "github"
	}

	fmt.Println("")
	return nil
}

func (w *Wizard) collectTargetConfig() error {
	switch w.config.DeployTarget {
	case "github":
		return w.collectGitHubConfig()
	case "cloudflare":
		return w.collectCloudflareConfig()
	case "local":
		return w.collectLocalConfig()
	}
	return nil
}

func (w *Wizard) collectGitHubConfig() error {
	fmt.Println("Step 3: GitHub Configuration")
	fmt.Println("────────────────────────────")

	// Suggest repo name based on current directory
	cwd, _ := os.Getwd()
	base := filepath.Base(cwd)
	if base == "." || base == "/" {
		base = "beads-viewer-pages"
	}
	suggestedName := base + "-pages"

	w.config.RepoName = w.askString("Repository name", suggestedName)
	w.config.RepoPrivate = w.askYesNo("Make repository private?", false)
	w.config.RepoDescription = w.askString("Repository description (optional)", "Issue tracker dashboard")

	fmt.Println("")
	return nil
}

func (w *Wizard) collectCloudflareConfig() error {
	fmt.Println("Step 3: Cloudflare Pages Configuration")
	fmt.Println("────────────────────────────")

	// Suggest project name based on bundle path or current directory
	suggestedName := SuggestProjectName(w.beadsPath)
	if suggestedName == "" {
		cwd, _ := os.Getwd()
		base := filepath.Base(cwd)
		if base == "." || base == "/" {
			base = "beads-viewer-pages"
		}
		suggestedName = base + "-pages"
	}

	w.config.CloudflareProject = w.askString("Cloudflare Pages project name", suggestedName)
	w.config.CloudflareBranch = w.askString("Branch name", "main")

	fmt.Println("")
	return nil
}

func (w *Wizard) collectLocalConfig() error {
	fmt.Println("Step 3: Local Export Configuration")
	fmt.Println("────────────────────────────")

	// Default output path
	defaultPath := "./bv-pages"
	w.config.OutputPath = w.askString("Output directory", defaultPath)

	fmt.Println("")
	return nil
}

func (w *Wizard) checkPrerequisites() error {
	fmt.Println("Step 4: Prerequisites Check")
	fmt.Println("────────────────────────────")

	switch w.config.DeployTarget {
	case "github":
		status, err := CheckGHStatus()
		if err != nil {
			return fmt.Errorf("failed to check GitHub status: %w", err)
		}

		// Check gh CLI
		if !status.Installed {
			fmt.Println("✗ gh CLI not installed")
			ShowInstallInstructions()
			return fmt.Errorf("gh CLI is required for GitHub Pages deployment")
		}
		fmt.Println("✓ gh CLI installed")

		// Check authentication
		if !status.Authenticated {
			fmt.Println("✗ gh CLI not authenticated")
			fmt.Println("")
			if w.askYesNo("Would you like to authenticate now?", true) {
				if err := AuthenticateGH(); err != nil {
					return fmt.Errorf("authentication failed: %w", err)
				}
				// Re-check
				status, _ = CheckGHStatus()
				if !status.Authenticated {
					return fmt.Errorf("authentication failed")
				}
			} else {
				return fmt.Errorf("GitHub authentication required")
			}
		}
		fmt.Printf("✓ Authenticated as %s\n", status.Username)

		// Check git config
		if !status.GitConfigured {
			fmt.Println("✗ Git identity not configured")
			fmt.Println("  Please run:")
			fmt.Println("    git config --global user.name \"Your Name\"")
			fmt.Println("    git config --global user.email \"your@email.com\"")
			return fmt.Errorf("git identity not configured")
		}
		fmt.Printf("✓ Git configured (%s <%s>)\n", status.GitName, status.GitEmail)

	case "cloudflare":
		status, err := CheckWranglerStatus()
		if err != nil {
			return fmt.Errorf("failed to check wrangler status: %w", err)
		}

		// Check wrangler CLI
		if !status.Installed {
			fmt.Println("✗ wrangler CLI not installed")
			if !status.NPMInstalled {
				fmt.Println("  npm is required to install wrangler")
				fmt.Println("  Download Node.js from: https://nodejs.org/")
				return fmt.Errorf("npm is required to install wrangler CLI")
			}
			ShowWranglerInstallInstructions()
			if w.askYesNo("Would you like to install wrangler now?", true) {
				if err := AttemptWranglerInstall(); err != nil {
					return fmt.Errorf("wrangler installation failed: %w", err)
				}
				// Re-check
				status, _ = CheckWranglerStatus()
				if !status.Installed {
					return fmt.Errorf("wrangler installation failed")
				}
			} else {
				return fmt.Errorf("wrangler CLI is required for Cloudflare Pages deployment")
			}
		}
		fmt.Println("✓ wrangler CLI installed")

		// Check authentication
		if !status.Authenticated {
			fmt.Println("✗ wrangler not authenticated")
			fmt.Println("")
			if w.askYesNo("Would you like to authenticate now?", true) {
				if err := AuthenticateWrangler(); err != nil {
					return fmt.Errorf("authentication failed: %w", err)
				}
				// Re-check
				status, _ = CheckWranglerStatus()
				if !status.Authenticated {
					return fmt.Errorf("authentication failed")
				}
			} else {
				return fmt.Errorf("Cloudflare authentication required")
			}
		}
		if status.AccountName != "" {
			fmt.Printf("✓ Authenticated (%s)\n", status.AccountName)
		} else {
			fmt.Println("✓ Authenticated with Cloudflare")
		}
	}

	fmt.Println("")
	return nil
}

// PerformExport creates the static site bundle.
// This is called by the main CLI after collecting issues.
func (w *Wizard) PerformExport(bundlePath string) error {
	w.bundlePath = bundlePath

	fmt.Println("Step 5: Export")
	fmt.Println("────────────────────────────")
	// Export logic is handled by caller
	return nil
}

// OfferPreview asks if user wants to preview and handles the preview flow.
func (w *Wizard) OfferPreview() (string, error) {
	fmt.Println("Step 6: Preview")
	fmt.Println("────────────────────────────")

	if !w.askYesNo("Preview the site before deploying?", true) {
		return "deploy", nil
	}

	fmt.Println("")
	fmt.Printf("Starting preview server for %s...\n", w.bundlePath)
	fmt.Println("Press Ctrl+C in the browser tab when done, then return here.")
	fmt.Println("")

	// Start preview server
	port, err := FindAvailablePort(PreviewPortRangeStart, PreviewPortRangeEnd)
	if err != nil {
		return "", fmt.Errorf("could not find available port: %w", err)
	}

	server := NewPreviewServer(w.bundlePath, port)

	// Open browser
	go func() {
		time.Sleep(500 * time.Millisecond)
		url := server.URL()
		OpenInBrowser(url)
	}()

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil {
			errChan <- err
		}
	}()

	// Wait for user to press enter
	fmt.Println("")
	fmt.Println("Press Enter when done previewing to continue with deployment...")
	w.reader.ReadString('\n')

	// Stop server
	server.Stop()

	fmt.Println("")
	return "deploy", nil
}

// PerformDeploy deploys the bundle to the configured target.
func (w *Wizard) PerformDeploy() (*WizardResult, error) {
	fmt.Println("Step 7: Deploy")
	fmt.Println("────────────────────────────")

	result := &WizardResult{
		BundlePath:   w.bundlePath,
		DeployTarget: w.config.DeployTarget,
	}

	switch w.config.DeployTarget {
	case "github":
		deployConfig := GitHubDeployConfig{
			RepoName:         w.config.RepoName,
			Private:          w.config.RepoPrivate,
			Description:      w.config.RepoDescription,
			BundlePath:       w.bundlePath,
			SkipConfirmation: false,
			ForceOverwrite:   false,
		}

		deployResult, err := DeployToGitHubPages(deployConfig)
		if err != nil {
			return nil, fmt.Errorf("deployment failed: %w", err)
		}

		result.RepoFullName = deployResult.RepoFullName
		result.PagesURL = deployResult.PagesURL

	case "cloudflare":
		deployConfig := CloudflareDeployConfig{
			ProjectName:      w.config.CloudflareProject,
			BundlePath:       w.bundlePath,
			Branch:           w.config.CloudflareBranch,
			SkipConfirmation: true, // Already confirmed in prerequisites
		}

		deployResult, err := DeployToCloudflarePages(deployConfig)
		if err != nil {
			return nil, fmt.Errorf("deployment failed: %w", err)
		}

		result.CloudflareProject = deployResult.ProjectName
		result.CloudflareURL = deployResult.URL
		result.PagesURL = deployResult.URL

	case "local":
		fmt.Printf("Bundle exported to: %s\n", w.bundlePath)
		result.BundlePath = w.bundlePath
	}

	return result, nil
}

// PrintSuccess prints the success message after deployment.
func (w *Wizard) PrintSuccess(result *WizardResult) {
	fmt.Println("")
	fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Deployment Complete!                          ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════════╣")

	switch result.DeployTarget {
	case "github":
		fmt.Printf("║  Repository: https://github.com/%-33s║\n", result.RepoFullName)
		fmt.Printf("║  Live site:  %-51s║\n", result.PagesURL)
		fmt.Println("║                                                                  ║")
		fmt.Println("║  Note: GitHub Pages may take 1-2 minutes to become available    ║")
	case "cloudflare":
		fmt.Printf("║  Project:    %-51s║\n", result.CloudflareProject)
		fmt.Printf("║  Live site:  %-51s║\n", result.CloudflareURL)
		fmt.Println("║                                                                  ║")
		fmt.Println("║  Cloudflare Pages deploys are typically available immediately   ║")
	case "local":
		fmt.Printf("║  Bundle: %-56s║\n", result.BundlePath)
		fmt.Println("║                                                                  ║")
		fmt.Println("║  To preview:                                                     ║")
		fmt.Printf("║    bv --preview-pages %s%-30s║\n", result.BundlePath, "")
	}

	fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
	fmt.Println("")
}

// Helper functions for user input

func (w *Wizard) askYesNo(question string, defaultYes bool) bool {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}

	fmt.Printf("%s %s: ", question, suffix)
	response, err := w.reader.ReadString('\n')
	if err != nil {
		return defaultYes
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "" {
		return defaultYes
	}

	return response == "y" || response == "yes"
}

func (w *Wizard) askString(question string, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", question, defaultValue)
	} else {
		fmt.Printf("%s: ", question)
	}

	response, err := w.reader.ReadString('\n')
	if err != nil {
		return defaultValue
	}

	response = strings.TrimSpace(response)
	if response == "" {
		return defaultValue
	}

	return response
}

func (w *Wizard) askChoice(question string, choices []string, defaultChoice string) string {
	fmt.Printf("%s [%s]: ", question, defaultChoice)
	response, err := w.reader.ReadString('\n')
	if err != nil {
		return defaultChoice
	}

	response = strings.TrimSpace(response)
	if response == "" {
		return defaultChoice
	}

	// Validate choice
	for _, c := range choices {
		if response == c {
			return response
		}
	}

	return defaultChoice
}

// WizardConfigPath returns the path to the wizard config file.
func WizardConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "bv", "pages-wizard.json")
}

// LoadWizardConfig loads previously saved wizard configuration.
func LoadWizardConfig() (*WizardConfig, error) {
	path := WizardConfigPath()
	if path == "" {
		return nil, fmt.Errorf("could not determine config path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No saved config
		}
		return nil, err
	}

	var config WizardConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveWizardConfig saves wizard configuration for future runs.
func SaveWizardConfig(config *WizardConfig) error {
	path := WizardConfigPath()
	if path == "" {
		return fmt.Errorf("could not determine config path")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
