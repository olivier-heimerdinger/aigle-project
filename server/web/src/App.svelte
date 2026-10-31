<script>
  import { onMount } from 'svelte';
  import { api, formatCurrency, ROLE_BADGES, setActiveWorkspaceId } from './lib/api.js';
  import GanttView from './lib/components/GanttView.svelte';
  import KanbanView from './lib/components/KanbanView.svelte';
  import FinanceView from './lib/components/FinanceView.svelte';
  import WikiView from './lib/components/WikiView.svelte';
  import AdminView from './lib/components/AdminView.svelte';
  import PublicPortalView from './lib/components/PublicPortalView.svelte';

  // Détermine si l'utilisateur consulte le Portail Public (/) ou l'Espace SaaS Protégé (/app)
  let viewMode = $state(
    typeof window !== 'undefined' && window.location.pathname.startsWith('/app') ? 'saas' : 'public'
  );
  let isAuthenticated = $state(false);

  let projects = $state([]);
  let activeProjectId = $state('');
  let activeTab = $state('gantt'); // 'portfolio' | 'gantt' | 'kanban' | 'finance' | 'wiki' | 'admin'
  let kanbanWbsFilter = $state('');

  // Contexte d'identité SSO Authelia & Multi-Tenant
  let authContext = $state({
    user: {
      id: '',
      username: 'admin',
      email: 'admin@heimerdinger.studio',
      display_name: 'Administrateur',
      role: 'superadmin',
      workspace_id: 'ws_default',
    },
    active_workspace: { id: 'ws_default', name: 'Studio Heimerdinger (Principal)', slug: 'default' },
    workspaces: [],
    can_write: true,
    can_manage_project: true,
    is_admin: true,
    auth_mode: 'authelia',
    app_domain: 'aigle.heimerdinger.studio',
    auth_domain: 'airgle-auth.heimerdinger.studio',
    logout_url: 'https://airgle-auth.heimerdinger.studio/logout',
  });

  let selectedWorkspaceId = $state('ws_default');

  // État complet du projet actif
  let projectData = $state({
    project: null,
    financials: {},
    wbs_nodes: [],
    dependencies: [],
    tickets: [],
    time_logs: {},
    docs: [],
    members: [],
  });

  let systemInfo = $state({ mode: 'saas', db_driver: 'postgres', auth_mode: 'authelia' });
  let showNewProjectModal = $state(false);
  let newProj = $state({
    code: '',
    name: '',
    client_name: '',
    description: '',
    start_date: '2026-11-01',
    target_end_date: '2027-01-31',
    initial_budget: 35000,
    default_hourly_rate: 75,
    currency: 'EUR',
  });

  async function loadIdentityAndWorkspaces() {
    try {
      const me = await api.getMe();
      authContext = me;
      if (me.active_workspace?.id) {
        selectedWorkspaceId = me.active_workspace.id;
        setActiveWorkspaceId(selectedWorkspaceId);
      }
    } catch (err) {
      console.error('Erreur chargement identité SSO:', err);
    }
  }

  async function switchWorkspace(newWsId) {
    selectedWorkspaceId = newWsId;
    setActiveWorkspaceId(newWsId);
    activeProjectId = '';
    await loadIdentityAndWorkspaces();
    await loadPortfolio();
  }

  async function loadPortfolio(selectId = '') {
    const list = await api.getProjects();
    projects = list || [];
    if (selectId) {
      activeProjectId = selectId;
    } else if (!activeProjectId && projects.length > 0) {
      activeProjectId = projects[0].id;
    } else if (projects.length === 0) {
      activeProjectId = '';
      projectData = {
        project: null,
        financials: {},
        wbs_nodes: [],
        dependencies: [],
        tickets: [],
        time_logs: {},
        docs: [],
        members: [],
      };
    }
    if (activeProjectId) {
      await loadActiveProject();
    }
  }

  async function loadActiveProject() {
    if (!activeProjectId) return;
    try {
      const full = await api.getProjectFull(activeProjectId);
      projectData = full;
      const list = await api.getProjects();
      projects = list || [];
    } catch {}
  }

  async function selectProject(id) {
    activeProjectId = id;
    kanbanWbsFilter = '';
    if (activeTab === 'portfolio' || activeTab === 'admin') {
      activeTab = 'gantt';
    }
    await loadActiveProject();
  }

  function handleJumpToTickets(wbsNodeId) {
    kanbanWbsFilter = wbsNodeId;
    activeTab = 'kanban';
  }

  async function createNewProject(e) {
    e.preventDefault();
    const created = await api.createProject(newProj);
    showNewProjectModal = false;
    activeTab = 'gantt';
    await loadPortfolio(created.id);
  }

  async function enterSaaSWorkspace() {
    if (typeof window !== 'undefined' && !window.location.pathname.startsWith('/app')) {
      // Redirige vers /app afin que Traefik + Authelia déclenchent l'authentification ForwardAuth
      window.location.href = '/app';
      return;
    }
    viewMode = 'saas';
    await loadIdentityAndWorkspaces();
    await loadPortfolio();
  }

  function openPublicPortal() {
    viewMode = 'public';
  }

  onMount(async () => {
    try {
      systemInfo = await api.getHealth();
    } catch {}

    // Si on est sur /app, on charge immédiatement l'environnement SaaS protégé
    if (viewMode === 'saas') {
      await loadIdentityAndWorkspaces();
      isAuthenticated = true;
      await loadPortfolio();

      // Synchronisation temps réel SSE par Workspace
      const evtSource = new EventSource('/api/events');
      evtSource.onmessage = () => {
        loadActiveProject();
      };
      return () => evtSource.close();
    } else {
      // Sur la page publique (/), on teste silencieusement si une session SSO est déjà ouverte
      try {
        const me = await api.getMe();
        if (me?.user?.email) {
          authContext = me;
          isAuthenticated = true;
        }
      } catch {}
    }
  });

  // Totaux consolidés du portefeuille du Workspace actif
  let portfolioTotals = $derived.by(() => {
    let budget = 0,
      consumed = 0,
      remaining = 0,
      landing = 0;
    for (const p of projects) {
      const f = p.financials || {};
      budget += f.initial_budget || 0;
      consumed += f.consumed_cost || 0;
      remaining += f.remaining_cost || 0;
      landing += f.landing_cost || 0;
    }
    return { budget, consumed, remaining, landing, variance: budget - landing };
  });

  let userRoleBadge = $derived(ROLE_BADGES[authContext.user?.role] || ROLE_BADGES.member);
