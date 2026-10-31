# 🦅 AIGLE — Logiciel de Gestion de Projet Tout-en-un (Go + Svelte)

**Aigle** est une plateforme complète de pilotage de projets combinant la puissance de planification de **MS Project** (WBS hiérarchique, diagramme de Gantt interactif, dépendances et chemin critique), l'agilité d'un **Kanban / Gestionnaire de Tickets**, un **Moteur Financier temps réel** (Budget, Consommé, Reste à faire, Atterrissage) et un **Wiki Documentaire** intégré.

Le logiciel est conçu selon une **architecture hybride** :
- **En mode Local (Binaire unique autonome) :** Tout le serveur Go, le moteur SQLite (sans dépendance C / CGO) et l'interface web Svelte 5 sont compilés dans **un seul exécutable `aigle.exe`**.
- **En mode Équipe / SaaS :** Ce même binaire peut être connecté à une base externe (PostgreSQL) et synchronise les modifications entre utilisateurs en temps réel via Server-Sent Events (SSE).

---

## ✨ Fonctionnalités Principales

### 1. Planification type MS Project (WBS & Gantt Interactif)
- **Structure WBS (Work Breakdown Structure) :** Découpage hiérarchique en **Phases / Lots parents**, **Tâches planifiées** et **Jalons (Milestones à durée 0)**.
- **Dépendances & Auto-planification :** Supporte les 4 types de liens MS Project avec décalage (`Lag` en jours) :
  - `FS` (*Finish-to-Start* / Fin-à-Début)
  - `SS` (*Start-to-Start* / Début-à-Début)
  - `FF` (*Finish-to-Finish* / Fin-à-Fin)
  - `SF` (*Start-to-Finish* / Début-à-Fin)
- **Moteur du Chemin Critique (CPM) :**
  - **Passe avant (*Forward Pass*) :** Recalcule automatiquement les dates au plus tôt (`EarlyStart`, `EarlyFinish`) de tous les successeurs dès qu'une tâche est décalée.
  - **Consolidation WBS (*Rollup*) :** Les phases parentes ajustent automatiquement leur plage de dates, leur durée et leur `%` d'avancement pondéré.
  - **Passe arrière (*Backward Pass*) :** Calcule les dates au plus tard (`LateStart`, `LateFinish`), la marge totale (`TotalSlackDays`) et met en évidence en rouge le **Chemin Critique** (marge = 0 jour).

### 2. Gestion de Tickets & Kanban en Mode "Hybride Libre" + Sous-tickets (Enfants)
Contrairement aux outils classiques qui séparent le planning Gantt et les tickets opérationnels :
- **Rattachement obligatoire au Projet :** Chaque ticket (`Tâche`, `Bug`, `Évolution`, `Risque`, `Question`) appartient à un projet et impacte directement son environnement financier global.
- **Épinglage optionnel sur la Timeline Gantt :** Au cas par cas, vous choisissez si un ticket :
  1. **Est épinglé sur une Phase, Tâche ou Jalon du Gantt (`wbs_node_id`) :** Son coût (Consommé, RAF, Atterrissage) et son statut remontent automatiquement dans cette ligne du Gantt, et le ticket hérite de l'échéance de la phase.
  2. **Reste global / transverse au Projet :** Idéal pour le support, l'infrastructure, les risques transverses ou la gestion de projet hors lot spécifique.
- **Hiérarchie Ticket Parent ↔ Sous-tickets Enfants (`parent_ticket_id`) :**
  - Tout ticket peut être découpé en **sous-tickets enfants** en un clic (`+Enfant` sur la carte Kanban ou depuis la fiche du ticket parent).
  - Un sous-ticket hérite par défaut de la ligne Gantt et de la priorité de son ticket parent.
  - Le ticket parent affiche en temps réel le nombre de sous-tickets terminés (`🌿 X/Y sous-tickets`) ainsi que l'**Atterrissage Consolidé (`Coût Parent + Somme des Sous-tickets`)**, sans jamais compter deux fois les coûts dans le budget global du projet.
