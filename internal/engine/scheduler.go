// Package engine contient les deux moteurs algorithmiques au cœur d'Aigle :
// 1. Le moteur d'ordonnancement temporel type MS Project (scheduler.go) basé sur la méthode du Chemin Critique (CPM).
// 2. Le moteur de consolidation financière temps réel (financial.go) calculant Budget, Consommé, RAF et Atterrissage.
package engine

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/aigle/aigle/internal/models"
)

// DateLayout définit le format de date ISO standard (YYYY-MM-DD) utilisé dans toute l'application.
const DateLayout = "2006-01-02"

// ParseDate convertit une chaîne YYYY-MM-DD en time.Time UTC, ou retourne la date du jour en cas d'erreur.
func ParseDate(s string) time.Time {
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
	return t
}

// FormatDate formate un objet time.Time au format YYYY-MM-DD.
func FormatDate(t time.Time) string {
	return t.Format(DateLayout)
}

// DiffDays calcule l'écart en jours calendaires entre deux dates.
func DiffDays(start, end time.Time) int {
	days := int(math.Round(end.Sub(start).Hours() / 24.0))
	if days < 0 {
		return 0
	}
	return days
}

// ComputeSchedule exécute le calcul d'ordonnancement automatique type MS Project (Critical Path Method - CPM) :
//
// Étape 1 — Tri Topologique (Algorithme de Kahn) :
//   Vérifie que le graphe de dépendances ne comporte aucun cycle (boucle infinie) et ordonne les tâches.
//
// Étape 2 — Passe Avant (Forward Pass) :
//   Propage les dates au plus tôt (EarlyStart / EarlyFinish) le long des liens de dépendance :
//   - FS (Fin-à-Début) : Le successeur démarre le lendemain de la fin du prédécesseur (+ LagDays).
//   - SS (Début-à-Début) : Le successeur démarre en même temps que le prédécesseur (+ LagDays).
//   - FF (Fin-à-Fin) : Le successeur se termine en même temps que le prédécesseur (+ LagDays).
//   - SF (Début-à-Fin) : Le successeur se termine par rapport au début du prédécesseur (+ LagDays).
//
// Étape 3 — Consolidation Hiérarchique WBS (Rollup) :
//   Les phases parentes calculent automatiquement leur date de début (Min des enfants),
//   leur date de fin (Max des enfants), leur durée totale et leur % d'avancement pondéré par la durée.
//
// Étape 4 — Passe Arrière (Backward Pass) & Chemin Critique :
//   En partant de la date de fin globale du projet, calcule les dates au plus tard (LateStart / LateFinish)
//   et la marge totale (TotalSlackDays = LateStart - EarlyStart).
//   Toute tâche ayant une marge nulle (TotalSlackDays == 0) est marquée sur le Chemin Critique (IsCritical = true).
func ComputeSchedule(nodes []*models.WBSNode, deps []models.WBSDependency) error {
	if len(nodes) == 0 {
		return nil
	}

	nodeMap := make(map[string]*models.WBSNode, len(nodes))
	childrenMap := make(map[string][]*models.WBSNode)
	for _, n := range nodes {
		nodeMap[n.ID] = n
		if n.ParentID != "" {
			childrenMap[n.ParentID] = append(childrenMap[n.ParentID], n)
		}
		// Normalisation initiale des dates et durées pour chaque ligne WBS
		start := ParseDate(n.StartDate)
		end := ParseDate(n.EndDate)
		if n.NodeType == "milestone" {
			// Un jalon (milestone) a toujours une durée de 0 jour
			n.DurationDays = 0
			n.EndDate = FormatDate(start)
		} else {
			if n.DurationDays <= 0 {
				d := DiffDays(start, end) + 1
				if d <= 0 {
					d = 1
				}
				n.DurationDays = d
			}
			end = start.AddDate(0, 0, n.DurationDays-1)
			n.StartDate = FormatDate(start)
			n.EndDate = FormatDate(end)
		}
	}

	// Construction des listes d'adjacence du graphe de dépendances
	inDegree := make(map[string]int, len(nodes))
	outgoing := make(map[string][]models.WBSDependency)
	incoming := make(map[string][]models.WBSDependency)
	for _, n := range nodes {
		inDegree[n.ID] = 0
	}
	for _, d := range deps {
		if _, ok1 := nodeMap[d.PredecessorID]; !ok1 {
			continue
		}
		if _, ok2 := nodeMap[d.SuccessorID]; !ok2 {
			continue
		}
		outgoing[d.PredecessorID] = append(outgoing[d.PredecessorID], d)
		incoming[d.SuccessorID] = append(incoming[d.SuccessorID], d)
		inDegree[d.SuccessorID]++
	}

	// Tri topologique de Kahn pour ordonner les tâches et détecter les dépendances circulaires
	var queue []string
	for _, n := range nodes {
		if inDegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	sort.Slice(queue, func(i, j int) bool {
		return nodeMap[queue[i]].OrderIndex < nodeMap[queue[j]].OrderIndex
	})

	var topoOrder []string
	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]
		topoOrder = append(topoOrder, currID)

		for _, dep := range outgoing[currID] {
			succID := dep.SuccessorID
			inDegree[succID]--
			if inDegree[succID] == 0 {
				queue = append(queue, succID)
			}
		}
	}

	if len(topoOrder) < len(nodes) {
		return fmt.Errorf("cycle détecté dans les dépendances du planning Gantt")
	}

	// 1. PASSE AVANT (FORWARD PASS) : Propagation des dates au plus tôt selon les prédécesseurs
	for _, id := range topoOrder {
		curr := nodeMap[id]
		start := ParseDate(curr.StartDate)

		for _, dep := range incoming[id] {
			pred := nodeMap[dep.PredecessorID]
			predStart := ParseDate(pred.StartDate)
			predEnd := ParseDate(pred.EndDate)

			var requiredStart time.Time
			switch dep.DepType {
			case "SS": // Début-à-Début
				requiredStart = predStart.AddDate(0, 0, dep.LagDays)
			case "FF": // Fin-à-Fin
				reqEnd := predEnd.AddDate(0, 0, dep.LagDays)
				if curr.NodeType == "milestone" || curr.DurationDays <= 1 {
					requiredStart = reqEnd
				} else {
					requiredStart = reqEnd.AddDate(0, 0, -(curr.DurationDays - 1))
				}
			case "SF": // Début-à-Fin
				reqEnd := predStart.AddDate(0, 0, dep.LagDays)
				if curr.NodeType == "milestone" || curr.DurationDays <= 1 {
					requiredStart = reqEnd
				} else {
					requiredStart = reqEnd.AddDate(0, 0, -(curr.DurationDays - 1))
				}
			default: // "FS" Fin-à-Début (Standard MS Project : jour suivant la fin du prédécesseur + lag)
				offset := 1 + dep.LagDays
				if pred.NodeType == "milestone" {
					offset = dep.LagDays
				}
				requiredStart = predEnd.AddDate(0, 0, offset)
			}

			if requiredStart.After(start) {
				start = requiredStart
			}
		}

		curr.StartDate = FormatDate(start)
		if curr.NodeType == "milestone" {
			curr.EndDate = curr.StartDate
		} else {
			end := start.AddDate(0, 0, curr.DurationDays-1)
			curr.EndDate = FormatDate(end)
		}
		curr.EarlyStart = curr.StartDate
		curr.EarlyFinish = curr.EndDate
	}

	// 2. CONSOLIDATION WBS (ROLLUP) : Agrégation ascendante des enfants vers les phases parentes
	var rollupNode func(n *models.WBSNode)
	visitedRollup := make(map[string]bool)
	rollupNode = func(n *models.WBSNode) {
		if visitedRollup[n.ID] {
			return
		}
		visitedRollup[n.ID] = true
		children := childrenMap[n.ID]
		if len(children) == 0 {
			return
		}

		var minStart, maxEnd time.Time
		var totalWeight, weightedProgress float64

		for i, ch := range children {
			rollupNode(ch)
			chStart := ParseDate(ch.StartDate)
			chEnd := ParseDate(ch.EndDate)
			if i == 0 || chStart.Before(minStart) {
				minStart = chStart
			}
			if i == 0 || chEnd.After(maxEnd) {
				maxEnd = chEnd
			}
			weight := float64(ch.DurationDays)
			if weight <= 0 {
				weight = 1
			}
			totalWeight += weight
			weightedProgress += ch.Progress * weight
		}

		n.StartDate = FormatDate(minStart)
		n.EndDate = FormatDate(maxEnd)
		n.EarlyStart = n.StartDate
		n.EarlyFinish = n.EndDate
		n.DurationDays = DiffDays(minStart, maxEnd) + 1
		if totalWeight > 0 {
			n.Progress = math.Round((weightedProgress/totalWeight)*10) / 10
		}
	}

	for _, n := range nodes {
		if n.ParentID == "" {
			rollupNode(n)
		}
	}
	for _, n := range nodes {
		rollupNode(n)
	}

	// 3. PASSE ARRIÈRE (BACKWARD PASS) : Calcul des dates au plus tard, de la marge et du Chemin Critique
	var projectEnd time.Time
	for i, n := range nodes {
		end := ParseDate(n.EndDate)
		if i == 0 || end.After(projectEnd) {
			projectEnd = end
		}
	}

	lateFinishMap := make(map[string]time.Time, len(nodes))
	lateStartMap := make(map[string]time.Time, len(nodes))
	for _, n := range nodes {
		lateFinishMap[n.ID] = projectEnd
	}

	// Parcours en ordre topologique inverse
	for i := len(topoOrder) - 1; i >= 0; i-- {
		id := topoOrder[i]
		curr := nodeMap[id]
		lf := lateFinishMap[id]

		for _, dep := range outgoing[id] {
			succ := nodeMap[dep.SuccessorID]
			succLS := lateStartMap[succ.ID]
			succLF := lateFinishMap[succ.ID]

			var allowedFinish time.Time
			switch dep.DepType {
			case "SS":
				allowedStart := succLS.AddDate(0, 0, -dep.LagDays)
				if curr.NodeType == "milestone" || curr.DurationDays <= 1 {
					allowedFinish = allowedStart
				} else {
					allowedFinish = allowedStart.AddDate(0, 0, curr.DurationDays-1)
				}
			case "FF":
				allowedFinish = succLF.AddDate(0, 0, -dep.LagDays)
			default: // FS
				offset := 1 + dep.LagDays
				if curr.NodeType == "milestone" {
					offset = dep.LagDays
				}
				allowedFinish = succLS.AddDate(0, 0, -offset)
			}

			if allowedFinish.Before(lf) {
				lf = allowedFinish
			}
		}

		lateFinishMap[id] = lf
		var ls time.Time
		if curr.NodeType == "milestone" || curr.DurationDays <= 1 {
			ls = lf
		} else {
			ls = lf.AddDate(0, 0, -(curr.DurationDays - 1))
		}
		lateStartMap[id] = ls

		curr.LateStart = FormatDate(ls)
		curr.LateFinish = FormatDate(lf)

		es := ParseDate(curr.EarlyStart)
		slack := int(math.Round(ls.Sub(es).Hours() / 24.0))
		if slack < 0 {
			slack = 0
		}
		curr.TotalSlackDays = slack
		// Une tâche feuille ou un jalon sans marge (slack == 0) appartient au Chemin Critique
		if len(childrenMap[curr.ID]) == 0 {
			curr.IsCritical = (slack == 0)
		}
	}

	// Une phase parente est marquée critique si au moins l'une de ses sous-tâches est critique
	var markParentCritical func(n *models.WBSNode) bool
	markParentCritical = func(n *models.WBSNode) bool {
		children := childrenMap[n.ID]
		if len(children) == 0 {
			return n.IsCritical
		}
		anyCrit := false
		for _, ch := range children {
			if markParentCritical(ch) {
				anyCrit = true
			}
		}
		n.IsCritical = anyCrit
		return anyCrit
	}
	for _, n := range nodes {
		if n.ParentID == "" {
			markParentCritical(n)
		}
	}

	return nil
}
