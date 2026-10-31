<script>
  import { onMount } from 'svelte';
  import { api, formatCurrency, ROLE_BADGES } from '../api.js';

  let { onWorkspaceChanged = () => {} } = $props();

  let subTab = $state('workspaces'); // 'workspaces' | 'users' | 'audit' | 'security'
  let loading = $state(true);
  let errorMsg = $state('');
  let overview = $state({
    app_version: '2.0.0-saas',
    auth_mode: 'authelia',
    db_driver: 'postgres',
    app_domain: 'aigle.heimerdinger.studio',
    auth_domain: 'airgle-auth.heimerdinger.studio',
    total_workspaces: 0,
    total_users: 0,
    total_projects: 0,
    total_tickets: 0,
    total_budget: 0,
    workspaces: [],
    users: [],
    recent_audit_logs: [],
  });

  // Filtre du journal d'audit
  let auditSearch = $state('');

  // Formulaire de création / édition de Workspace (Tenant)
  let showWsModal = $state(false);
  let editingWs = $state({
    id: '',
    name: '',
    slug: '',
    description: '',
    status: 'active',
    max_projects: 50,
    max_users: 100,
  });

  // Formulaire de pré-enregistrement / édition d'Utilisateur SSO
  let showUserModal = $state(false);
  let editingUser = $state({
    id: '',
    username: '',
    email: '',
    display_name: '',
    role: 'member',
    workspace_id: 'ws_default',
    is_active: true,
  });

  async function loadAdminData() {
    loading = true;
    errorMsg = '';
    try {
      overview = await api.getAdminOverview();
    } catch (err) {
      errorMsg = err.message || 'Impossible de charger les données administrateur';
    } finally {
      loading = false;
    }
  }

  onMount(loadAdminData);

  function openNewWorkspace() {
    editingWs = {
      id: '',
      name: '',
      slug: '',
      description: '',
      status: 'active',
      max_projects: 50,
      max_users: 100,
    };
    showWsModal = true;
  }

  function openEditWorkspace(ws) {
    editingWs = { ...ws };
    showWsModal = true;
  }

  async function saveWorkspace(e) {
    e.preventDefault();
    try {
      if (editingWs.id) {
        await api.updateAdminWorkspace(editingWs.id, editingWs);
      } else {
        await api.createAdminWorkspace(editingWs);
      }
      showWsModal = false;
      await loadAdminData();
      onWorkspaceChanged();
    } catch (err) {
      alert(err.message);
    }
  }

  async function toggleWorkspaceStatus(ws) {
    const nextStatus = ws.status === 'active' ? 'suspended' : 'active';
    await api.updateAdminWorkspace(ws.id, { ...ws, status: nextStatus });
    await loadAdminData();
    onWorkspaceChanged();
  }

  function openNewUser() {
    editingUser = {
      id: '',
      username: '',
      email: '',
      display_name: '',
      role: 'member',
      workspace_id: overview.workspaces[0]?.id || 'ws_default',
      is_active: true,
    };
    showUserModal = true;
  }

  function openEditUser(u) {
    editingUser = { ...u };
    showUserModal = true;
  }

  async function saveUser(e) {
    e.preventDefault();
    try {
      if (editingUser.id) {
        await api.updateAdminUser(editingUser.id, editingUser);
      } else {
        await api.createAdminUser(editingUser);
      }
      showUserModal = false;
      await loadAdminData();
    } catch (err) {
      alert(err.message);
    }
  }

  async function toggleUserActive(u) {
    await api.updateAdminUser(u.id, { ...u, is_active: !u.is_active });
    await loadAdminData();
  }

  let filteredLogs = $derived.by(() => {
    const q = auditSearch.trim().toLowerCase();
    if (!q) return overview.recent_audit_logs || [];
    return (overview.recent_audit_logs || []).filter(
      (l) =>
        (l.action || '').toLowerCase().includes(q) ||
        (l.user_email || '').toLowerCase().includes(q) ||
        (l.details || '').toLowerCase().includes(q) ||
        (l.ip_address || '').toLowerCase().includes(q)
    );
  });
