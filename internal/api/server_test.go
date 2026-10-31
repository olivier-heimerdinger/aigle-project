package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aigle/aigle/internal/config"
	"github.com/aigle/aigle/internal/database"
	"github.com/aigle/aigle/internal/models"
	"github.com/aigle/aigle/web"
)

func TestAPIEndToEnd_SQLiteInMemory(t *testing.T) {
	cfg := &config.Config{
		Mode:     "local",
		DBDriver: "sqlite",
		DBDSN:    ":memory:",
		Port:     "8080",
	}

	store, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open in-memory SQLite store: %v", err)
	}
	defer store.DB.Close()

	srv := NewServer(cfg, store, web.DistFS())
	handler := srv.Handler()

	// 0. Verify embedded Svelte frontend is served at "/"
	reqUI := httptest.NewRequest("GET", "/", nil)
	recUI := httptest.NewRecorder()
	handler.ServeHTTP(recUI, reqUI)
	if recUI.Code != http.StatusOK || !strings.Contains(recUI.Body.String(), `<div id="app"></div>`) {
		t.Fatalf("Expected embedded Svelte index.html at '/', got status %d", recUI.Code)
	}

	// 1. Verify seeded projects are returned with computed Financials
	req := httptest.NewRequest("GET", "/api/projects", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/projects returned %d: %s", rec.Code, rec.Body.String())
	}

	var projects []models.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
		t.Fatalf("Unmarshal projects failed: %v", err)
	}
	if len(projects) < 2 {
		t.Fatalf("Expected at least 2 seeded projects, got %d", len(projects))
	}

	// 2. Fetch full project environment for prj_cloud
	reqFull := httptest.NewRequest("GET", "/api/projects/prj_cloud/full", nil)
	recFull := httptest.NewRecorder()
	handler.ServeHTTP(recFull, reqFull)

	if recFull.Code != http.StatusOK {
		t.Fatalf("GET /api/projects/prj_cloud/full returned %d: %s", recFull.Code, recFull.Body.String())
	}

	var fullResp struct {
		Project    models.Project        `json:"project"`
		Financials models.FinancialSummary `json:"financials"`
		WBSNodes   []models.WBSNode      `json:"wbs_nodes"`
		Tickets    []models.Ticket       `json:"tickets"`
		Docs       []models.DocPage      `json:"docs"`
	}
	if err := json.Unmarshal(recFull.Body.Bytes(), &fullResp); err != nil {
		t.Fatalf("Unmarshal full project failed: %v", err)
	}

	if fullResp.Financials.InitialBudget != 45000 {
		t.Errorf("Expected InitialBudget 45000, got %.2f", fullResp.Financials.InitialBudget)
	}
	if fullResp.Financials.ConsumedCost <= 0 || fullResp.Financials.LandingCost <= 0 {
		t.Errorf("Expected positive ConsumedCost and LandingCost, got %.2f / %.2f",
			fullResp.Financials.ConsumedCost, fullResp.Financials.LandingCost)
	}

	// 3. Create a new ticket pinned to wbs_2_2 and log time on it
	newTicket := map[string]any{
		"wbs_node_id":     "wbs_2_2",
		"title":           "Intégration des graphiques d'atterrissage financier",
		"ticket_type":     "feature",
		"status":          "in_progress",
		"priority":        "high",
		"assignee_id":     "mem_3",
		"estimated_hours": 16,
		"remaining_hours": 16,
	}
	body, _ := json.Marshal(newTicket)
	reqCreate := httptest.NewRequest("POST", "/api/projects/prj_cloud/tickets", bytes.NewReader(body))
	recCreate := httptest.NewRecorder()
	handler.ServeHTTP(recCreate, reqCreate)

	if recCreate.Code != http.StatusCreated {
		t.Fatalf("POST /api/projects/prj_cloud/tickets returned %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var createdTicket models.Ticket
	_ = json.Unmarshal(recCreate.Body.Bytes(), &createdTicket)
	if createdTicket.ID == "" {
		t.Fatalf("Expected created ticket ID")
	}

	// Log 6 hours on the ticket and set RAF to 8 hours
	rem := 8.0
	logBody, _ := json.Marshal(map[string]any{
		"member_id":       "mem_3",
		"hours":           6,
		"comment":         "Développement initial du composant budget",
		"remaining_hours": rem,
	})
	reqLog := httptest.NewRequest("POST", "/api/tickets/"+createdTicket.ID+"/timelogs", bytes.NewReader(logBody))
	recLog := httptest.NewRecorder()
	handler.ServeHTTP(recLog, reqLog)

	if recLog.Code != http.StatusCreated {
		t.Fatalf("POST /api/tickets/{id}/timelogs returned %d: %s", recLog.Code, recLog.Body.String())
	}
}