</script>

{#if viewMode === 'public'}
  <PublicPortalView onEnterSaaS={enterSaaSWorkspace} {isAuthenticated} />
{:else}
<div class="min-h-screen flex bg-slate-950 text-slate-100">
  <!-- Barre latérale gauche : Sélecteur Multi-Tenant (Workspace), Projets & Console Admin -->
  <aside class="w-68 shrink-0 bg-slate-900 border-r border-slate-800 flex flex-col justify-between">
    <div class="p-4 space-y-4">
      <!-- En-tête Marque Aigle SaaS -->
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2.5">
          <div class="w-9 h-9 rounded-xl bg-indigo-600 flex items-center justify-center font-black text-lg text-white shadow-md">
            🦅
          </div>
          <div>
            <div class="font-bold text-base tracking-tight text-white flex items-center gap-1.5">
              <span>AIGLE</span>
              <span class="px-1.5 py-0.5 rounded bg-indigo-500/20 text-indigo-300 text-[9px] font-mono uppercase">SaaS</span>
            </div>
            <div class="text-[10px] text-emerald-400 font-mono truncate">
              🔒 SSO Authelia Actif
            </div>
          </div>
        </div>
        <button
          onclick={openPublicPortal}
          class="px-2 py-1 rounded-lg bg-slate-800 hover:bg-slate-700 text-[10px] text-slate-300 border border-slate-700 cursor-pointer"
          title="Ouvrir la vitrine publique & les téléchargements Windows/macOS/Linux"
        >
          🌐 Public / DL
        </button>
      </div>

      <!-- Sélecteur d'Espace Client (Multi-Tenant Workspace) -->
      <div class="p-2.5 rounded-xl bg-slate-950/90 border border-slate-800 space-y-1.5">
        <div class="flex items-center justify-between text-[10px] uppercase tracking-wider text-slate-400 font-semibold">
          <span>🏢 Workspace (Tenant)</span>
          <span class="text-indigo-400 font-mono">{authContext.workspaces?.length || 1}</span>
        </div>
        <select
          value={selectedWorkspaceId}
          onchange={(e) => switchWorkspace(e.currentTarget.value)}
          class="w-full px-2.5 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-xs text-white font-medium cursor-pointer"
        >
          {#each authContext.workspaces || [] as ws (ws.id)}
            <option value={ws.id}>
              {ws.name} {ws.status === 'suspended' ? '(Suspendu)' : ''}
            </option>
          {/each}
        </select>
      </div>

      <!-- Navigation Globale : Portefeuille & Console d'Administration SaaS -->
      <div class="space-y-1.5">
        <button
          onclick={() => (activeTab = 'portfolio')}
          class="w-full flex items-center justify-between px-3 py-2.5 rounded-xl text-xs font-medium transition cursor-pointer {activeTab ===
          'portfolio'
            ? 'bg-indigo-600 text-white shadow-sm'
            : 'bg-slate-950/70 text-slate-300 hover:bg-slate-800 border border-slate-800'}"
        >
          <span>📊 Portefeuille Multi-Projets</span>
          <span class="px-1.5 py-0.5 rounded bg-slate-800 text-[10px] text-slate-200 font-mono">{projects.length}</span>
        </button>

        {#if authContext.is_admin}
          <button
            onclick={() => (activeTab = 'admin')}
            class="w-full flex items-center justify-between px-3 py-2.5 rounded-xl text-xs font-semibold transition cursor-pointer {activeTab ===
            'admin'
              ? 'bg-rose-600 text-white shadow-sm'
              : 'bg-rose-950/30 text-rose-300 hover:bg-rose-950/50 border border-rose-500/30'}"
          >
            <span>🛡️ Console Admin SaaS</span>
            <span class="px-1.5 py-0.5 rounded bg-rose-500/20 text-[9px] text-rose-200 font-mono uppercase">RBAC</span>
          </button>
        {/if}
      </div>

      <!-- Liste des Projets du Workspace Actif -->
      <div class="space-y-2">
        <div class="flex items-center justify-between text-[11px] font-semibold uppercase tracking-wider text-slate-400 px-1">
          <span>Projets du Workspace</span>
          {#if authContext.can_manage_project}
            <button
              onclick={() => {
                newProj = {
                  code: `PRJ${projects.length + 1}`,
                  name: '',
                  client_name: '',
                  description: '',
                  start_date: new Date().toISOString().slice(0, 10),
                  target_end_date: '2027-01-31',
                  initial_budget: 30000,
                  default_hourly_rate: 75,
                  currency: 'EUR',
                };
                showNewProjectModal = true;
              }}
              class="text-indigo-400 hover:text-indigo-300 cursor-pointer"
            >
              + Nouveau
            </button>
          {/if}
        </div>

        <div class="space-y-1.5 max-h-[340px] overflow-y-auto pr-0.5">
          {#each projects as p (p.id)}
            {@const f = p.financials || {}}
            {@const isSelected = activeProjectId === p.id && activeTab !== 'portfolio' && activeTab !== 'admin'}
            <button
              onclick={() => selectProject(p.id)}
              class="w-full text-left p-2.5 rounded-xl border transition cursor-pointer {isSelected
                ? 'bg-indigo-950/50 border-indigo-500/60 text-white'
                : 'bg-slate-950/40 border-slate-800/80 text-slate-300 hover:bg-slate-800/50'}"
            >
              <div class="flex items-center justify-between mb-1">
                <span class="font-mono text-[10px] font-bold px-1.5 py-0.5 rounded bg-indigo-500/20 text-indigo-300">
                  {p.code}
                </span>
                <span
                  class="text-[10px] font-mono {(f.variance || 0) >= 0 ? 'text-emerald-400' : 'text-rose-400'}"
                  title="Atterrissage vs Budget"
                >
                  Att: {formatCurrency(f.landing_cost, p.currency)}
                </span>
              </div>
              <div class="text-xs font-semibold truncate">{p.name}</div>
              <div class="text-[10px] text-slate-500 truncate mt-0.5">
                Budget: {formatCurrency(f.initial_budget, p.currency)} • RAF: {formatCurrency(f.remaining_cost, p.currency)}
              </div>
            </button>
          {/each}
        </div>
      </div>
    </div>

    <!-- Pied de barre latérale : Profil SSO Authelia + Badge Rôle RBAC + Infrastructure -->
    <div class="p-3 border-t border-slate-800 bg-slate-950/60 space-y-2.5 text-xs">
      <div class="p-2.5 rounded-xl bg-slate-900 border border-slate-800 space-y-1.5">
        <div class="flex items-center justify-between">
          <span class="font-semibold text-white truncate">{authContext.user?.display_name || authContext.user?.username}</span>
          <span class="px-1.5 py-0.5 rounded text-[9px] font-semibold border {userRoleBadge.class}">
            {userRoleBadge.label}
          </span>
        </div>
        <div class="text-[10px] text-slate-400 font-mono truncate">{authContext.user?.email}</div>
        <div class="flex items-center justify-between pt-1 border-t border-slate-800/80 text-[10px]">
          <span class="text-emerald-400 font-mono">SSO: {authContext.auth_mode}</span>
          <a
            href={authContext.logout_url}
            class="text-rose-400 hover:underline font-medium"
            title="Déconnexion du portail Authelia"
          >
            Déconnexion ↗
          </a>
        </div>
      </div>

      <div class="text-[10px] text-slate-500 flex items-center justify-between px-1">
        <span>DB: <strong class="text-emerald-400 font-mono uppercase">{systemInfo.db_driver}</strong></span>
        <span class="font-mono text-slate-400">{authContext.app_domain}</span>
      </div>
    </div>
  </aside>

  <!-- Zone Principale -->
  <main class="flex-1 flex flex-col min-w-0 overflow-hidden">
    {#if activeTab === 'admin' && authContext.is_admin}
      <div class="flex-1 p-6 overflow-y-auto">
        <AdminView
          onWorkspaceChanged={async () => {
            await loadIdentityAndWorkspaces();
            await loadPortfolio();
          }}
        />
      </div>
    {:else if activeTab === 'portfolio'}
      <!-- Vue Portefeuille Multi-Projets du Workspace Actif -->
      <div class="p-6 space-y-6 overflow-y-auto">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-[11px] font-mono text-indigo-400 uppercase">
              Workspace : {authContext.active_workspace?.name || 'Principal'}
            </div>
            <h1 class="text-xl font-bold text-white">Portefeuille Multi-Projets — Synthèse Direction</h1>
            <p class="text-xs text-slate-400">
              Vue consolidée des budgets initiaux, des consommés, des restes à faire (RAF) et des atterrissages sur l'espace client actif.
            </p>
          </div>
          {#if authContext.can_manage_project}
            <button
              onclick={() => (showNewProjectModal = true)}
              class="px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold cursor-pointer"
            >
              + Créer un Projet
            </button>
          {/if}
        </div>

        <!-- Totaux Globaux du Portefeuille -->
        <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
          <div class="p-4 rounded-xl bg-slate-900 border border-slate-800">
            <div class="text-xs text-slate-400">Budget Cumulé Portefeuille</div>
            <div class="text-2xl font-bold font-mono text-white mt-1">{formatCurrency(portfolioTotals.budget)}</div>
          </div>
          <div class="p-4 rounded-xl bg-slate-900 border border-slate-800">
            <div class="text-xs text-slate-400">Consommé Cumulé</div>
            <div class="text-2xl font-bold font-mono text-sky-400 mt-1">{formatCurrency(portfolioTotals.consumed)}</div>
          </div>
          <div class="p-4 rounded-xl bg-slate-900 border border-slate-800">
            <div class="text-xs text-slate-400">Reste à Faire Cumulé (RAF)</div>
            <div class="text-2xl font-bold font-mono text-amber-400 mt-1">{formatCurrency(portfolioTotals.remaining)}</div>
          </div>
          <div class="p-4 rounded-xl bg-slate-900 border border-indigo-500/40">
            <div class="text-xs text-indigo-300">Atterrissage Portefeuille</div>
            <div class="text-2xl font-bold font-mono text-white mt-1">{formatCurrency(portfolioTotals.landing)}</div>
          </div>
          <div class="p-4 rounded-xl bg-slate-900 border border-slate-800">
            <div class="text-xs text-slate-400">Écart Global Portefeuille</div>
            <div
              class="text-2xl font-bold font-mono mt-1 {portfolioTotals.variance >= 0
                ? 'text-emerald-400'
                : 'text-rose-400'}"
            >
              {portfolioTotals.variance >= 0 ? '+' : ''}{formatCurrency(portfolioTotals.variance)}
            </div>
          </div>
        </div>

        <!-- Grille des Projets -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          {#each projects as p (p.id)}
            {@const f = p.financials || {}}
            <div
              onclick={() => selectProject(p.id)}
              class="p-5 rounded-2xl bg-slate-900 border border-slate-800 hover:border-indigo-500/50 transition cursor-pointer space-y-4"
            >
              <div class="flex items-start justify-between">
                <div>
                  <div class="flex items-center gap-2">
                    <span class="px-2 py-0.5 rounded bg-indigo-500/20 text-indigo-300 font-mono text-xs font-bold">
                      {p.code}
                    </span>
                    <h3 class="text-base font-bold text-white">{p.name}</h3>
                  </div>
                  <p class="text-xs text-slate-400 mt-1">{p.client_name || 'Projet interne'} • {p.start_date} → {p.target_end_date}</p>
                </div>
                <span
                  class="px-2.5 py-1 rounded-full text-xs font-mono font-semibold {(f.variance || 0) >= 0
                    ? 'bg-emerald-500/15 text-emerald-300 border border-emerald-500/30'
                    : 'bg-rose-500/15 text-rose-300 border border-rose-500/30'}"
                >
                  Écart: {(f.variance || 0) >= 0 ? '+' : ''}{formatCurrency(f.variance, p.currency)}
                </span>
              </div>

              <p class="text-xs text-slate-300 line-clamp-2">{p.description}</p>

              <div class="grid grid-cols-4 gap-2 p-3 rounded-xl bg-slate-950 border border-slate-800 text-xs font-mono">
                <div>
                  <div class="text-[10px] text-slate-500 font-sans">Budget Initial</div>
                  <div class="text-white font-semibold">{formatCurrency(f.initial_budget, p.currency)}</div>
                </div>
                <div>
                  <div class="text-[10px] text-slate-500 font-sans">Consommé</div>
                  <div class="text-sky-400 font-semibold">{formatCurrency(f.consumed_cost, p.currency)}</div>
                </div>
                <div>
                  <div class="text-[10px] text-slate-500 font-sans">RAF</div>
                  <div class="text-amber-400 font-semibold">{formatCurrency(f.remaining_cost, p.currency)}</div>
                </div>
                <div>
                  <div class="text-[10px] text-slate-500 font-sans">Atterrissage</div>
                  <div class="text-indigo-300 font-bold">{formatCurrency(f.landing_cost, p.currency)}</div>
                </div>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {:else if projectData.project}
      {@const proj = projectData.project}
      {@const fin = projectData.financials || {}}

      <!-- Bandeau Financier & Timeline Persistant du Projet -->
      <header class="px-6 py-3.5 bg-slate-900 border-b border-slate-800 flex flex-wrap items-center justify-between gap-4">
        <div class="space-y-0.5">
          <div class="flex items-center gap-2">
            <span class="px-2 py-0.5 rounded bg-indigo-500/20 border border-indigo-500/40 text-indigo-300 font-mono text-xs font-bold">
              {proj.code}
            </span>
            <h1 class="text-base font-bold text-white">{proj.name}</h1>
            <span class="text-xs text-slate-500">({proj.start_date} → {proj.target_end_date})</span>
          </div>
          <p class="text-xs text-slate-400">{proj.description}</p>
        </div>

        <!-- Ruban Financier Temps Réel (Budget, Consommé, RAF, Atterrissage) -->
        <div class="flex items-center gap-3 bg-slate-950 px-3.5 py-2 rounded-xl border border-slate-800 text-xs">
          <div>
            <span class="text-slate-500 block text-[10px]">Budget Initial</span>
            <span class="font-mono font-semibold text-white">{formatCurrency(fin.initial_budget, proj.currency)}</span>
          </div>
          <div class="h-6 w-px bg-slate-800"></div>
          <div>
            <span class="text-slate-500 block text-[10px]">Consommé ({fin.logged_hours || 0}h)</span>
            <span class="font-mono font-semibold text-sky-400">{formatCurrency(fin.consumed_cost, proj.currency)}</span>
          </div>
          <div class="h-6 w-px bg-slate-800"></div>
          <div>
            <span class="text-slate-500 block text-[10px]">Reste à faire ({fin.remaining_hours || 0}h)</span>
            <span class="font-mono font-semibold text-amber-400">{formatCurrency(fin.remaining_cost, proj.currency)}</span>
          </div>
          <div class="h-6 w-px bg-slate-800"></div>
          <div>
            <span class="text-indigo-300 block text-[10px] font-medium">Atterrissage Prévu</span>
            <span class="font-mono font-bold text-white">{formatCurrency(fin.landing_cost, proj.currency)}</span>
          </div>
          <div class="h-6 w-px bg-slate-800"></div>
          <div>
            <span class="text-slate-500 block text-[10px]">Écart Budget</span>
            <span class="font-mono font-bold {(fin.variance || 0) >= 0 ? 'text-emerald-400' : 'text-rose-400'}">
              {(fin.variance || 0) >= 0 ? '+' : ''}{formatCurrency(fin.variance, proj.currency)}
            </span>
          </div>
        </div>
      </header>

      <!-- Onglets des Modules -->
      <nav class="px-6 bg-slate-900/60 border-b border-slate-800 flex items-center gap-1 text-xs overflow-x-auto">
        <button
          onclick={() => (activeTab = 'gantt')}
          class="px-4 py-3 font-medium border-b-2 transition cursor-pointer shrink-0 {activeTab === 'gantt'
            ? 'border-indigo-500 text-indigo-400 bg-indigo-950/20'
            : 'border-transparent text-slate-400 hover:text-slate-200'}"
        >
          📅 1. Timeline MS Project (WBS & Gantt)
          <span class="ml-1 px-1.5 py-0.5 rounded bg-slate-800 text-[10px] font-mono">{projectData.wbs_nodes.length}</span>
        </button>

        <button
          onclick={() => {
            kanbanWbsFilter = '';
            activeTab = 'kanban';
          }}
          class="px-4 py-3 font-medium border-b-2 transition cursor-pointer shrink-0 {activeTab === 'kanban'
            ? 'border-indigo-500 text-indigo-400 bg-indigo-950/20'
            : 'border-transparent text-slate-400 hover:text-slate-200'}"
        >
          📋 2. Kanban & Tickets (Hybride + Sous-tickets)
          <span class="ml-1 px-1.5 py-0.5 rounded bg-slate-800 text-[10px] font-mono">{projectData.tickets.length}</span>
        </button>

        <button
          onclick={() => (activeTab = 'finance')}
          class="px-4 py-3 font-medium border-b-2 transition cursor-pointer shrink-0 {activeTab === 'finance'
            ? 'border-indigo-500 text-indigo-400 bg-indigo-950/20'
            : 'border-transparent text-slate-400 hover:text-slate-200'}"
        >
          💰 3. Environnement Financier (Budget / RAF / Atterrissage)
        </button>

        <button
          onclick={() => (activeTab = 'wiki')}
          class="px-4 py-3 font-medium border-b-2 transition cursor-pointer shrink-0 {activeTab === 'wiki'
            ? 'border-indigo-500 text-indigo-400 bg-indigo-950/20'
            : 'border-transparent text-slate-400 hover:text-slate-200'}"
        >
          📚 4. Documentation & Wiki
          <span class="ml-1 px-1.5 py-0.5 rounded bg-slate-800 text-[10px] font-mono">{projectData.docs.length}</span>
        </button>

        {#if authContext.is_admin}
          <button
            onclick={() => (activeTab = 'admin')}
            class="px-4 py-3 font-semibold border-b-2 transition cursor-pointer shrink-0 {activeTab === 'admin'
              ? 'border-rose-500 text-rose-300 bg-rose-950/20'
              : 'border-transparent text-rose-400/80 hover:text-rose-300'}"
          >
            🛡️ 5. Administration SaaS & Sécurité
          </button>
        {/if}
      </nav>

      <!-- Conteneur de la Vue Active -->
      <div class="flex-1 p-5 overflow-auto">
        {#if activeTab === 'gantt'}
          <GanttView
            project={proj}
            wbsNodes={projectData.wbs_nodes}
            dependencies={projectData.dependencies}
            members={projectData.members}
            onRefresh={loadActiveProject}
            onJumpToTickets={handleJumpToTickets}
          />
        {:else if activeTab === 'kanban'}
          <KanbanView
            project={proj}
            tickets={projectData.tickets}
            wbsNodes={projectData.wbs_nodes}
            members={projectData.members}
            timeLogs={projectData.time_logs}
            initialWbsFilter={kanbanWbsFilter}
            onRefresh={loadActiveProject}
          />
        {:else if activeTab === 'finance'}
          <FinanceView
            project={proj}
            financials={projectData.financials}
            wbsNodes={projectData.wbs_nodes}
            tickets={projectData.tickets}
            members={projectData.members}
            onRefresh={loadActiveProject}
          />
        {:else if activeTab === 'wiki'}
          <WikiView
            project={proj}
            docs={projectData.docs}
            tickets={projectData.tickets}
            wbsNodes={projectData.wbs_nodes}
            onRefresh={loadActiveProject}
          />
        {/if}
      </div>
    {:else}
      <div class="flex-1 flex flex-col items-center justify-center p-8 text-center space-y-3">
        <div class="text-3xl">📂</div>
        <h2 class="text-base font-bold text-white">Aucun projet dans ce Workspace</h2>
        <p class="text-xs text-slate-400 max-w-md">
          Cet espace client ({authContext.active_workspace?.name}) ne contient pas encore de projet.
        </p>
        {#if authContext.can_manage_project}
          <button
            onclick={() => (showNewProjectModal = true)}
            class="px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold cursor-pointer"
          >
            + Créer le premier projet de ce Workspace
          </button>
        {/if}
      </div>
    {/if}
  </main>
</div>

<!-- Modale : Créer un Nouveau Projet -->
{#if showNewProjectModal}
  <div class="fixed inset-0 bg-black/70 backdrop-blur-xs flex items-center justify-center z-50 p-4">
    <form
      onsubmit={createNewProject}
      class="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-lg p-6 space-y-4 shadow-2xl text-xs"
    >
      <div class="flex items-center justify-between border-b border-slate-800 pb-3">
        <h3 class="text-base font-semibold text-white">
          Créer un Projet dans « {authContext.active_workspace?.name || 'Workspace'} »
        </h3>
        <button type="button" onclick={() => (showNewProjectModal = false)} class="text-slate-400 hover:text-white cursor-pointer">✕</button>
      </div>

      <div class="grid grid-cols-3 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Code Projet (Clé)</label>
          <input
            type="text"
            required
            maxlength="6"
            bind:value={newProj.code}
            placeholder="Ex: CRM"
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white uppercase font-mono"
          />
        </div>
        <div class="col-span-2">
          <label class="block text-slate-400 mb-1">Nom du Projet</label>
          <input
            type="text"
            required
            bind:value={newProj.name}
            placeholder="Ex: Déploiement CRM & Portail Client"
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
      </div>

      <div>
        <label class="block text-slate-400 mb-1">Client / Direction</label>
        <input
          type="text"
          bind:value={newProj.client_name}
          class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
        />
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Date de Début</label>
          <input
            type="date"
            required
            bind:value={newProj.start_date}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Date de Fin Cible</label>
          <input
            type="date"
            required
            bind:value={newProj.target_end_date}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Budget Initial Global (€)</label>
          <input
            type="number"
            step="1000"
            required
            bind:value={newProj.initial_budget}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white font-mono"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Taux Horaire par Défaut (€/h)</label>
          <input
            type="number"
            step="5"
            required
            bind:value={newProj.default_hourly_rate}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white font-mono"
          />
        </div>
      </div>

      <div>
        <label class="block text-slate-400 mb-1">Objectifs & Description</label>
        <textarea
          rows="2"
          bind:value={newProj.description}
          class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
        ></textarea>
      </div>

      <div class="flex justify-end gap-2 pt-3 border-t border-slate-800">
        <button type="button" onclick={() => (showNewProjectModal = false)} class="px-4 py-2 rounded-lg bg-slate-800 text-slate-300 cursor-pointer">
          Annuler
        </button>
        <button type="submit" class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium cursor-pointer">
          Créer le Projet
        </button>
      </div>
    </form>
  </div>
{/if}
{/if}

