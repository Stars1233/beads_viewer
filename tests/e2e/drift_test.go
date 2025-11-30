package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEndDriftWorkflow(t *testing.T) {
	// 1. Build the binary
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "bv")

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/bv/main.go")
	cmd.Dir = "../../"
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Build failed: %v\n%s", err, out)
	}

	// 2. Prepare environment
	envDir := filepath.Join(tempDir, "env")
	if err := os.MkdirAll(filepath.Join(envDir, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}

	// 3. Create initial healthy graph
	// Chain A <- B <- C (acyclic)
	jsonlContent := `{"id": "A", "title": "Task A", "status": "open", "priority": 1, "issue_type": "task"}
{"id": "B", "title": "Task B", "status": "open", "priority": 1, "issue_type": "task", "dependencies": [{"depends_on_id": "A", "type": "blocks"}]}
{"id": "C", "title": "Task C", "status": "open", "priority": 1, "issue_type": "task", "dependencies": [{"depends_on_id": "B", "type": "blocks"}]}`

	beadsPath := filepath.Join(envDir, ".beads", "beads.jsonl")
	if err := os.WriteFile(beadsPath, []byte(jsonlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Save Baseline
	cmdSave := exec.Command(binPath, "--save-baseline", "Initial state")
	cmdSave.Dir = envDir
	if out, err := cmdSave.CombinedOutput(); err != nil {
		t.Fatalf("Save baseline failed: %v\n%s", err, out)
	}

	// Verify baseline file exists
	baselinePath := filepath.Join(envDir, ".bv", "baseline.json")
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		t.Fatalf("Baseline file not created at %s", baselinePath)
	}

	// 5. Check Drift (Should be clean)
	cmdCheck := exec.Command(binPath, "--check-drift")
	cmdCheck.Dir = envDir
	if out, err := cmdCheck.CombinedOutput(); err != nil {
		t.Fatalf("Check drift (clean) failed: %v\n%s", err, out)
	}

	// 6. Introduce Drift (New Cycle)
	// Create cycle A <- B <- C <- A
	driftedContent := `{"id": "A", "title": "Task A", "status": "open", "priority": 1, "issue_type": "task", "dependencies": [{"depends_on_id": "C", "type": "blocks"}]}
{"id": "B", "title": "Task B", "status": "open", "priority": 1, "issue_type": "task", "dependencies": [{"depends_on_id": "A", "type": "blocks"}]}
{"id": "C", "title": "Task C", "status": "open", "priority": 1, "issue_type": "task", "dependencies": [{"depends_on_id": "B", "type": "blocks"}]}`

	if err := os.WriteFile(beadsPath, []byte(driftedContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 7. Check Drift (Should Fail with Exit Code 1)
	cmdDrift := exec.Command(binPath, "--check-drift")
	cmdDrift.Dir = envDir
	out, err := cmdDrift.CombinedOutput()

	// Expect exit code 1 (Critical)
	if err == nil {
		t.Fatalf("Expected drift check to fail, but it succeeded. Output:\n%s", out)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("Expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit code 1 (critical), got %d", exitErr.ExitCode())
	}
	// Verify output contains "CRITICAL"
	outputStr := string(out)
	if !strings.Contains(outputStr, "CRITICAL") {
		t.Errorf("Expected output to mention CRITICAL, got:\n%s", outputStr)
	}

	// 8. Verify with JSON output
	cmdJson := exec.Command(binPath, "--check-drift", "--robot-drift")
	cmdJson.Dir = envDir
	outJson, err := cmdJson.CombinedOutput()
	// JSON mode also exits with code, so we expect error
	if err == nil {
		t.Fatal("Expected JSON drift check to fail with exit code")
	}

	var result map[string]interface{}
	if err := json.Unmarshal(outJson, &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v\n%s", err, outJson)
	}

	if hasDrift, ok := result["has_drift"].(bool); !ok || !hasDrift {
		t.Error("JSON output has_drift should be true")
	}

	alerts, ok := result["alerts"].([]interface{})
	if !ok || len(alerts) == 0 {
		t.Error("JSON output should have alerts")
	} else {
		// Check first alert is new_cycle
		firstAlert := alerts[0].(map[string]interface{})
		if firstAlert["type"] != "new_cycle" {
			t.Errorf("Expected new_cycle alert, got %v", firstAlert["type"])
		}
	}
}

func TestDriftAlerts(t *testing.T) {
	// 1. Build
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "bv")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/bv/main.go")
	cmd.Dir = "../../"
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Build failed: %v\n%s", err, out)
	}

	// 2. Setup Env
	envDir := filepath.Join(tempDir, "env")
	if err := os.MkdirAll(filepath.Join(envDir, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}
	beadsPath := filepath.Join(envDir, ".beads", "beads.jsonl")

	// 3. Create Baseline
	// 10 Nodes (A..J). 1 Edge (A->B).
	// Density = 1 / (10*9) = 1/90 = 0.0111
	// Blocked: 0 (all marked open)
	baselineContent := ""
	// A is free
	baselineContent += `{"id": "A", "title": "A", "status": "open", "issue_type": "task"}` + "\n"
	// B depends on A
	baselineContent += `{"id": "B", "title": "B", "status": "open", "issue_type": "task", "dependencies": [{"depends_on_id": "A", "type": "blocks"}]}` + "\n"
	// C..J are free
	ids := []string{"C", "D", "E", "F", "G", "H", "I", "J"}
	for _, id := range ids {
		baselineContent += fmt.Sprintf(`{"id": "%s", "title": "%s", "status": "open", "issue_type": "task"}`, id, id) + "\n"
	}

	if err := os.WriteFile(beadsPath, []byte(baselineContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Save Baseline
	cmdSave := exec.Command(binPath, "--save-baseline", "Baseline")
	cmdSave.Dir = envDir
	if out, err := cmdSave.CombinedOutput(); err != nil {
		t.Fatalf("Save baseline failed: %v\n%s", err, out)
	}

	// 4. Create High Density & Blocked Increase
	// Keep 10 Nodes.
	// Add edges: A->C, A->D, ..., A->J (8 more edges).
	// Total Edges: 9.
	// Density = 9/90 = 0.1.
	// Increase: (0.1 - 0.0111) / 0.0111 ~ 800%. Warning.

	// Blocked:
	// Mark B..J as "blocked".
	// Total Blocked: 9.
	// Baseline Blocked: 0.
	// Delta: 9. Threshold 5. Warning.

	driftContent := ""
	driftContent += `{"id": "A", "title": "A", "status": "open", "issue_type": "task"}` + "\n"

	// B..J depend on A and are blocked
	allIds := []string{"B", "C", "D", "E", "F", "G", "H", "I", "J"}
	for _, id := range allIds {
		driftContent += fmt.Sprintf(`{"id": "%s", "title": "%s", "status": "blocked", "issue_type": "task", "dependencies": [{"depends_on_id": "A", "type": "blocks"}]}`, id, id) + "\n"
	}

	if err := os.WriteFile(beadsPath, []byte(driftContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 5. Check Drift
	cmdCheck := exec.Command(binPath, "--check-drift")
	cmdCheck.Dir = envDir
	out, err := cmdCheck.CombinedOutput()

	// Expect Exit Code 2 (Warning)
	if err == nil {
		t.Fatalf("Expected warning exit code, got success. Output:\n%s", out)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("Expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("Expected exit code 2 (warning), got %d", exitErr.ExitCode())
	}

	outputStr := string(out)
	if !strings.Contains(outputStr, "Graph density increased") {
		t.Errorf("Output missing density warning. Got:\n%s", outputStr)
	}
	if !strings.Contains(outputStr, "Blocked issues increased") {
		t.Error("Output missing blocked issues warning")
	}
}
func TestDriftConfigCustomization(t *testing.T) {
	// 1. Build
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "bv")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/bv/main.go")
	cmd.Dir = "../../"
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Build failed: %v\n%s", err, out)
	}

	envDir := filepath.Join(tempDir, "env")
	if err := os.MkdirAll(filepath.Join(envDir, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(envDir, ".bv"), 0755); err != nil {
		t.Fatal(err)
	}

	beadsPath := filepath.Join(envDir, ".beads", "beads.jsonl")
	configPath := filepath.Join(envDir, ".bv", "drift.yaml")
	var out []byte
	var err error

	// -------------------------------------------------------------------------
	// Scenario 1: Custom High Thresholds (Suppress Warnings)
	// -------------------------------------------------------------------------
	t.Log("Scenario 1: Custom High Thresholds")

	// Write custom config: Blocked Increase Threshold = 100 (default 5)
	configContent := `blocked_increase_threshold: 100`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Baseline: 0 blocked
	baselineContent := `{"id": "A", "status": "open", "issue_type": "task"}` + "\n"
	if err := os.WriteFile(beadsPath, []byte(baselineContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmdSave := exec.Command(binPath, "--save-baseline", "Baseline")
	cmdSave.Dir = envDir
	out, err = cmdSave.CombinedOutput()
	if err != nil {
		t.Fatalf("Save baseline failed: %v\n%s", err, out)
	}

	// Current: 10 blocked (would trigger default warning of 5)
	driftContent := `{"id": "A", "status": "open", "issue_type": "task"}` + "\n"
	// Add 10 blocked issues
	for i := 0; i < 10; i++ {
		driftContent += fmt.Sprintf(`{"id": "B%d", "status": "blocked", "issue_type": "task", "dependencies": [{"depends_on_id": "A", "type": "blocks"}]}`+"\n", i)
	}
	if err := os.WriteFile(beadsPath, []byte(driftContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Check Drift - Should PASS (Exit 0) because 10 < 100
	cmdCheck := exec.Command(binPath, "--check-drift")
	cmdCheck.Dir = envDir
	out, err = cmdCheck.CombinedOutput()
	if err != nil {
		t.Errorf("Expected pass with high threshold, got error: %v\nOutput: %s", err, out)
	}

	// -------------------------------------------------------------------------
	// Scenario 2: Custom Low Thresholds (Trigger Warnings)
	// -------------------------------------------------------------------------
	t.Log("Scenario 2: Custom Low Thresholds")

	// Write custom config: Blocked Increase Threshold = 1
	configContent = `blocked_increase_threshold: 1`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Same drift data (10 blocked). 10 > 1. Should FAIL (Exit 2).
	cmdCheck = exec.Command(binPath, "--check-drift")
	cmdCheck.Dir = envDir
	out, err = cmdCheck.CombinedOutput()

	if err == nil {
		t.Error("Expected warning with low threshold, got success")
	} else {
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 2 {
			t.Errorf("Expected exit code 2, got %v", err)
		}
		if !strings.Contains(string(out), "Blocked issues increased") {
			t.Errorf("Expected blocked warning, got:\n%s", out)
		}
	}
}