</script>

<div class="space-y-6">
  <!-- En-tête de la Console d'Administration SaaS -->
  <div class="flex flex-wrap items-center justify-between gap-4 p-5 rounded-2xl bg-gradient-to-r from-slate-900 via-indigo-950/40 to-slate-900 border border-indigo-500/30">
    <div>
      <div class="flex items-center gap-2">
        <span class="px-2.5 py-0.5 rounded-full bg-rose-500/20 border border-rose-500/40 text-rose-300 text-[11px] font-bold uppercase tracking-wider">
          🛡️ Back-Office SuperAdmin
        </span>
        <span class="px-2.5 py-0.5 rounded-full bg-emerald-500/15 border border-emerald-500/30 text-emerald-300 text-[11px] font-mono">
          SSO Authelia ({overview.auth_domain})
        </span>
      </div>
      <h2 class="text-lg font-bold text-white mt-1.5">Console d'Administration SaaS & Sécurité Multi-Tenant</h2>
      <p class="text-xs text-slate-400">
        Gouvernance centralisée des espaces clients (Workspaces), des habilitations RBAC Authelia, des quotas et du journal d'audit immuable.
      </p>
    </div>

    <div class="flex items-center gap-2">
      <button
        onclick={loadAdminData}
        class="px-3.5 py-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-medium cursor-pointer border border-slate-700"
      >
        ↻ Actualiser la télémétrie
      </button>
    </div>
  </div>

  {#if errorMsg}
    <div class="p-4 rounded-xl bg-rose-950/50 border border-rose-500/50 text-rose-200 text-xs">
      ⚠️ {errorMsg}
    </div>
  {/if}

  <!-- KPIs Globaux Plateforme SaaS -->
  <div class="grid grid-cols-2 md:grid-cols-5 gap-4">
    <div class="p-4 rounded-xl bg-slate-900 border border-slate-800">
      <div class="text-[11px] text-slate-400">Workspaces (Tenants)</div>
      <div class="text-2xl font-bold font-mono text-white mt-1">{overview.total_workspaces}</div>
      <div class="text-[10px] text-emerald-400 mt-0.5">Isolation SQL stricte</div>
    </div>
    <div class="p-4 rounded-xl bg-slate-900 border border-slate-800">
      <div class="text-[11px] text-slate-400">Comptes SSO Enregistrés</div>
      <div class="text-2xl font-bold font-mono text-sky-400 mt-1">{overview.total_users}</div>
      <div class="text-[10px] text-slate-500 mt-0.5">Synchronisés via ForwardAuth</div>
    </div>
    <div class="p-4 rounded-xl bg-slate-900 border border-slate-800">
      <div class="text-[11px] text-slate-400">Projets Hébergés</div>
      <div class="text-2xl font-bold font-mono text-indigo-400 mt-1">{overview.total_projects}</div>
      <div class="text-[10px] text-slate-500 mt-0.5">{overview.total_tickets} tickets actifs</div>
    </div>
    <div class="p-4 rounded-xl bg-slate-900 border border-slate-800">
      <div class="text-[11px] text-slate-400">Volume Financier Géré</div>
      <div class="text-2xl font-bold font-mono text-amber-400 mt-1">{formatCurrency(overview.total_budget)}</div>
      <div class="text-[10px] text-slate-500 mt-0.5">Cumul multi-organisations</div>
    </div>
    <div class="p-4 rounded-xl bg-slate-900 border border-slate-800">
      <div class="text-[11px] text-slate-400">Moteur de Persistance</div>
      <div class="text-xl font-bold font-mono uppercase text-emerald-400 mt-1">{overview.db_driver}</div>
      <div class="text-[10px] text-slate-400 mt-0.5">Mode Auth : <span class="text-white font-mono">{overview.auth_mode}</span></div>
    </div>
  </div>

  <!-- Sous-navigation de la Console d'Administration -->
  <div class="flex flex-wrap items-center gap-2 border-b border-slate-800 pb-3 text-xs">
    <button
      onclick={() => (subTab = 'workspaces')}
      class="px-4 py-2 rounded-xl font-semibold transition cursor-pointer {subTab === 'workspaces'
        ? 'bg-indigo-600 text-white'
        : 'bg-slate-900 text-slate-400 hover:text-white border border-slate-800'}"
    >
      🏢 1. Espaces Clients (Workspaces Multi-Tenant) ({overview.workspaces?.length || 0})
    </button>
    <button
      onclick={() => (subTab = 'users')}
      class="px-4 py-2 rounded-xl font-semibold transition cursor-pointer {subTab === 'users'
        ? 'bg-indigo-600 text-white'
        : 'bg-slate-900 text-slate-400 hover:text-white border border-slate-800'}"
    >
      👥 2. Utilisateurs SSO & Rôles RBAC ({overview.users?.length || 0})
    </button>
    <button
      onclick={() => (subTab = 'audit')}
      class="px-4 py-2 rounded-xl font-semibold transition cursor-pointer {subTab === 'audit'
        ? 'bg-indigo-600 text-white'
        : 'bg-slate-900 text-slate-400 hover:text-white border border-slate-800'}"
    >
      📜 3. Journal d'Audit de Sécurité ({overview.recent_audit_logs?.length || 0})
    </button>
    <button
      onclick={() => (subTab = 'security')}
      class="px-4 py-2 rounded-xl font-semibold transition cursor-pointer {subTab === 'security'
        ? 'bg-indigo-600 text-white'
        : 'bg-slate-900 text-slate-400 hover:text-white border border-slate-800'}"
    >
      🔒 4. Architecture Sécurité & Traefik / Authelia
    </button>
  </div>

  <!-- ONGLET 1 : GESTION DES WORKSPACES (MULTI-TENANT) -->
  {#if subTab === 'workspaces'}
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <div>
          <h3 class="text-sm font-bold text-white">Organisations & Espaces Clients Isolés (Tenants)</h3>
          <p class="text-xs text-slate-400">
            Chaque Workspace possède ses propres projets, collaborateurs TJM, plannings Gantt, tickets Kanban et pages Wiki.
          </p>
        </div>
        <button
          onclick={openNewWorkspace}
          class="px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold cursor-pointer"
        >
          + Nouveau Workspace Client
        </button>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        {#each overview.workspaces || [] as ws (ws.id)}
          <div class="p-5 rounded-2xl bg-slate-900 border border-slate-800 space-y-4">
            <div class="flex items-start justify-between">
              <div>
                <div class="flex items-center gap-2">
                  <span class="font-mono text-xs px-2 py-0.5 rounded bg-indigo-500/20 text-indigo-300 font-bold">
                    {ws.slug}
                  </span>
                  <h4 class="text-base font-bold text-white">{ws.name}</h4>
                </div>
                <p class="text-xs text-slate-400 mt-1">{ws.description || 'Aucune description'}</p>
              </div>
              <span
                class="px-2.5 py-1 rounded-full text-[11px] font-semibold {ws.status === 'active'
                  ? 'bg-emerald-500/15 text-emerald-300 border border-emerald-500/30'
                  : 'bg-rose-500/20 text-rose-300 border border-rose-500/40'}"
              >
                {ws.status === 'active' ? '● Actif' : '⛔ Suspendu'}
              </span>
            </div>

            <div class="grid grid-cols-4 gap-2 p-3 rounded-xl bg-slate-950 border border-slate-800 text-xs font-mono">
              <div>
                <div class="text-[10px] text-slate-500 font-sans">Projets / Quota</div>
                <div class="text-white font-semibold">{ws.project_count} / {ws.max_projects}</div>
              </div>
              <div>
                <div class="text-[10px] text-slate-500 font-sans">Membres / Quota</div>
                <div class="text-sky-400 font-semibold">{ws.member_count} / {ws.max_users}</div>
              </div>
              <div>
                <div class="text-[10px] text-slate-500 font-sans">Tickets</div>
                <div class="text-indigo-300 font-semibold">{ws.ticket_count}</div>
              </div>
              <div>
                <div class="text-[10px] text-slate-500 font-sans">Budget Cumulé</div>
                <div class="text-amber-400 font-semibold">{formatCurrency(ws.total_budget)}</div>
              </div>
            </div>

            <div class="flex items-center justify-between pt-2 border-t border-slate-800/80 text-xs">
              <span class="text-[11px] text-slate-500 font-mono">ID: {ws.id}</span>
              <div class="flex items-center gap-2">
                <button
                  onclick={() => openEditWorkspace(ws)}
                  class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 cursor-pointer"
                >
                  Modifier Quotas
                </button>
                {#if ws.id !== 'ws_default'}
                  <button
                    onclick={() => toggleWorkspaceStatus(ws)}
                    class="px-3 py-1.5 rounded-lg cursor-pointer {ws.status === 'active'
                      ? 'bg-rose-950/60 hover:bg-rose-900/70 text-rose-300 border border-rose-500/30'
                      : 'bg-emerald-950/60 hover:bg-emerald-900/70 text-emerald-300 border border-emerald-500/30'}"
                  >
                    {ws.status === 'active' ? 'Suspendre' : 'Réactiver'}
                  </button>
                {/if}
              </div>
            </div>
          </div>
        {/each}
      </div>
    </div>

  <!-- ONGLET 2 : GESTION DES UTILISATEURS SSO & RÔLES RBAC -->
  {:else if subTab === 'users'}
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <div>
          <h3 class="text-sm font-bold text-white">Utilisateurs Authentifiés (Authelia SSO) & Habilitations RBAC</h3>
          <p class="text-xs text-slate-400">
            Associez chaque utilisateur à son Workspace client et définissez son niveau d'autorisation (SuperAdmin, Admin, Manager, Member, Viewer).
          </p>
        </div>
        <button
          onclick={openNewUser}
          class="px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold cursor-pointer"
        >
          + Pré-enregistrer un Utilisateur SSO
        </button>
      </div>

      <div class="rounded-2xl bg-slate-900 border border-slate-800 overflow-hidden">
        <table class="w-full text-left text-xs">
          <thead class="bg-slate-950/80 text-slate-400 uppercase text-[10px] border-b border-slate-800">
            <tr>
              <th class="py-3 px-4">Utilisateur / Email SSO</th>
              <th class="py-3 px-4">Groupes Authelia</th>
              <th class="py-3 px-4">Rôle RBAC Aigle</th>
              <th class="py-3 px-4">Workspace Assigné</th>
              <th class="py-3 px-4">Statut</th>
              <th class="py-3 px-4 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70">
            {#each overview.users || [] as u (u.id)}
              {@const badge = ROLE_BADGES[u.role] || ROLE_BADGES.member}
              {@const wsObj = (overview.workspaces || []).find((w) => w.id === u.workspace_id)}
              <tr class="hover:bg-slate-800/40">
                <td class="py-3 px-4">
                  <div class="font-semibold text-white">{u.display_name || u.username}</div>
                  <div class="text-[11px] text-slate-400 font-mono">{u.email}</div>
                </td>
                <td class="py-3 px-4 font-mono text-[11px] text-slate-300">
                  {u.groups || '—'}
                </td>
                <td class="py-3 px-4">
                  <span class="px-2.5 py-0.5 rounded-full text-[11px] font-semibold border {badge.class}">
                    {badge.label}
                  </span>
                </td>
                <td class="py-3 px-4">
                  <span class="px-2 py-0.5 rounded bg-slate-800 text-slate-200 font-mono text-[11px]">
                    {wsObj ? wsObj.name : u.workspace_id}
                  </span>
                </td>
                <td class="py-3 px-4">
                  {#if u.is_active}
                    <span class="text-emerald-400 font-semibold">● Autorisé</span>
                  {:else}
                    <span class="text-rose-400 font-semibold">⛔ Bloqué</span>
                  {/if}
                </td>
                <td class="py-3 px-4 text-right space-x-2">
                  <button
                    onclick={() => openEditUser(u)}
                    class="px-2.5 py-1 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 cursor-pointer"
                  >
                    Éditer RBAC
                  </button>
                  <button
                    onclick={() => toggleUserActive(u)}
                    class="px-2.5 py-1 rounded-lg cursor-pointer {u.is_active
                      ? 'bg-rose-950/60 text-rose-300 hover:bg-rose-900/70'
                      : 'bg-emerald-950/60 text-emerald-300 hover:bg-emerald-900/70'}"
                  >
                    {u.is_active ? 'Suspendre' : 'Activer'}
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>

  <!-- ONGLET 3 : JOURNAL D'AUDIT DE SÉCURITÉ (AUDIT TRAIL) -->
  {:else if subTab === 'audit'}
    <div class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 class="text-sm font-bold text-white">Journal d'Audit de Sécurité Immuable (Audit Trail)</h3>
          <p class="text-xs text-slate-400">
            Traçabilité complète des connexions, rejets d'authentification, changements de rôles RBAC et opérations sensibles avec adresse IP.
          </p>
        </div>
        <input
          type="text"
          bind:value={auditSearch}
          placeholder="Filtrer par email, action, IP..."
          class="px-3.5 py-2 rounded-xl bg-slate-900 border border-slate-700 text-xs text-white w-72"
        />
      </div>

      <div class="rounded-2xl bg-slate-900 border border-slate-800 overflow-hidden">
        <table class="w-full text-left text-xs">
          <thead class="bg-slate-950/80 text-slate-400 uppercase text-[10px] border-b border-slate-800">
            <tr>
              <th class="py-3 px-4">Horodatage</th>
              <th class="py-3 px-4">Action</th>
              <th class="py-3 px-4">Utilisateur & Rôle</th>
              <th class="py-3 px-4">Workspace / Cible</th>
              <th class="py-3 px-4">Détails</th>
              <th class="py-3 px-4">IP Source</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/70 font-mono text-[11px]">
            {#each filteredLogs as log (log.id)}
              {@const isAlert = log.action.includes('REJECTED') || log.action.includes('DENIED') || log.action.includes('BLOCKED') || log.action.includes('DELETE')}
              <tr class="hover:bg-slate-800/40">
                <td class="py-2.5 px-4 text-slate-400 whitespace-nowrap">
                  {new Date(log.created_at).toLocaleString('fr-FR')}
                </td>
                <td class="py-2.5 px-4">
                  <span
                    class="px-2 py-0.5 rounded text-[10px] font-bold {isAlert
                      ? 'bg-rose-500/20 text-rose-300 border border-rose-500/40'
                      : 'bg-indigo-500/15 text-indigo-300 border border-indigo-500/30'}"
                  >
                    {log.action}
                  </span>
                </td>
                <td class="py-2.5 px-4">
                  <span class="text-white">{log.user_email}</span>
                  <span class="text-slate-500 ml-1">({log.user_role})</span>
                </td>
                <td class="py-2.5 px-4 text-slate-300">
                  {log.workspace_id} • {log.target_type}:{log.target_id}
                </td>
                <td class="py-2.5 px-4 font-sans text-slate-300">
                  {log.details}
                </td>
                <td class="py-2.5 px-4 text-sky-400">
                  {log.ip_address}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>

  <!-- ONGLET 4 : ARCHITECTURE DE SÉCURITÉ & PROTECTIONS ACTIVES -->
  {:else if subTab === 'security'}
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs">
      <div class="p-5 rounded-2xl bg-slate-900 border border-slate-800 space-y-3">
        <h3 class="text-sm font-bold text-white flex items-center gap-2">
          <span>🔐</span> Protection Zero-Trust Authelia & Traefik ForwardAuth
        </h3>
        <p class="text-slate-400 leading-relaxed">
          Toutes les requêtes vers <code class="text-indigo-300">https://{overview.app_domain}</code> sont interceptées en amont par Traefik et validées auprès du portail SSO <code class="text-emerald-300">https://{overview.auth_domain}</code> avant d'atteindre le conteneur Go.
        </p>
        <ul class="space-y-1.5 text-slate-300 list-disc list-inside">
          <li>En-têtes vérifiés : <code class="text-sky-300">Remote-User</code>, <code class="text-sky-300">Remote-Email</code>, <code class="text-sky-300">Remote-Groups</code></li>
          <li>Comparaison en temps constant (<code class="text-sky-300">crypto/subtle</code>) du secret <code class="text-sky-300">X-Aigle-Proxy-Secret</code></li>
          <li>Isolation réseau Docker : seul Traefik est sur le réseau externe <code class="text-amber-300">web</code> ; PostgreSQL et Redis résident sur le réseau interne isolé <code class="text-amber-300">braigle</code>.</li>
        </ul>
      </div>

      <div class="p-5 rounded-2xl bg-slate-900 border border-slate-800 space-y-3">
        <h3 class="text-sm font-bold text-white flex items-center gap-2">
          <span>🛡️</span> Défense en Profondeur OWASP Top 10 Active
        </h3>
        <ul class="space-y-2 text-slate-300">
          <li class="flex items-center justify-between py-1 border-b border-slate-800">
            <span>Requêtes SQL 100% Paramétrées (Anti-SQLi)</span>
            <span class="text-emerald-400 font-mono font-bold">ACTIF</span>
          </li>
          <li class="flex items-center justify-between py-1 border-b border-slate-800">
            <span>Content-Security-Policy (CSP) & Anti-Clickjacking (DENY)</span>
            <span class="text-emerald-400 font-mono font-bold">ACTIF</span>
          </li>
          <li class="flex items-center justify-between py-1 border-b border-slate-800">
            <span>HSTS (Strict-Transport-Security 1 an + Preload)</span>
            <span class="text-emerald-400 font-mono font-bold">ACTIF</span>
          </li>
          <li class="flex items-center justify-between py-1 border-b border-slate-800">
            <span>Limiteur de Débit Token-Bucket par IP (Anti-DoS / BruteForce)</span>
            <span class="text-emerald-400 font-mono font-bold">ACTIF</span>
          </li>
          <li class="flex items-center justify-between py-1">
            <span>Conteneur Non-Root (UID 10001) & Limite Payload (2 Mo)</span>
            <span class="text-emerald-400 font-mono font-bold">ACTIF</span>
          </li>
        </ul>
      </div>
    </div>
  {/if}
</div>

<!-- Modal : Créer / Modifier un Workspace (Tenant) -->
{#if showWsModal}
  <div class="fixed inset-0 bg-black/70 backdrop-blur-xs flex items-center justify-center z-50 p-4">
    <form
      onsubmit={saveWorkspace}
      class="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-md p-6 space-y-4 shadow-2xl text-xs"
    >
      <div class="flex items-center justify-between border-b border-slate-800 pb-3">
        <h3 class="text-base font-bold text-white">
          {editingWs.id ? 'Modifier le Workspace Client' : 'Créer un Nouveau Workspace (Tenant)'}
        </h3>
        <button type="button" onclick={() => (showWsModal = false)} class="text-slate-400 hover:text-white cursor-pointer">✕</button>
      </div>

      <div>
        <label class="block text-slate-400 mb-1">Nom de l'Organisation / Client</label>
        <input
          type="text"
          required
          bind:value={editingWs.name}
          placeholder="Ex: Pôle Ingénierie Alpha"
          class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
        />
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Identifiant Court (Slug)</label>
          <input
            type="text"
            required
            bind:value={editingWs.slug}
            placeholder="Ex: client-alpha"
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white font-mono"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Statut</label>
          <select
            bind:value={editingWs.status}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          >
            <option value="active">Actif</option>
            <option value="suspended">Suspendu</option>
          </select>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Quota Max Projets</label>
          <input
            type="number"
            min="1"
            bind:value={editingWs.max_projects}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white font-mono"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Quota Max Utilisateurs</label>
          <input
            type="number"
            min="1"
            bind:value={editingWs.max_users}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white font-mono"
          />
        </div>
      </div>

      <div>
        <label class="block text-slate-400 mb-1">Description / Périmètre</label>
        <textarea
          rows="2"
          bind:value={editingWs.description}
          class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
        ></textarea>
      </div>

      <div class="flex justify-end gap-2 pt-3 border-t border-slate-800">
        <button type="button" onclick={() => (showWsModal = false)} class="px-4 py-2 rounded-lg bg-slate-800 text-slate-300 cursor-pointer">
          Annuler
        </button>
        <button type="submit" class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-semibold cursor-pointer">
          Enregistrer
        </button>
      </div>
    </form>
  </div>
{/if}

<!-- Modal : Pré-enregistrer / Modifier un Utilisateur SSO -->
{#if showUserModal}
  <div class="fixed inset-0 bg-black/70 backdrop-blur-xs flex items-center justify-center z-50 p-4">
    <form
      onsubmit={saveUser}
      class="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-md p-6 space-y-4 shadow-2xl text-xs"
    >
      <div class="flex items-center justify-between border-b border-slate-800 pb-3">
        <h3 class="text-base font-bold text-white">
          {editingUser.id ? 'Modifier les Habilitations RBAC' : 'Pré-enregistrer un Compte SSO Authelia'}
        </h3>
        <button type="button" onclick={() => (showUserModal = false)} class="text-slate-400 hover:text-white cursor-pointer">✕</button>
      </div>

      <div>
        <label class="block text-slate-400 mb-1">Adresse Email (Remote-Email Authelia)</label>
        <input
          type="email"
          required
          disabled={!!editingUser.id}
          bind:value={editingUser.email}
          placeholder="Ex: sophie.martin@heimerdinger.studio"
          class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white font-mono disabled:opacity-50"
        />
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Identifiant (Remote-User)</label>
          <input
            type="text"
            required
            bind:value={editingUser.username}
            placeholder="Ex: smartin"
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white font-mono"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Nom Complet</label>
          <input
            type="text"
            required
            bind:value={editingUser.display_name}
            placeholder="Ex: Sophie Martin"
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Rôle RBAC</label>
          <select
            bind:value={editingUser.role}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          >
            <option value="superadmin">SuperAdmin SaaS (Global)</option>
            <option value="admin">Admin Tenant</option>
            <option value="manager">Chef de Projet (Manager)</option>
            <option value="member">Collaborateur (Member)</option>
            <option value="viewer">Lecture Seule (Viewer)</option>
          </select>
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Workspace Assigné</label>
          <select
            bind:value={editingUser.workspace_id}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          >
            {#each overview.workspaces || [] as ws (ws.id)}
              <option value={ws.id}>{ws.name} ({ws.slug})</option>
            {/each}
          </select>
        </div>
      </div>

      <div class="flex items-center gap-2 pt-1">
        <input type="checkbox" id="usr_active" bind:checked={editingUser.is_active} class="rounded" />
        <label for="usr_active" class="text-slate-300">Compte actif (autorisé à accéder à Aigle)</label>
      </div>

      <div class="flex justify-end gap-2 pt-3 border-t border-slate-800">
        <button type="button" onclick={() => (showUserModal = false)} class="px-4 py-2 rounded-lg bg-slate-800 text-slate-300 cursor-pointer">
          Annuler
        </button>
        <button type="submit" class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-semibold cursor-pointer">
          Enregistrer
        </button>
      </div>
    </form>
  </div>
{/if}

