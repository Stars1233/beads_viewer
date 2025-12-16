package export

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newWizardWithInput(beadsPath string, input string) *Wizard {
	w := NewWizard(beadsPath)
	w.reader = bufio.NewReader(strings.NewReader(input))
	return w
}

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestWizard_Run_LocalFlow(t *testing.T) {
	wizard := newWizardWithInput("/tmp/test", "y\nMy Site\n\n3\n/tmp/out\n")

	result, err := wizard.Run()
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Run returned nil result")
	}
	if result.DeployTarget != "local" {
		t.Fatalf("Expected DeployTarget 'local', got %q", result.DeployTarget)
	}

	config := wizard.GetConfig()
	if !config.IncludeClosed {
		t.Error("Expected IncludeClosed to be true")
	}
	if config.Title != "My Site" {
		t.Fatalf("Expected Title %q, got %q", "My Site", config.Title)
	}
	if config.Subtitle != "" {
		t.Fatalf("Expected empty Subtitle, got %q", config.Subtitle)
	}
	if config.DeployTarget != "local" {
		t.Fatalf("Expected DeployTarget %q, got %q", "local", config.DeployTarget)
	}
	if config.OutputPath != "/tmp/out" {
		t.Fatalf("Expected OutputPath %q, got %q", "/tmp/out", config.OutputPath)
	}
}

func TestWizard_collectGitHubConfig_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "myproj")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	wizard := newWizardWithInput("/tmp/test", "\ny\n\n")
	if err := wizard.collectGitHubConfig(); err != nil {
		t.Fatalf("collectGitHubConfig returned error: %v", err)
	}

	if wizard.config.RepoName != "myproj-pages" {
		t.Fatalf("Expected RepoName %q, got %q", "myproj-pages", wizard.config.RepoName)
	}
	if !wizard.config.RepoPrivate {
		t.Fatal("Expected RepoPrivate to be true")
	}
	if wizard.config.RepoDescription != "Issue tracker dashboard" {
		t.Fatalf("Expected RepoDescription %q, got %q", "Issue tracker dashboard", wizard.config.RepoDescription)
	}
}

func TestWizard_collectCloudflareConfig_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "my_project")
	bundlePath := filepath.Join(projectDir, "bv-pages")
	if err := os.MkdirAll(bundlePath, 0755); err != nil {
		t.Fatalf("Failed to create bundle dir: %v", err)
	}

	wizard := newWizardWithInput(bundlePath, "\n\n")
	if err := wizard.collectCloudflareConfig(); err != nil {
		t.Fatalf("collectCloudflareConfig returned error: %v", err)
	}

	if wizard.config.CloudflareProject != "my-project-pages" {
		t.Fatalf("Expected CloudflareProject %q, got %q", "my-project-pages", wizard.config.CloudflareProject)
	}
	if wizard.config.CloudflareBranch != "main" {
		t.Fatalf("Expected CloudflareBranch %q, got %q", "main", wizard.config.CloudflareBranch)
	}
}

func TestWizard_InputHelpers_DefaultsOnError(t *testing.T) {
	wizard := NewWizard("/tmp/test")
	wizard.reader = bufio.NewReader(failingReader{})

	if got := wizard.askYesNo("Question?", true); got != true {
		t.Fatalf("askYesNo expected default true, got %v", got)
	}
	if got := wizard.askYesNo("Question?", false); got != false {
		t.Fatalf("askYesNo expected default false, got %v", got)
	}
	if got := wizard.askString("Text", "default"); got != "default" {
		t.Fatalf("askString expected default %q, got %q", "default", got)
	}
	if got := wizard.askChoice("Choice", []string{"1", "2"}, "2"); got != "2" {
		t.Fatalf("askChoice expected default %q, got %q", "2", got)
	}
}

func TestWizard_OfferPreview_Skip(t *testing.T) {
	wizard := newWizardWithInput("/tmp/test", "n\n")
	wizard.bundlePath = "/tmp/bundle"

	next, err := wizard.OfferPreview()
	if err != nil {
		t.Fatalf("OfferPreview returned error: %v", err)
	}
	if next != "deploy" {
		t.Fatalf("Expected OfferPreview to return %q, got %q", "deploy", next)
	}
}

func TestWizard_OfferPreview_PreviewSelected_MissingIndexDoesNotHang(t *testing.T) {
	// OfferPreview spawns a goroutine that attempts to open the browser after 500ms.
	// Keep PATH empty long enough so we don't actually open a browser on developer machines.
	t.Setenv("PATH", "")

	bundleDir := t.TempDir()
	wizard := newWizardWithInput("/tmp/test", "y\n\n")
	wizard.bundlePath = bundleDir

	next, err := wizard.OfferPreview()
	if err != nil {
		t.Fatalf("OfferPreview returned error: %v", err)
	}
	if next != "deploy" {
		t.Fatalf("Expected OfferPreview to return %q, got %q", "deploy", next)
	}

	// Ensure the delayed browser-open goroutine runs while PATH is still empty.
	time.Sleep(600 * time.Millisecond)
}

func TestWizard_PerformDeploy_Local(t *testing.T) {
	wizard := NewWizard("/tmp/test")
	wizard.config.DeployTarget = "local"
	wizard.bundlePath = "/tmp/bundle"

	result, err := wizard.PerformDeploy()
	if err != nil {
		t.Fatalf("PerformDeploy returned error: %v", err)
	}
	if result == nil {
		t.Fatal("PerformDeploy returned nil result")
	}
	if result.DeployTarget != "local" {
		t.Fatalf("Expected DeployTarget %q, got %q", "local", result.DeployTarget)
	}
	if result.BundlePath != "/tmp/bundle" {
		t.Fatalf("Expected BundlePath %q, got %q", "/tmp/bundle", result.BundlePath)
	}
}
