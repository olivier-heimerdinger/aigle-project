package engine

import (
	"testing"

	"github.com/aigle/aigle/internal/models"
)

func TestComputeSchedule_CPMAndRollup(t *testing.T) {
	nodes := []*models.WBSNode{
		{
			ID:           "phase-1",
			ProjectID:    "p1",
			WBSCode:      "1",
			Title:        "Phase 1 : Conception & Architecture",
			NodeType:     "phase",
			StartDate:    "2026-11-01",
			EndDate:      "2026-11-01",
			DurationDays: 1,
			OrderIndex:   1,
		},
		{
			ID:           "task-1",
			ProjectID:    "p1",
			ParentID:     "phase-1",
			WBSCode:      "1.1",
			Title:        "Cadrage fonctionnel",
			NodeType:     "task",
			StartDate:    "2026-11-02",
			DurationDays: 5, // 2026-11-02 -> 2026-11-06
			Progress:     100,
			OrderIndex:   2,
		},
		{
			ID:           "task-2",
			ProjectID:    "p1",
			ParentID:     "phase-1",
			WBSCode:      "1.2",
			Title:        "Spécifications techniques",
			NodeType:     "task",
			StartDate:    "2026-11-02", // Will be pushed by FS dependency on task-1
			DurationDays: 4,            // Expected: 2026-11-07 -> 2026-11-10
			Progress:     50,
			OrderIndex:   3,
		},
		{
			ID:           "mile-1",
			ProjectID:    "p1",
			ParentID:     "phase-1",
			WBSCode:      "1.3",
			Title:        "Jalon Validation Conception",
			NodeType:     "milestone",
			StartDate:    "2026-11-02", // Pushed by task-2 FS
			OrderIndex:   4,
		},
	}

	deps := []models.WBSDependency{
		{
			ID:            "d1",
			ProjectID:     "p1",
			PredecessorID: "task-1",
			SuccessorID:   "task-2",
			DepType:       "FS",
			LagDays:       0,
		},
		{
			ID:            "d2",
			ProjectID:     "p1",
			PredecessorID: "task-2",
			SuccessorID:   "mile-1",
			DepType:       "FS",
			LagDays:       0,
		},
	}

	if err := ComputeSchedule(nodes, deps); err != nil {
		t.Fatalf("ComputeSchedule returned unexpected error: %v", err)
	}

	// Verify Task 1 dates
	if nodes[1].EndDate != "2026-11-06" {
		t.Errorf("Expected task-1 EndDate 2026-11-06, got %s", nodes[1].EndDate)
	}

	// Verify Task 2 was auto-scheduled after Task 1 (FS)
	if nodes[2].StartDate != "2026-11-07" || nodes[2].EndDate != "2026-11-10" {
		t.Errorf("Expected task-2 2026-11-07..2026-11-10, got %s..%s", nodes[2].StartDate, nodes[2].EndDate)
	}

	// Verify Milestone 1 was auto-scheduled after Task 2
	if nodes[3].StartDate != "2026-11-11" {
		t.Errorf("Expected milestone StartDate 2026-11-11, got %s", nodes[3].StartDate)
	}

	// Verify Parent Phase 1 rolled up child dates and critical path
	if nodes[0].StartDate != "2026-11-02" || nodes[0].EndDate != "2026-11-11" {
		t.Errorf("Expected phase-1 rolled up dates 2026-11-02..2026-11-11, got %s..%s", nodes[0].StartDate, nodes[0].EndDate)
	}
	if !nodes[1].IsCritical || !nodes[2].IsCritical || !nodes[0].IsCritical {
		t.Errorf("Expected chain to be marked critical")
	}
}

