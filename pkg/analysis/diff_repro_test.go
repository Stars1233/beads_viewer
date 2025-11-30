package analysis

import (
	"testing"

	"beads_viewer/pkg/model"
)

func TestCompareSnapshots_DependencyTypeChange(t *testing.T) {
	// Issue A relates to B
	fromIssues := []model.Issue{
		{
			ID:     "A",
			Status: model.StatusOpen,
			Dependencies: []*model.Dependency{
				{DependsOnID: "B", Type: model.DepRelated},
			},
		},
		{ID: "B", Status: model.StatusOpen},
	}

	// Issue A now BLOCKS B (Type change)
	toIssues := []model.Issue{
		{
			ID:     "A",
			Status: model.StatusOpen,
			Dependencies: []*model.Dependency{
				{DependsOnID: "B", Type: model.DepBlocks},
			},
		},
		{ID: "B", Status: model.StatusOpen},
	}

	from := NewSnapshot(fromIssues)
	to := NewSnapshot(toIssues)
	diff := CompareSnapshots(from, to)

	// We expect this to be detected as a modified issue
	if len(diff.ModifiedIssues) != 1 {
		t.Fatalf("expected 1 modified issue (dependency type change), got %d", len(diff.ModifiedIssues))
	}

	mod := diff.ModifiedIssues[0]
	foundDepChange := false
	for _, c := range mod.Changes {
		if c.Field == "dependencies" {
			foundDepChange = true
			break
		}
	}

	if !foundDepChange {
		t.Error("expected dependency change to be detected when Type changes from related to blocks")
	}
}
