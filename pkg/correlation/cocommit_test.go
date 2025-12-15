package correlation

import (
	"testing"
	"time"
)

func TestIsCodeFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Code files
		{"pkg/auth/login.go", true},
		{"src/app.py", true},
		{"index.js", true},
		{"app.tsx", true},
		{"main.rs", true},
		{"App.java", true},
		{"config.yaml", true},
		{"data.json", true},
		{"README.md", true},
		{"schema.sql", true},
		{"script.sh", true},

		// Non-code files
		{"image.png", false},
		{"photo.jpg", false},
		{"document.pdf", false},
		{"archive.zip", false},
		{"binary.exe", false},
		{"data.csv", false},

		// Edge cases
		{"Makefile", false}, // No extension
		{".gitignore", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isCodeFile(tt.path)
			if got != tt.want {
				t.Errorf("isCodeFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsExcludedPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Excluded paths
		{".beads/beads.jsonl", true},
		{".beads/issues.jsonl", true},
		{".bv/hooks.yaml", true},
		{".git/objects/abc", true},
		{"node_modules/lodash/index.js", true},
		{"vendor/github.com/pkg/errors/errors.go", true},
		{"__pycache__/module.pyc", true},
		{".venv/lib/python3.9/site.py", true},

		// Not excluded
		{"pkg/auth/login.go", false},
		{"src/components/Button.tsx", false},
		{"cmd/main.go", false},
		{"internal/service/user.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isExcludedPath(tt.path)
			if got != tt.want {
				t.Errorf("isExcludedPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestContainsBeadID(t *testing.T) {
	tests := []struct {
		text   string
		beadID string
		want   bool
	}{
		{"fix: resolve issue bv-123", "bv-123", true},
		{"feat(auth): implement login for BV-123", "bv-123", true}, // Case insensitive
		{"chore: update deps", "bv-123", false},
		{"", "bv-123", false},
		{"some text", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := containsBeadID(tt.text, tt.beadID)
			if got != tt.want {
				t.Errorf("containsBeadID(%q, %q) = %v, want %v", tt.text, tt.beadID, got, tt.want)
			}
		})
	}
}

func TestAllTestFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []FileChange
		want  bool
	}{
		{
			name:  "empty list",
			files: []FileChange{},
			want:  false,
		},
		{
			name: "all go tests",
			files: []FileChange{
				{Path: "pkg/auth/login_test.go"},
				{Path: "pkg/auth/session_test.go"},
			},
			want: true,
		},
		{
			name: "all js tests",
			files: []FileChange{
				{Path: "src/app.test.js"},
				{Path: "src/utils.spec.ts"},
			},
			want: true,
		},
		{
			name: "mixed files",
			files: []FileChange{
				{Path: "pkg/auth/login.go"},
				{Path: "pkg/auth/login_test.go"},
			},
			want: false,
		},
		{
			name: "no test files",
			files: []FileChange{
				{Path: "pkg/auth/login.go"},
				{Path: "pkg/auth/session.go"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allTestFiles(tt.files)
			if got != tt.want {
				t.Errorf("allTestFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShortSHA(t *testing.T) {
	tests := []struct {
		sha  string
		want string
	}{
		{"abc123def456789012345678901234567890abcd", "abc123d"},
		{"abc123", "abc123"},
		{"abc", "abc"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.sha, func(t *testing.T) {
			got := shortSHA(tt.sha)
			if got != tt.want {
				t.Errorf("shortSHA(%q) = %q, want %q", tt.sha, got, tt.want)
			}
		})
	}
}

func TestExtractNewPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Simple rename
		{"old.go => new.go", "new.go"},
		// With braces
		{"pkg/{old => new}/file.go", "pkg/new/file.go"},
		// Complex braces
		{"{old => new}.go", "new.go"},
		// No rename
		{"regular/path.go", "regular/path.go"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractNewPath(tt.input)
			if got != tt.want {
				t.Errorf("extractNewPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCalculateConfidence(t *testing.T) {
	c := NewCoCommitExtractor("/test/repo")
	now := time.Now()

	tests := []struct {
		name      string
		event     BeadEvent
		files     []FileChange
		wantRange [2]float64 // [min, max] expected range
	}{
		{
			name: "base case",
			event: BeadEvent{
				BeadID:    "bv-123",
				CommitMsg: "fix: some bug",
			},
			files: []FileChange{
				{Path: "file.go"},
			},
			wantRange: [2]float64{0.94, 0.96}, // ~0.95
		},
		{
			name: "commit mentions bead ID",
			event: BeadEvent{
				BeadID:    "bv-123",
				CommitMsg: "fix: resolve bv-123",
			},
			files: []FileChange{
				{Path: "file.go"},
			},
			wantRange: [2]float64{0.98, 1.0}, // 0.95 + 0.04 = 0.99
		},
		{
			name: "shotgun commit",
			event: BeadEvent{
				BeadID:    "bv-123",
				CommitMsg: "refactor: big change",
			},
			files: make([]FileChange, 25), // >20 files
			wantRange: [2]float64{0.84, 0.86}, // 0.95 - 0.10 = 0.85
		},
		{
			name: "only test files",
			event: BeadEvent{
				BeadID:    "bv-123",
				CommitMsg: "test: add tests",
			},
			files: []FileChange{
				{Path: "auth_test.go"},
				{Path: "user_test.go"},
			},
			wantRange: [2]float64{0.89, 0.91}, // 0.95 - 0.05 = 0.90
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.event.Timestamp = now
			got := c.calculateConfidence(tt.event, tt.files)
			if got < tt.wantRange[0] || got > tt.wantRange[1] {
				t.Errorf("calculateConfidence() = %v, want in range [%v, %v]", got, tt.wantRange[0], tt.wantRange[1])
			}
		})
	}
}

func TestGenerateReason(t *testing.T) {
	c := NewCoCommitExtractor("/test/repo")

	event := BeadEvent{
		BeadID:    "bv-123",
		EventType: EventClosed,
		CommitMsg: "fix: resolve bv-123",
	}

	files := []FileChange{{Path: "file.go"}}

	reason := c.generateReason(event, files, 0.99)

	if reason == "" {
		t.Error("reason should not be empty")
	}

	// Should mention the event type
	if !contains(reason, "closed") {
		t.Errorf("reason should mention event type, got: %s", reason)
	}

	// Should mention bead ID reference
	if !contains(reason, "bead ID") {
		t.Errorf("reason should mention bead ID reference, got: %s", reason)
	}
}

func TestCreateCorrelatedCommit(t *testing.T) {
	c := NewCoCommitExtractor("/test/repo")
	now := time.Now()

	event := BeadEvent{
		BeadID:      "bv-123",
		EventType:   EventClosed,
		Timestamp:   now,
		CommitSHA:   "abc123def456",
		CommitMsg:   "fix: close bv-123",
		Author:      "Test User",
		AuthorEmail: "test@example.com",
	}

	files := []FileChange{
		{Path: "pkg/auth/login.go", Action: "M", Insertions: 10, Deletions: 5},
	}

	commit := c.CreateCorrelatedCommit(event, files)

	if commit.SHA != event.CommitSHA {
		t.Errorf("SHA mismatch: got %s, want %s", commit.SHA, event.CommitSHA)
	}
	if commit.ShortSHA != "abc123d" {
		t.Errorf("ShortSHA mismatch: got %s", commit.ShortSHA)
	}
	if commit.Method != MethodCoCommitted {
		t.Errorf("Method should be MethodCoCommitted, got %s", commit.Method)
	}
	if commit.Confidence < 0.9 {
		t.Errorf("Confidence should be high for bead ID mention, got %v", commit.Confidence)
	}
	if len(commit.Files) != 1 {
		t.Errorf("Files count mismatch: got %d, want 1", len(commit.Files))
	}
	if commit.Author != event.Author {
		t.Errorf("Author mismatch: got %s, want %s", commit.Author, event.Author)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
