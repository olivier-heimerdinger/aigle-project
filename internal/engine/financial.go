package engine

import (
	"math"

	"github.com/aigle/aigle/internal/models"
)

// roundCurrency arrondit un montant financier à 2 décimales pour éviter les imprécisions de virgule flottante.
func roundCurrency(val float64) float64 {
	return math.Round(val*100) / 100
}

// ComputeProjectFinancials évalue l'environnement financier complet d'un projet selon le modèle Hybride Libre :
//
// 1. Valorisation de chaque Ticket :
//    - Détermine le taux horaire applicable (Taux de l'intervenant assigné ou taux par défaut du projet).
//    - Coût Initial Estimé = (EstimatedHours * Rate) + FixedCost
//    - Coût Consommé Réel (AC) = Somme(TimeLogs.Hours * RateSnapshot) + FixedCost
//    - Coût du Reste à Faire - RAF (ETC) = RemainingHours * Rate (0 si le ticket est au statut "done")
//    - Atterrissage Prévu (EAC) = ConsumedCost + RemainingCost
//    - Écart sur Ticket (Variance) = InitialCost - LandingCost
//
// 2. Consolidation sur la Timeline Gantt (WBSNodes) :
//    - Si un ticket est épinglé sur une ligne du Gantt (WBSNodeID != ""), ses heures et ses coûts
//      (Estimé, Consommé, RAF, Atterrissage) sont cumulés sur cette ligne WBS puis remontés vers les Phases parentes.
//    - Si un ticket n'est pas épinglé (WBSNodeID == ""), il est comptabilisé dans les coûts transverses du projet (Unpinned*).
//
// 3. Consolidation au niveau du Projet Global :
//    - Compare le Budget Initial du Projet face à la somme de TOUS les tickets du projet (épinglés + transverses).
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
		defaultRate = 65.0 // Taux horaire par défaut (520€ TJM base 8h)
	}

	projSummary := &models.FinancialSummary{
		InitialBudget: project.InitialBudget,
		HealthStatus:  "healthy",
	}

	// 1. Évaluation financière individuelle de chaque Ticket
	for _, t := range tickets {
		rate := defaultRate
		if m, ok := memberMap[t.AssigneeID]; ok {
			t.AssigneeName = m.Name
			if m.HourlyRate > 0 {
				rate = m.HourlyRate
			}
		}
		t.EffectiveRate = rate

		// Calcul du temps consommé et de son coût à partir de l'historique des pointages (TimeLogs)
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

		// Lorsqu'un ticket passe à "Terminé" (done), son Reste à faire (RAF) tombe automatiquement à 0
		if t.Status == "done" {
			t.RemainingHours = 0
		}

		t.InitialCost = roundCurrency((t.EstimatedHours * rate) + t.FixedCost)
		t.ConsumedCost = roundCurrency(timeLogCost + t.FixedCost)
		t.RemainingCost = roundCurrency(t.RemainingHours * rate)
		t.LandingCost = roundCurrency(t.ConsumedCost + t.RemainingCost)
		t.CostVariance = roundCurrency(t.InitialCost - t.LandingCost)
		t.IsOverBudget = t.LandingCost > t.InitialCost && t.InitialCost > 0

		// Vérifie si le ticket est épinglé sur une ligne de la Timeline Gantt (WBSNode)
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
				// Si la ligne WBS a été supprimée, le ticket redevient un ticket global du projet
				t.WBSNodeID = ""
			}
		}

		// Si le ticket est rattaché uniquement au Projet (hors ligne Gantt), on l'isole aussi dans Unpinned*
		if t.WBSNodeID == "" {
			projSummary.UnpinnedConsumed = roundCurrency(projSummary.UnpinnedConsumed + t.ConsumedCost)
			projSummary.UnpinnedRemaining = roundCurrency(projSummary.UnpinnedRemaining + t.RemainingCost)
			projSummary.UnpinnedLanding = roundCurrency(projSummary.UnpinnedLanding + t.LandingCost)
		}

		// Consolidation dans les totaux globaux du Projet
		projSummary.EstimatedCost = roundCurrency(projSummary.EstimatedCost + t.InitialCost)
		projSummary.ConsumedCost = roundCurrency(projSummary.ConsumedCost + t.ConsumedCost)
		projSummary.RemainingCost = roundCurrency(projSummary.RemainingCost + t.RemainingCost)
		projSummary.LandingCost = roundCurrency(projSummary.LandingCost + t.LandingCost)

		projSummary.EstimatedHours = roundCurrency(projSummary.EstimatedHours + t.EstimatedHours)
		projSummary.LoggedHours = roundCurrency(projSummary.LoggedHours + t.LoggedHours)
		projSummary.RemainingHours = roundCurrency(projSummary.RemainingHours + t.RemainingHours)
		projSummary.LandingHours = roundCurrency(projSummary.LoggedHours + projSummary.RemainingHours)
	}

	// 1b. Consolidation Parent / Enfants entre Tickets (Sous-tickets) :
	// Permet d'afficher sur chaque ticket parent le nombre de sous-tickets et le coût consolidé (Parent + Enfants),
	// et sur chaque ticket enfant la référence (Clé + Titre) de son ticket parent.
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

	// 2. Remontée hiérarchique (Rollup) des coûts des sous-tâches WBS vers leurs Phases parentes
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

		// Si la phase parente n'a pas de budget propre saisi, elle consolide la somme des budgets de ses sous-tâches
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

	// 3. Calcul final des ratios et de la santé financière globale du Projet
	finalizeSummaryMetrics(projSummary)
	project.Financials = projSummary
	return projSummary
}

// finalizeSummaryMetrics calcule l'écart (Variance), les pourcentages de consommation/atterrissage et l'indicateur de santé.
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
			s.HealthStatus = "critical" // Dépassement supérieur à 5% du budget
		} else if s.LandingCost > refBudget {
			s.HealthStatus = "warning" // Léger dépassement entre 0% et 5%
		} else {
			s.HealthStatus = "healthy" // Sous ou à l'équilibre du budget
		}
	}
}