func TestComputeProjectFinancials_HybridModel(t *testing.T) {
	project := &models.Project{
		ID:                "p1",
		InitialBudget:     10000, // 10 000 €
		DefaultHourlyRate: 50,
	}

	members := []models.Member{
		{ID: "m1", Name: "Alice", HourlyRate: 80}, // 640€ TJM
	}

	nodes := []*models.WBSNode{
		{
			ID:              "phase-1",
			ProjectID:       "p1",
			Title:           "Phase 1",
			AllocatedBudget: 6000,
		},
	}

	tickets := []*models.Ticket{
		{
			// Ticket 1: Pinned to Phase 1 (Parent ticket)
			ID:             "t1",
			TicketKey:      "AIG-1",
			ProjectID:      "p1",
			WBSNodeID:      "phase-1",
			Title:          "Architecture DB",
			Status:         "in_progress",
			AssigneeID:     "m1", // 80€/h
			EstimatedHours: 20,   // Initial: 1600€
			RemainingHours: 10,   // RAF: 10h * 80€ = 800€
			FixedCost:      200,  // Licence: 200€
		},
		{
			// Ticket 2: Unpinned (attached globally to Project only) AND Child of Ticket 1
			ID:             "t2",
			TicketKey:      "AIG-2",
			ProjectID:      "p1",
			WBSNodeID:      "",
			ParentTicketID: "t1",
			Title:          "Support Transverse & DevOps",
			Status:         "todo",
			AssigneeID:     "", // Uses DefaultHourlyRate = 50€/h
			EstimatedHours: 10, // Initial: 500€
			LoggedHours:    4,  // Consumed: 4h * 50€ = 200€
			RemainingHours: 8,  // RAF: 8h * 50€ = 400€
		},
	}

	logsByTicket := map[string][]models.TimeLog{
		"t1": {
			{ID: "l1", TicketID: "t1", MemberID: "m1", Hours: 15, HourlyRateSnapshot: 80}, // 15h * 80€ = 1200€
		},
	}

	summary := ComputeProjectFinancials(project, nodes, tickets, members, logsByTicket)

	// Check Ticket 1 (Pinned):
	// Consumed = 1200 (logs) + 200 (fixed) = 1400€
	// Remaining (RAF) = 10h * 80 = 800€
	// Landing (Atterrissage) = 1400 + 800 = 2200€
	if tickets[0].ConsumedCost != 1400 || tickets[0].RemainingCost != 800 || tickets[0].LandingCost != 2200 {
		t.Fatalf("Unexpected Ticket 1 financials: Consumed=%.2f, Remaining=%.2f, Landing=%.2f",
			tickets[0].ConsumedCost, tickets[0].RemainingCost, tickets[0].LandingCost)
	}
	if !tickets[0].IsOverBudget { // Landing 2200 > Initial (20*80 + 200 = 1800)
		t.Errorf("Expected Ticket 1 to be flagged IsOverBudget")
	}

	// Check Phase 1 rollup:
	if nodes[0].Financials.LandingCost != 2200 || nodes[0].PinnedTicketCnt != 1 {
		t.Errorf("Expected Phase 1 LandingCost=2200 and PinnedTicketCnt=1, got %.2f and %d",
			nodes[0].Financials.LandingCost, nodes[0].PinnedTicketCnt)
	}

	// Check Project Global Summary (Ticket 1 Pinned + Ticket 2 Unpinned):
	// Ticket 2: Consumed = 200€, Remaining = 400€, Landing = 600€
	// Total Project Consumed = 1400 + 200 = 1600€
	// Total Project Remaining (RAF) = 800 + 400 = 1200€
	// Total Project Landing (Atterrissage) = 2200 + 600 = 2800€
	// Total Project Variance = 10000 - 2800 = +7200€
	if summary.ConsumedCost != 1600 || summary.RemainingCost != 1200 || summary.LandingCost != 2800 || summary.Variance != 7200 {
		t.Fatalf("Unexpected Project Summary: Consumed=%.2f, RAF=%.2f, Landing=%.2f, Variance=%.2f",
			summary.ConsumedCost, summary.RemainingCost, summary.LandingCost, summary.Variance)
	}
	if summary.UnpinnedLanding != 600 {
		t.Errorf("Expected UnpinnedLanding=600, got %.2f", summary.UnpinnedLanding)
	}

	// Verify Parent/Child sub-ticket consolidation (2200 parent + 600 child = 2800 consolidated)
	if tickets[0].SubTicketCnt != 1 || tickets[0].ConsolidatedLanding != 2800 {
		t.Errorf("Expected Parent Ticket SubTicketCnt=1 and ConsolidatedLanding=2800, got %d and %.2f",
			tickets[0].SubTicketCnt, tickets[0].ConsolidatedLanding)
	}
	if tickets[1].ParentTicketKey != "AIG-1" {
		t.Errorf("Expected Child Ticket ParentTicketKey='AIG-1', got %q", tickets[1].ParentTicketKey)
	}
}