- **Deux vues synchronisées :** Vue **Kanban Drag-and-Drop** (`Backlog`, `À faire`, `En cours`, `En revue`, `Terminé`) et **Vue Tableau Financier**.

### 3. Environnement Financier Complet (EVM Simplifié)
Chaque intervenant possède un **Taux Horaire (THM)** et un **TJM** (`THM × 8h`). À chaque pointage de temps (`TimeLog`) ou ajustement du **Reste à faire (RAF)**, le moteur financier recalcule instantanément :

$$\text{Coût Initial Estimé} = (\text{Heures Estimées} \times \text{Taux Horaire}) + \text{Coûts Fixes}$$

$$\text{Coût Consommé (AC)} = \sum (\text{Heures Pointées} \times \text{Taux Horaire}) + \text{Coûts Fixes}$$

$$\text{Valorisation du Reste à Faire — RAF (ETC)} = \text{Heures RAF} \times \text{Taux Horaire}$$

$$\text{Atterrissage Prévu (EAC)} = \text{Coût Consommé} + \text{Coût RAF}$$

$$\text{Écart à Terminaison (Variance)} = \text{Budget Initial} - \text{Atterrissage Prévu}$$

Ces indicateurs sont consolidés à **3 niveaux** :
1. **Par Ticket** (avec alerte visuelle en cas de dérive sur le budget initial du ticket).
2. **Par Phase / Lot du Gantt** (ainsi qu'une ligne dédiée aux *Tickets Transverses hors Gantt*).
3. **Par Projet et sur le Portefeuille Multi-Projets** (Synthèse Direction).

### 4. Gestion Documentaire (Wiki Projet)
- Pages organisées par catégories (`Architecture`, `Spécifications`, `Guide / Méthode`, `Risques & ADR`, `Compte-rendu COPIL`).
- Support du Markdown avec **liens croisés bidirectionnels** : mentionner `#AIG-3` dans une page affiche un badge interactif et liste automatiquement en bas du document tous les tickets liés avec leur statut et leur atterrissage financier.

---

## 🚀 Démarrage Rapide (Scripts `.cmd` inclus)

Quatre scripts batch Windows sont fournis à la racine du projet pour simplifier toutes les opérations :

| Script | Rôle |
| :--- | :--- |
| **`run.cmd`** | Lance `aigle.exe` et ouvre `http://localhost:8080` dans le navigateur (compile automatiquement le projet si `aigle.exe` n'existe pas). |
| **`build.cmd`** | Compile le frontend Svelte avec Bun (`web/dist`), exécute les tests Go, et génère l'exécutable autonome `aigle.exe`. |
| **`dev.cmd`** | Mode développement avec rechargement à chaud (Hot-Reload) : démarre le serveur API Go sur `:8080` et le serveur Vite/Svelte sur `http://localhost:5173`. |
| **`test.cmd`** | Exécute l'ensemble des tests unitaires (moteurs CPM & Financier) et des tests d'intégration API sur SQLite en mémoire. |

### Lancement en ligne de commande
```powershell
# Démarrage standard en mode local (crée aigle.db automatiquement)
.\aigle.exe -open

# Choisir un port et un fichier de base de données spécifiques
.\aigle.exe -port=3000 -db="mes_projets.db" -open

# Mode SaaS / Équipe
.\aigle.exe -mode=saas -db-driver=sqlite -db="equipe.db" -port=8080
```

---

## 📂 Structure du Projet

```text
Aigle/
├── cmd/
│   └── aigle/
│       └── main.go                 # Point d'entrée de l'exécutable unique
├── internal/
│   ├── config/
│   │   └── config.go               # Configuration CLI & Variables d'environnement (Local / SaaS)
│   ├── models/
│   │   └── models.go               # Modèles métier (Project, WBSNode, Ticket, TimeLog, FinancialSummary, DocPage)
│   ├── engine/
│   │   ├── scheduler.go            # Moteur MS Project (CPM, tri topologique, Forward/Backward pass, WBS Rollup)
│   │   ├── financial.go            # Moteur Financier (Consommé, RAF, Atterrissage, Écart par Ticket/Phase/Projet)
│   │   └── engine_test.go          # Tests unitaires des moteurs temporel et financier
│   ├── database/
│   │   ├── db.go                   # Couche SQL (SQLite WAL pur Go / Postgres) & Requêtes CRUD/Upsert
│   │   └── seed.go                 # Jeu de données de démonstration réaliste (2 projets complets)
│   └── api/
│       ├── server.go               # Routeur REST + Flux temps réel Server-Sent Events (SSE)
│       └── server_test.go          # Tests d'intégration de bout en bout (API + SQLite :memory: + UI embarquée)
├── web/                            # Frontend Svelte 5 + Vite + TailwindCSS v4
│   ├── embed.go                    # Directive //go:embed all:dist pour intégrer le build dans le binaire Go
│   ├── package.json                # Configuration Bun / Vite / Svelte 5
│   └── src/
│       ├── App.svelte              # Coque Multi-Projets, Portefeuille Direction & Bandeau Financier permanent
│       └── lib/
│           ├── api.js              # Client HTTP REST & Formatage financier/temporel
│           └── components/
│               ├── GanttView.svelte    # Vue 1 : Grille WBS MS Project + Diagramme de Gantt SVG interactif
│               ├── KanbanView.svelte   # Vue 2 : Tableau Kanban Drag & Drop + Pointages & RAF
│               ├── FinanceView.svelte  # Vue 3 : Pilotage Budgétaire, Ventilation par Lot & Grille TJM/THM
│               └── WikiView.svelte     # Vue 4 : Documentation Markdown & Liens bidirectionnels Tickets
├── build.cmd                       # Script de build complet (Frontend Svelte + Tests + Binaire Go)
├── dev.cmd                         # Script de développement Live-Reload (Go + Vite)
├── run.cmd                         # Script d'exécution rapide de aigle.exe
└── test.cmd                        # Script d'exécution des tests automatisés
```

---

## 🔌 Principaux Endpoints de l'API REST

| Méthode | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/health` | État du serveur, mode (`local` / `saas`) et driver DB actif |
| `GET` | `/api/events` | Flux temps réel **SSE** notifiant les modifications aux clients connectés |
| `GET / POST` | `/api/projects` | Liste du portefeuille multi-projets (avec calculs financiers) et création |
| `GET` | `/api/projects/{id}/full` | Environnement complet d'un projet (WBS recalculé, dépendances, tickets, pointages, finances, wiki, membres) |
| `POST / PUT / DELETE` | `/api/projects/{id}/wbs` & `/api/wbs/{nodeId}` | Gestion des lignes du Gantt et recalcul automatique du chemin critique |
| `POST / DELETE` | `/api/projects/{id}/dependencies` | Création / suppression de liens `FS`, `SS`, `FF`, `SF` avec détection de cycles |
| `POST / PUT / DELETE` | `/api/projects/{id}/tickets` & `/api/tickets/{ticketId}` | Gestion des tickets (rattachés au projet + épinglage Gantt optionnel) |
| `PATCH` | `/api/tickets/{ticketId}/move` | Déplacement rapide de colonne Kanban et mise à jour auto du RAF si `done` |
| `POST` | `/api/tickets/{ticketId}/timelogs` | Ajout d'un pointage d'heures valorisé et ajustement du Reste à faire (RAF) |
| `POST / PUT / DELETE` | `/api/projects/{id}/docs` & `/api/docs/{docId}` | Gestion de l'arborescence documentaire Markdown |
| `GET / POST / PUT` | `/api/members` | Gestion des intervenants et de leur taux horaire (`hourly_rate`) / TJM |

