package engine

import (
	"math"

	"github.com/aigle/aigle-server/internal/models"
)

func roundCurrency(val float64) float64 {
	return math.Round(val*100) / 100
}

// ComputeProjectFinancials calcule l'environnement financier complet d'un projet (Budget, Consommé, RAF, Atterrissage)
// ainsi que la consolidation Parent/Sous-tickets et la remontée vers les phases WBS du Gantt.
func ComputeProjectFinancials(
	project *models.Project,
	nodes []*models.WBSNode,
	tickets []*models.Ticket,
	members []models.Member,
	logsByTicket map[string][]models.TimeLog,
) *models.FinancialSummary {
	memberMap := make(map[string]models.Member, len(members))
	for _, m := range members {
		memberMap[m.ID] = m
	}

	nodeMap := make(map[string]*models.WBSNode, len(nodes))
	childrenMap := make(map[string][]*models.WBSNode)
	for _, n := range nodes {
		nodeMap[n.ID] = n
		n.PinnedTicketCnt = 0
		n.DoneTicketCnt = 0
		n.Financials = &models.FinancialSummary{
			InitialBudget: n.AllocatedBudget,
			HealthStatus:  "healthy",
		}
		if n.ParentID != "" {
			childrenMap[n.ParentID] = append(childrenMap[n.ParentID], n)
		}
	}

	defaultRate := project.DefaultHourlyRate
	if defaultRate <= 0 {
		defaultRate = 65.0
	}

	projSummary := &models.FinancialSummary{
		InitialBudget: project.InitialBudget,
		HealthStatus:  "healthy",
	}

	for _, t := range tickets {
		rate := defaultRate
		if m, ok := memberMap[t.AssigneeID]; ok {
			t.AssigneeName = m.Name
			if m.HourlyRate > 0 {
				rate = m.HourlyRate
			}
		}
		t.EffectiveRate = rate

		logs := logsByTicket[t.ID]
		var loggedHours, timeLogCost float64
		if len(logs) > 0 {
			for _, l := range logs {
				loggedHours += l.Hours
				r := l.HourlyRateSnapshot
				if r <= 0 {
					r = rate
				}
				timeLogCost += l.Hours * r
			}
			t.LoggedHours = roundCurrency(loggedHours)
		} else {
			loggedHours = t.LoggedHours
			timeLogCost = loggedHours * rate
		}

		if t.Status == "done" {
			t.RemainingHours = 0
		}

		t.InitialCost = roundCurrency((t.EstimatedHours * rate) + t.FixedCost)
		t.ConsumedCost = roundCurrency(timeLogCost + t.FixedCost)
		t.RemainingCost = roundCurrency(t.RemainingHours * rate)
		t.LandingCost = roundCurrency(t.ConsumedCost + t.RemainingCost)
		t.CostVariance = roundCurrency(t.InitialCost - t.LandingCost)
		t.IsOverBudget = t.LandingCost > t.InitialCost && t.InitialCost > 0

		if t.WBSNodeID != "" {
			if wbs, ok := nodeMap[t.WBSNodeID]; ok {
				t.WBSNodeTitle = wbs.Title
				t.WBSNodeCode = wbs.WBSCode
				t.WBSNodeEndDate = wbs.EndDate
				if t.DueDate == "" {
					t.DueDate = wbs.EndDate
				}
				if t.DueDate != "" && wbs.EndDate != "" && t.DueDate > wbs.EndDate {
					t.IsLateOnTimeline = true
				}

				wbs.PinnedTicketCnt++
				if t.Status == "done" {
					wbs.DoneTicketCnt++
				}
				wbs.Financials.EstimatedCost = roundCurrency(wbs.Financials.EstimatedCost + t.InitialCost)
				wbs.Financials.ConsumedCost = roundCurrency(wbs.Financials.ConsumedCost + t.ConsumedCost)
				wbs.Financials.RemainingCost = roundCurrency(wbs.Financials.RemainingCost + t.RemainingCost)
				wbs.Financials.LandingCost = roundCurrency(wbs.Financials.LandingCost + t.LandingCost)

				wbs.Financials.EstimatedHours = roundCurrency(wbs.Financials.EstimatedHours + t.EstimatedHours)
				wbs.Financials.LoggedHours = roundCurrency(wbs.Financials.LoggedHours + t.LoggedHours)
				wbs.Financials.RemainingHours = roundCurrency(wbs.Financials.RemainingHours + t.RemainingHours)
				wbs.Financials.LandingHours = roundCurrency(wbs.Financials.LoggedHours + wbs.Financials.RemainingHours)
			} else {
				t.WBSNodeID = ""
			}
		}

		if t.WBSNodeID == "" {
			projSummary.UnpinnedConsumed = roundCurrency(projSummary.UnpinnedConsumed + t.ConsumedCost)
			projSummary.UnpinnedRemaining = roundCurrency(projSummary.UnpinnedRemaining + t.RemainingCost)
			projSummary.UnpinnedLanding = roundCurrency(projSummary.UnpinnedLanding + t.LandingCost)
		}

		projSummary.EstimatedCost = roundCurrency(projSummary.EstimatedCost + t.InitialCost)
		projSummary.ConsumedCost = roundCurrency(projSummary.ConsumedCost + t.ConsumedCost)
		projSummary.RemainingCost = roundCurrency(projSummary.RemainingCost + t.RemainingCost)
		projSummary.LandingCost = roundCurrency(projSummary.LandingCost + t.LandingCost)

		projSummary.EstimatedHours = roundCurrency(projSummary.EstimatedHours + t.EstimatedHours)
		projSummary.LoggedHours = roundCurrency(projSummary.LoggedHours + t.LoggedHours)
		projSummary.RemainingHours = roundCurrency(projSummary.RemainingHours + t.RemainingHours)
		projSummary.LandingHours = roundCurrency(projSummary.LoggedHours + projSummary.RemainingHours)
	}

	// Consolidation Parent / Sous-tickets (Enfants)
	ticketMap := make(map[string]*models.Ticket, len(tickets))
	for _, t := range tickets {
		ticketMap[t.ID] = t
		t.SubTicketCnt = 0
		t.DoneSubTicketCnt = 0
		t.ChildrenConsumedCost = 0
		t.ChildrenRemainingCost = 0
		t.ChildrenLandingCost = 0
		t.ConsolidatedLanding = t.LandingCost
	}
	for _, t := range tickets {
		if t.ParentTicketID != "" {
			if parent, ok := ticketMap[t.ParentTicketID]; ok && parent.ID != t.ID {
				t.ParentTicketKey = parent.TicketKey
				t.ParentTicketTitle = parent.Title
				parent.SubTicketCnt++
				if t.Status == "done" {
					parent.DoneSubTicketCnt++
				}
				parent.ChildrenConsumedCost = roundCurrency(parent.ChildrenConsumedCost + t.ConsumedCost)
				parent.ChildrenRemainingCost = roundCurrency(parent.ChildrenRemainingCost + t.RemainingCost)
				parent.ChildrenLandingCost = roundCurrency(parent.ChildrenLandingCost + t.LandingCost)
				parent.ConsolidatedLanding = roundCurrency(parent.LandingCost + parent.ChildrenLandingCost)
			} else {
				t.ParentTicketID = ""
			}
		}
	}

	// Remontée hiérarchique WBS
	visited := make(map[string]bool)
	var rollupWBS func(n *models.WBSNode)
	rollupWBS = func(n *models.WBSNode) {
		if visited[n.ID] {
			return
		}
		visited[n.ID] = true

		children := childrenMap[n.ID]
		var childBudgetSum float64
		for _, ch := range children {
			rollupWBS(ch)
			childBudgetSum += ch.Financials.InitialBudget
			n.PinnedTicketCnt += ch.PinnedTicketCnt
			n.DoneTicketCnt += ch.DoneTicketCnt

			n.Financials.EstimatedCost = roundCurrency(n.Financials.EstimatedCost + ch.Financials.EstimatedCost)
			n.Financials.ConsumedCost = roundCurrency(n.Financials.ConsumedCost + ch.Financials.ConsumedCost)
			n.Financials.RemainingCost = roundCurrency(n.Financials.RemainingCost + ch.Financials.RemainingCost)
			n.Financials.LandingCost = roundCurrency(n.Financials.LandingCost + ch.Financials.LandingCost)

			n.Financials.EstimatedHours = roundCurrency(n.Financials.EstimatedHours + ch.Financials.EstimatedHours)
			n.Financials.LoggedHours = roundCurrency(n.Financials.LoggedHours + ch.Financials.LoggedHours)
			n.Financials.RemainingHours = roundCurrency(n.Financials.RemainingHours + ch.Financials.RemainingHours)
			n.Financials.LandingHours = roundCurrency(n.Financials.LoggedHours + n.Financials.RemainingHours)
		}

		if n.Financials.InitialBudget <= 0 && childBudgetSum > 0 {
			n.Financials.InitialBudget = roundCurrency(childBudgetSum)
			n.AllocatedBudget = n.Financials.InitialBudget
		}

		finalizeSummaryMetrics(n.Financials)
	}

	for _, n := range nodes {
		if n.ParentID == "" {
			rollupWBS(n)
		}
	}
	for _, n := range nodes {
		rollupWBS(n)
	}

	finalizeSummaryMetrics(projSummary)
	project.Financials = projSummary
	return projSummary
}

func finalizeSummaryMetrics(s *models.FinancialSummary) {
	refBudget := s.InitialBudget
	if refBudget <= 0 {
		refBudget = s.EstimatedCost
	}
	s.Variance = roundCurrency(refBudget - s.LandingCost)
	if refBudget > 0 {
		s.ConsumptionRatio = roundCurrency((s.ConsumedCost / refBudget) * 100)
		s.LandingRatio = roundCurrency((s.LandingCost / refBudget) * 100)
		s.VariancePercent = roundCurrency((s.Variance / refBudget) * 100)
	}

	if refBudget > 0 {
		if s.LandingCost > refBudget*1.05 {
			s.HealthStatus = "critical"
		} else if s.LandingCost > refBudget {
			s.HealthStatus = "warning"
		} else {
			s.HealthStatus = "healthy"
		}
	}
}

