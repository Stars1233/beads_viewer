package export

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, dir string, name string, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatalf("Chmod %s: %v", name, err)
	}
	return path
}

func TestWizard_checkPrerequisites_GitHub_HappyPathWithStubGH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stubs not supported on windows in this test")
	}

	binDir := t.TempDir()
	stateDir := t.TempDir()

	ghScript := `#!/bin/sh
set -eu
state_dir="${BV_TEST_STATE_DIR:-}"
authed_file="$state_dir/gh_authed"
case "${1-}" in
  auth)
    case "${2-}" in
      status)
        if [ -f "$authed_file" ]; then
          echo "Logged in to github.com account testuser (GitHub)"
          exit 0
        fi
        echo "You are not logged in"
        exit 1
        ;;
      login)
        mkdir -p "$state_dir"
        : > "$authed_file"
        exit 0
        ;;
    esac
    ;;
esac
exit 0
`
	writeExecutable(t, binDir, "gh", ghScript)

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("%s%c%s", binDir, os.PathListSeparator, origPath))
	t.Setenv("BV_TEST_STATE_DIR", stateDir)

	// Provide a minimal global git identity without touching the real user config.
	gitConfigPath := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(gitConfigPath, []byte("[user]\n\tname = Test User\n\temail = test@example.com\n"), 0644); err != nil {
		t.Fatalf("WriteFile gitconfig: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfigPath)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	wizard := NewWizard("/tmp/test")
	wizard.config.DeployTarget = "github"
	// Accept default "yes" for auth prompt.
	wizard.reader = bufio.NewReader(strings.NewReader("\n"))

	if err := wizard.checkPrerequisites(); err != nil {
		t.Fatalf("checkPrerequisites returned error: %v", err)
	}
}

func TestWizard_checkPrerequisites_Cloudflare_HappyPathWithStubWrangler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stubs not supported on windows in this test")
	}

	binDir := t.TempDir()
	stateDir := t.TempDir()

	wranglerScript := `#!/bin/sh
set -eu
state_dir="${BV_TEST_STATE_DIR:-}"
authed_file="$state_dir/wrangler_authed"
case "${1-}" in
  whoami)
    if [ -f "$authed_file" ]; then
      echo "Account Name: test@example.com"
      echo "Account ID: 123"
      exit 0
    fi
    echo "You are not authenticated. Please run wrangler login"
    exit 0
    ;;
  login)
    mkdir -p "$state_dir"
    : > "$authed_file"
    exit 0
    ;;
esac
exit 0
`
	writeExecutable(t, binDir, "wrangler", wranglerScript)

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("%s%c%s", binDir, os.PathListSeparator, origPath))
	t.Setenv("BV_TEST_STATE_DIR", stateDir)

	wizard := NewWizard("/tmp/test")
	wizard.config.DeployTarget = "cloudflare"
	// Accept default "yes" for auth prompt.
	wizard.reader = bufio.NewReader(strings.NewReader("\n"))

	if err := wizard.checkPrerequisites(); err != nil {
		t.Fatalf("checkPrerequisites returned error: %v", err)
	}
}
