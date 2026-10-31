<script>
  import { api, formatCurrency, KANBAN_COLUMNS, TICKET_TYPES, PRIORITIES } from '../api.js';

  let {
    project,
    tickets = [],
    wbsNodes = [],
    members = [],
    timeLogs = {},
    initialWbsFilter = '',
    onRefresh,
  } = $props();

  let displayMode = $state('kanban'); // 'kanban' | 'list'
  let filterWbs = $state('');
  let filterType = $state('');
  let filterAssignee = $state('');
  let filterHierarchy = $state(''); // '' | 'parents' | 'children'
  let filterOverBudgetOnly = $state(false);
  let searchQuery = $state('');

  // Synchronise le filtre WBS lorsqu'on clique sur le badge de tickets depuis la vue Gantt
  $effect(() => {
    if (initialWbsFilter !== undefined) {
      filterWbs = initialWbsFilter;
    }
  });

  let draggedTicketId = $state(null);
  let dragOverCol = $state(null);

  let showTicketModal = $state(false);
  let showTimeLogModal = $state(false);
  let errorMsg = $state('');

  let editingTicket = $state({
    id: '',
    wbs_node_id: '',
    parent_ticket_id: '',
    title: '',
    description: '',
    ticket_type: 'task',
    status: 'todo',
    priority: 'medium',
    assignee_id: '',
    estimated_hours: 16,
    logged_hours: 0,
    remaining_hours: 16,
    fixed_cost: 0,
    due_date: '',
    tags: '',
  });

  let activeTicketForLog = $state(null);
  let newLog = $state({
    member_id: '',
    hours: 4,
    work_date: new Date().toISOString().slice(0, 10),
    comment: '',
    remaining_hours: 0,
  });

  let filteredTickets = $derived.by(() => {
    return tickets.filter((t) => {
      if (filterWbs === '__unpinned__' && t.wbs_node_id !== '') return false;
      if (filterWbs && filterWbs !== '__unpinned__' && t.wbs_node_id !== filterWbs) return false;
      if (filterType && t.ticket_type !== filterType) return false;
      if (filterAssignee && t.assignee_id !== filterAssignee) return false;
      if (filterHierarchy === 'parents' && t.parent_ticket_id !== '') return false;
      if (filterHierarchy === 'children' && t.parent_ticket_id === '') return false;
      if (filterOverBudgetOnly && !t.is_over_budget) return false;
      if (searchQuery) {
        const q = searchQuery.toLowerCase();
        const matchKey = (t.ticket_key || '').toLowerCase().includes(q);
        const matchTitle = (t.title || '').toLowerCase().includes(q);
        const matchTags = (t.tags || '').toLowerCase().includes(q);
        const matchParent = (t.parent_ticket_key || '').toLowerCase().includes(q);
        if (!matchKey && !matchTitle && !matchTags && !matchParent) return false;
      }
      return true;
    });
  });

  // Liste des sous-tickets rattachés au ticket actuellement ouvert dans la modale
  let currentSubTickets = $derived.by(() => {
    if (!editingTicket.id) return [];
    return tickets.filter((t) => t.parent_ticket_id === editingTicket.id);
  });

  // Ouvre la modale de création d'un nouveau ticket (ou d'un sous-ticket si parentTicket est fourni)
  function openNewTicket(defaultStatus = 'todo', parentTicket = null) {
    errorMsg = '';
    editingTicket = {
      id: '',
      project_id: project.id,
      wbs_node_id: parentTicket
        ? parentTicket.wbs_node_id
        : filterWbs && filterWbs !== '__unpinned__'
          ? filterWbs
          : '',
      parent_ticket_id: parentTicket ? parentTicket.id : '',
      title: '',
      description: parentTicket ? `Sous-ticket rattaché à #${parentTicket.ticket_key} (${parentTicket.title})` : '',
      ticket_type: 'task',
      status: defaultStatus,
      priority: parentTicket ? parentTicket.priority : 'medium',
      assignee_id: parentTicket ? parentTicket.assignee_id : members[0]?.id || '',
      estimated_hours: 8,
      logged_hours: 0,
      remaining_hours: defaultStatus === 'done' ? 0 : 8,
      fixed_cost: 0,
      due_date: parentTicket ? parentTicket.due_date : '',
      tags: '',
    };
    showTicketModal = true;
  }

  function openEditTicket(t) {
    errorMsg = '';
    editingTicket = { ...t };
    showTicketModal = true;
  }

  function handleParentTicketChange() {
    if (editingTicket.parent_ticket_id) {
      const parent = tickets.find((x) => x.id === editingTicket.parent_ticket_id);
      // Si le sous-ticket n'avait pas encore de ligne Gantt assignée, il hérite par défaut de celle de son parent
      if (parent && !editingTicket.wbs_node_id && parent.wbs_node_id) {
        editingTicket.wbs_node_id = parent.wbs_node_id;
      }
    }
  }

  async function saveTicket(e) {
    e.preventDefault();
    errorMsg = '';
    try {
      if (editingTicket.id) {
        await api.updateTicket(editingTicket.id, editingTicket);
      } else {
        await api.createTicket(project.id, editingTicket);
      }
      showTicketModal = false;
      onRefresh?.();
    } catch (err) {
      errorMsg = err.message;
    }
  }

  async function removeTicket(id) {
    try {
      await api.deleteTicket(id);
      showTicketModal = false;
      onRefresh?.();
    } catch (err) {
      errorMsg = err.message;
    }
  }

  function openLogModal(t, e) {
    e?.stopPropagation();
    errorMsg = '';
    activeTicketForLog = t;
    const nextRem = Math.max(0, (t.remaining_hours || 0) - 4);
    newLog = {
      member_id: t.assignee_id || members[0]?.id || '',
      hours: 4,
      work_date: new Date().toISOString().slice(0, 10),
      comment: '',
      remaining_hours: nextRem,
    };
    showTimeLogModal = true;
  }

  async function submitTimeLog(e) {
    e.preventDefault();
    errorMsg = '';
    try {
      await api.addTimeLog(activeTicketForLog.id, newLog);
      showTimeLogModal = false;
      onRefresh?.();
    } catch (err) {
      errorMsg = err.message;
    }
  }

  // Gestion du Drag & Drop entre colonnes Kanban
  function handleDragStart(e, ticket) {
    draggedTicketId = ticket.id;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', ticket.id);
  }

  function handleDragOver(e, colId) {
    e.preventDefault();
    dragOverCol = colId;
  }

  async function handleDrop(e, colId) {
    e.preventDefault();
    const tId = draggedTicketId || e.dataTransfer.getData('text/plain');
    draggedTicketId = null;
    dragOverCol = null;
    if (!tId) return;
    const ticket = tickets.find((x) => x.id === tId);
    if (!ticket || ticket.status === colId) return;
    await api.moveTicket(tId, { status: colId });
    onRefresh?.();
  }
</script>

<div class="flex flex-col h-full space-y-3">
  <!-- Barre de Filtres & Actions -->
  <div class="flex flex-wrap items-center justify-between gap-3 p-3.5 bg-slate-900/90 border border-slate-800 rounded-xl">
    <div class="flex flex-wrap items-center gap-2 text-xs">
      <button
        onclick={() => openNewTicket('todo', null)}
        class="px-3.5 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium shadow-sm transition cursor-pointer"
      >
        + Nouveau Ticket
      </button>

      <!-- Recherche -->
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Rechercher clé, titre, tag..."
        class="px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-700 text-slate-200 placeholder:text-slate-500 w-44"
      />

      <!-- Filtre WBS / Gantt -->
      <select
        bind:value={filterWbs}
        class="px-2.5 py-1.5 rounded-lg bg-slate-950 border border-slate-700 text-slate-200"
      >
        <option value="">📌 Toutes les lignes Gantt + Projet Global</option>
        <option value="__unpinned__">🌐 Tickets rattachés au Projet hors Gantt uniquement</option>
        {#each wbsNodes as n}
          <option value={n.id}>Gantt {n.wbs_code} — {n.title}</option>
        {/each}
      </select>

      <!-- Filtre Hiérarchie Parent / Sous-tickets -->
      <select
        bind:value={filterHierarchy}
        class="px-2.5 py-1.5 rounded-lg bg-slate-950 border border-slate-700 text-slate-200"
      >
        <option value="">🌿 Tous niveaux (Parents + Sous-tickets)</option>
        <option value="parents">Tickets Parents / Principaux uniquement</option>
        <option value="children">↳ Sous-tickets (Enfants) uniquement</option>
      </select>

      <!-- Filtre Nature -->
      <select
        bind:value={filterType}
        class="px-2.5 py-1.5 rounded-lg bg-slate-950 border border-slate-700 text-slate-200"
      >
        <option value="">Toutes natures</option>
        {#each Object.entries(TICKET_TYPES) as [k, v]}
          <option value={k}>{v.icon} {v.label}</option>
        {/each}
      </select>

      <!-- Filtre Intervenant -->
      <select
        bind:value={filterAssignee}
        class="px-2.5 py-1.5 rounded-lg bg-slate-950 border border-slate-700 text-slate-200"
      >
        <option value="">Tous les intervenants</option>
        {#each members as m}
          <option value={m.id}>{m.name}</option>
        {/each}
      </select>

      <label class="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-slate-300 cursor-pointer">
        <input type="checkbox" bind:checked={filterOverBudgetOnly} class="accent-rose-500" />
        <span>En dérive RAF/Budget</span>
      </label>
    </div>

    <div class="flex items-center gap-2 text-xs">
      <div class="flex bg-slate-950 p-0.5 rounded-lg border border-slate-800">
        <button
          onclick={() => (displayMode = 'kanban')}
          class="px-3 py-1 rounded-md transition cursor-pointer {displayMode === 'kanban'
            ? 'bg-indigo-600 text-white font-medium'
            : 'text-slate-400 hover:text-white'}"
        >
          Vue Kanban
        </button>
        <button
          onclick={() => (displayMode = 'list')}
          class="px-3 py-1 rounded-md transition cursor-pointer {displayMode === 'list'
            ? 'bg-indigo-600 text-white font-medium'
            : 'text-slate-400 hover:text-white'}"
        >
          Vue Tableau Financier
        </button>
      </div>
    </div>
  </div>

  {#if displayMode === 'kanban'}
    <!-- Colonnes Kanban -->
    <div class="flex-1 grid grid-cols-1 md:grid-cols-5 gap-3 overflow-x-auto pb-2">
      {#each KANBAN_COLUMNS as col (col.id)}
        {@const colTickets = filteredTickets.filter((t) => t.status === col.id)}
        {@const colLanding = colTickets.reduce((acc, t) => acc + (t.landing_cost || 0), 0)}
        <div
          ondragover={(e) => handleDragOver(e, col.id)}
          ondragleave={() => (dragOverCol = null)}
          ondrop={(e) => handleDrop(e, col.id)}
          class="flex flex-col rounded-xl border p-3 transition {col.color} {dragOverCol === col.id
            ? 'ring-2 ring-indigo-500 bg-indigo-950/30'
            : ''}"
        >
          <!-- En-tête de colonne -->
          <div class="flex items-center justify-between mb-3 pb-2 border-b border-slate-800/80">
            <div class="flex items-center gap-2">
              <span class="w-2.5 h-2.5 rounded-full {col.dot}"></span>
              <h4 class="text-xs font-semibold uppercase tracking-wider text-slate-200">{col.label}</h4>
              <span class="px-1.5 py-0.5 text-[10px] rounded-full bg-slate-800 text-slate-300 font-mono">
                {colTickets.length}
              </span>
            </div>
            <span class="text-[11px] font-mono text-slate-400" title="Atterrissage cumulé de la colonne">
              {formatCurrency(colLanding, project.currency)}
            </span>
          </div>

          <!-- Cartes de la colonne -->
          <div class="flex-1 space-y-2.5 overflow-y-auto pr-0.5">
            {#each colTickets as t (t.id)}
              {@const tType = TICKET_TYPES[t.ticket_type] || TICKET_TYPES.task}
              {@const prio = PRIORITIES[t.priority] || PRIORITIES.medium}
              {@const landingHours = (t.logged_hours || 0) + (t.remaining_hours || 0)}
              {@const progressPct = landingHours > 0 ? Math.min(100, Math.round(((t.logged_hours || 0) / landingHours) * 100)) : 0}
              <div
                draggable="true"
                ondragstart={(e) => handleDragStart(e, t)}
                onclick={() => openEditTicket(t)}
                class="group p-3 rounded-xl bg-slate-950/90 hover:bg-slate-900 border {t.parent_ticket_id
                  ? 'border-l-4 border-l-purple-500/70 border-slate-800/90'
                  : 'border-slate-800/90'} hover:border-indigo-500/50 shadow-sm transition cursor-pointer space-y-2"
              >
                <!-- Badge Enfant (si c'est un sous-ticket) -->
                {#if t.parent_ticket_id && t.parent_ticket_key}
                  <div class="flex items-center gap-1 text-[10px] text-purple-300 font-medium bg-purple-500/10 px-2 py-0.5 rounded border border-purple-500/20">
                    <span>↳ Sous-ticket de</span>
                    <strong class="font-mono">#{t.parent_ticket_key}</strong>
                    <span class="truncate text-purple-400/80">— {t.parent_ticket_title}</span>
                  </div>
                {/if}

                <!-- Ligne supérieure : Clé, Nature & Priorité -->
                <div class="flex items-center justify-between gap-1">
                  <div class="flex items-center gap-1.5">
                    <span class="font-mono text-[11px] font-semibold text-indigo-400">{t.ticket_key}</span>
                    <span class="px-1.5 py-0.5 text-[10px] rounded border {tType.badge}">
                      {tType.icon} {tType.label}
                    </span>
                  </div>
                  <span class="text-[10px] font-medium {prio.color}" title="Priorité {prio.label}">
                    ● {prio.label}
                  </span>
                </div>

                <!-- Titre -->
                <p class="text-xs font-medium text-slate-100 leading-snug line-clamp-2 group-hover:text-indigo-200">
                  {t.title}
                </p>

                <!-- Badges Rattachement Gantt + Compteur Sous-tickets -->
                <div class="flex flex-wrap items-center gap-1.5">
                  {#if t.wbs_node_id}
                    <span
                      class="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-indigo-500/15 border border-indigo-500/30 text-[10px] text-indigo-300"
                      title="Épinglé sur la ligne Gantt {t.wbs_node_code} ({t.wbs_node_title})"
                    >
                      📌 Gantt {t.wbs_node_code} : {t.wbs_node_title}
                    </span>
                  {:else}
                    <span
                      class="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-slate-800/90 border border-slate-700 text-[10px] text-slate-400"
                      title="Rattaché globalement à l'environnement financier du projet (hors tâche Gantt)"
                    >
                      🌐 Projet Global
                    </span>
                  {/if}

                  {#if t.sub_ticket_cnt > 0}
                    <span
                      class="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-purple-500/15 border border-purple-500/30 text-[10px] text-purple-300 font-medium"
                      title="Ce ticket parent possède {t.sub_ticket_cnt} sous-ticket(s). Coût consolidé Parent+Enfants : {formatCurrency(t.consolidated_landing, project.currency)}"
                    >
                      🌿 {t.done_sub_ticket_cnt}/{t.sub_ticket_cnt} sous-tickets • Consolidé: {formatCurrency(t.consolidated_landing, project.currency)}
                    </span>
                  {/if}
                </div>

                <!-- Encart Temps & Coûts (Consommé + RAF = Atterrissage) -->
                <div class="p-2 rounded-lg bg-slate-900/90 border border-slate-800/80 space-y-1.5 text-[11px]">
                  <div class="flex items-center justify-between">
                    <span class="text-slate-400">
                      Conso: <strong class="text-slate-200">{t.logged_hours}h</strong> • RAF:
                      <strong class="text-amber-300">{t.remaining_hours}h</strong>
                    </span>
                    <span
                      class="font-mono font-semibold {t.is_over_budget ? 'text-rose-400' : 'text-emerald-400'}"
                      title="Atterrissage propre du ticket ({formatCurrency(t.landing_cost)}) vs Budget initial ({formatCurrency(t.initial_cost)})"
                    >
                      {formatCurrency(t.landing_cost, project.currency)}
                    </span>
                  </div>

                  <div class="w-full h-1.5 bg-slate-800 rounded-full overflow-hidden flex">
                    <div
                      style="width: {progressPct}%"
                      class="h-full {t.is_over_budget ? 'bg-rose-500' : 'bg-indigo-500'}"
                    ></div>
                  </div>

                  <div class="flex items-center justify-between text-[10px] text-slate-500">
                    <span>Budget init: {formatCurrency(t.initial_cost, project.currency)} ({t.estimated_hours}h)</span>
                    {#if t.is_over_budget}
                      <span class="text-rose-400 font-medium">⚠️ Dépassement</span>
                    {/if}
                  </div>
                </div>

                <!-- Pied de carte : Intervenant, Échéance, Bouton Sous-ticket & Bouton Pointage -->
                <div class="flex items-center justify-between pt-1 text-[11px] text-slate-400">
                  <span class="truncate">{t.assignee_name || 'Non assigné'}</span>
                  <div class="flex items-center gap-1">
                    <button
                      type="button"
                      onclick={(e) => {
                        e.stopPropagation();
                        openNewTicket('todo', t);
                      }}
                      class="px-1.5 py-0.5 rounded bg-purple-500/15 hover:bg-purple-500/30 text-purple-300 text-[10px] font-medium cursor-pointer"
                      title="Créer un sous-ticket enfant rattaché à {t.ticket_key}"
                    >
                      +Enfant
                    </button>
                    <button
                      type="button"
                      onclick={(e) => openLogModal(t, e)}
                      class="px-2 py-0.5 rounded bg-indigo-500/20 hover:bg-indigo-500/40 text-indigo-300 text-[10px] font-medium cursor-pointer"
                      title="Pointer du temps et mettre à jour le Reste à faire (RAF)"
                    >
                      ⏱️ +Temps/RAF
                    </button>
                  </div>
                </div>
              </div>
            {/each}

            <button
              onclick={() => openNewTicket(col.id, null)}
              class="w-full py-2 rounded-lg border border-dashed border-slate-800 hover:border-slate-700 text-slate-500 hover:text-slate-300 text-xs transition cursor-pointer"
            >
              + Ajouter dans {col.label}
            </button>
          </div>
        </div>
      {/each}
    </div>
  {:else}
    <!-- Vue Tableau Financier & Opérationnel -->
    <div class="flex-1 bg-slate-900/90 border border-slate-800 rounded-xl overflow-auto">
      <table class="w-full text-left border-collapse text-xs">
        <thead>
          <tr class="border-b border-slate-800 bg-slate-950/80 text-[11px] uppercase text-slate-400">
            <th class="py-3 px-3">Clé & Titre</th>
            <th class="py-3 px-3">Parent / Enfants</th>
            <th class="py-3 px-3">Nature</th>
            <th class="py-3 px-3">Rattachement Timeline Gantt</th>
            <th class="py-3 px-3">Statut</th>
            <th class="py-3 px-3">Intervenant</th>
            <th class="py-3 px-3 text-right">Estimé Init.</th>
            <th class="py-3 px-3 text-right">Consommé</th>
            <th class="py-3 px-3 text-right">RAF</th>
            <th class="py-3 px-3 text-right">Atterrissage</th>
            <th class="py-3 px-3 text-right">Action</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/70">
          {#each filteredTickets as t (t.id)}
            {@const tType = TICKET_TYPES[t.ticket_type] || TICKET_TYPES.task}
            <tr class="hover:bg-slate-800/40 transition">
              <td class="py-2.5 px-3">
                <button onclick={() => openEditTicket(t)} class="text-left hover:text-indigo-400 cursor-pointer">
                  <span class="font-mono text-indigo-400 font-semibold mr-1.5">{t.ticket_key}</span>
                  <span class="text-slate-100 font-medium">{t.title}</span>
                </button>
              </td>
              <td class="py-2.5 px-3 text-[11px]">
                {#if t.parent_ticket_key}
                  <span class="text-purple-300 font-mono">↳ Enfant de #{t.parent_ticket_key}</span>
                {:else if t.sub_ticket_cnt > 0}
                  <span class="text-purple-300 font-medium">
                    🌿 Parent ({t.done_sub_ticket_cnt}/{t.sub_ticket_cnt}) • Consolidé: {formatCurrency(t.consolidated_landing, project.currency)}
                  </span>
                {:else}
                  <span class="text-slate-600">—</span>
                {/if}
              </td>
              <td class="py-2.5 px-3">
                <span class="px-2 py-0.5 rounded text-[10px] border {tType.badge}">{tType.icon} {tType.label}</span>
              </td>
              <td class="py-2.5 px-3">
                {#if t.wbs_node_id}
                  <span class="text-indigo-300 font-medium">📌 {t.wbs_node_code} — {t.wbs_node_title}</span>
                {:else}
                  <span class="text-slate-400">🌐 Projet Global</span>
                {/if}
              </td>
              <td class="py-2.5 px-3 uppercase text-[10px] font-semibold text-slate-300">{t.status}</td>
              <td class="py-2.5 px-3 text-slate-300">
                {t.assignee_name || '—'} <span class="text-slate-500">({t.effective_rate}€/h)</span>
              </td>
              <td class="py-2.5 px-3 text-right font-mono text-slate-300">
                {formatCurrency(t.initial_cost, project.currency)}
              </td>
              <td class="py-2.5 px-3 text-right font-mono text-sky-300">
                {formatCurrency(t.consumed_cost, project.currency)}
              </td>
              <td class="py-2.5 px-3 text-right font-mono text-amber-300">
                {formatCurrency(t.remaining_cost, project.currency)}
              </td>
              <td class="py-2.5 px-3 text-right font-mono font-semibold text-white">
                {formatCurrency(t.landing_cost, project.currency)}
              </td>
              <td class="py-2.5 px-3 text-right space-x-1">
                <button
                  onclick={() => openNewTicket('todo', t)}
                  class="px-2 py-1 rounded bg-purple-600/30 hover:bg-purple-600 text-purple-200 text-[10px] cursor-pointer"
                >
                  +Enfant
                </button>
                <button
                  onclick={(e) => openLogModal(t, e)}
                  class="px-2 py-1 rounded bg-indigo-600/30 hover:bg-indigo-600 text-indigo-200 text-[10px] cursor-pointer"
                >
                  ⏱️ Pointer
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<!-- Modale : Création / Édition d'un Ticket (avec Rattachement Gantt + Gestion des Sous-tickets Enfants) -->
{#if showTicketModal}
  <div class="fixed inset-0 bg-black/70 backdrop-blur-xs flex items-center justify-center z-50 p-4 overflow-y-auto">
    <form
      onsubmit={saveTicket}
      class="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-2xl p-6 space-y-4 shadow-2xl text-xs my-8"
    >
      <div class="flex items-center justify-between border-b border-slate-800 pb-3">
        <div>
          <h3 class="text-base font-semibold text-white">
            {editingTicket.id
              ? `Ticket ${editingTicket.ticket_key} — ${editingTicket.title}`
              : editingTicket.parent_ticket_id
                ? `Nouveau Sous-ticket (Enfant) dans le projet ${project.code}`
                : `Nouveau Ticket rattaché au projet ${project.code}`}
          </h3>
        </div>
        <button type="button" onclick={() => (showTicketModal = false)} class="text-slate-400 hover:text-white cursor-pointer">✕</button>
      </div>

      {#if errorMsg}
        <div class="p-2.5 rounded-lg bg-rose-500/20 border border-rose-500/40 text-rose-300">{errorMsg}</div>
      {/if}

      <div class="grid grid-cols-3 gap-3">
        <div class="col-span-2">
          <label class="block text-slate-400 mb-1">Titre du Ticket</label>
          <input
            type="text"
            required
            bind:value={editingTicket.title}
            placeholder="Ex: Implémenter le connecteur de facturation..."
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Nature du Ticket</label>
          <select
            bind:value={editingTicket.ticket_type}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          >
            {#each Object.entries(TICKET_TYPES) as [k, v]}
              <option value={k}>{v.icon} {v.label}</option>
            {/each}
          </select>
        </div>
      </div>

      <!-- Double Rattachement : 1. Timeline Gantt (Optionnel) & 2. Ticket Parent (Optionnel) -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div class="p-3 rounded-xl bg-indigo-950/30 border border-indigo-500/30 space-y-1.5">
          <label class="block font-semibold text-indigo-300">
            📌 Épinglage Timeline MS Project / Gantt
          </label>
          <select
            bind:value={editingTicket.wbs_node_id}
            class="w-full px-2.5 py-2 rounded-lg bg-slate-950 border border-indigo-500/40 text-white"
          >
            <option value="">🌐 Projet Global (Hors tâche Gantt)</option>
            {#each wbsNodes as n}
              <option value={n.id}>
                📌 Gantt {n.wbs_node_code || n.wbs_code} — {n.title}
              </option>
            {/each}
          </select>
        </div>

        <div class="p-3 rounded-xl bg-purple-950/25 border border-purple-500/30 space-y-1.5">
          <label class="block font-semibold text-purple-300">
            🌿 Ticket Parent (Optionnel — Sous-ticket)
          </label>
          <select
            bind:value={editingTicket.parent_ticket_id}
            onchange={handleParentTicketChange}
            class="w-full px-2.5 py-2 rounded-lg bg-slate-950 border border-purple-500/40 text-white"
          >
            <option value="">— Aucun (Ticket Principal / Racine) —</option>
            {#each tickets.filter((x) => x.id !== editingTicket.id) as pt}
              <option value={pt.id}>
                ↳ Enfant de #{pt.ticket_key} — {pt.title}
              </option>
            {/each}
          </select>
        </div>
      </div>

      <div class="grid grid-cols-4 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Colonne Kanban</label>
          <select bind:value={editingTicket.status} class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white">
            {#each KANBAN_COLUMNS as c}
              <option value={c.id}>{c.label}</option>
            {/each}
          </select>
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Priorité</label>
          <select bind:value={editingTicket.priority} class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white">
            {#each Object.entries(PRIORITIES) as [k, v]}
              <option value={k}>{v.label}</option>
            {/each}
          </select>
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Intervenant assigné</label>
          <select bind:value={editingTicket.assignee_id} class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white">
            <option value="">— Taux projet par défaut ({project.default_hourly_rate}€/h) —</option>
            {#each members as m}
              <option value={m.id}>{m.name} ({m.hourly_rate}€/h)</option>
            {/each}
          </select>
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Date d'échéance</label>
          <input
            type="date"
            bind:value={editingTicket.due_date}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
      </div>

      <!-- Section Financière & Temps du Ticket -->
      <div class="p-3.5 rounded-xl bg-slate-950 border border-slate-800 space-y-3">
        <div class="flex items-center justify-between">
          <span class="font-semibold text-slate-200">💰 Environnement Temps & Coûts propres à ce Ticket</span>
          {#if editingTicket.sub_ticket_cnt > 0}
            <span class="text-purple-300 font-mono text-[11px]">
              Atterrissage Consolidé (Parent + {editingTicket.sub_ticket_cnt} sous-tickets) :
              <strong>{formatCurrency(editingTicket.consolidated_landing, project.currency)}</strong>
            </span>
          {/if}
        </div>
        <div class="grid grid-cols-4 gap-3">
          <div>
            <label class="block text-slate-400 mb-1">Estimé Initial (Heures)</label>
            <input
              type="number"
              step="0.5"
              min="0"
              bind:value={editingTicket.estimated_hours}
              class="w-full px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-white"
            />
          </div>
          <div>
            <label class="block text-slate-400 mb-1">Temps Consommé (h)</label>
            <input
              type="number"
              step="0.5"
              min="0"
              bind:value={editingTicket.logged_hours}
              class="w-full px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-sky-300"
            />
          </div>
          <div>
            <label class="block text-amber-300 mb-1 font-medium">Reste à faire - RAF (h)</label>
            <input
              type="number"
              step="0.5"
              min="0"
              bind:value={editingTicket.remaining_hours}
              class="w-full px-3 py-1.5 rounded-lg bg-slate-900 border border-amber-500/50 text-amber-300 font-semibold"
            />
          </div>
          <div>
            <label class="block text-slate-400 mb-1">Coût Fixe Additionnel (€)</label>
            <input
              type="number"
              step="50"
              min="0"
              bind:value={editingTicket.fixed_cost}
              class="w-full px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-white"
            />
          </div>
        </div>
      </div>

      <div>
        <label class="block text-slate-400 mb-1">Description & Spécifications</label>
        <textarea
          rows="2"
          bind:value={editingTicket.description}
          class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
        ></textarea>
      </div>

      <!-- Section : Liste des Sous-tickets (Tickets Enfants) rattachés à ce Ticket -->
      {#if editingTicket.id}
        <div class="p-3.5 rounded-xl bg-purple-950/15 border border-purple-500/30 space-y-2.5">
          <div class="flex items-center justify-between">
            <span class="font-semibold text-purple-300">
              🌿 Sous-tickets rattachés ({currentSubTickets.length})
            </span>
            <button
              type="button"
              onclick={() => openNewTicket('todo', editingTicket)}
              class="px-2.5 py-1 rounded-lg bg-purple-600 hover:bg-purple-500 text-white text-[11px] font-medium cursor-pointer"
            >
              + Créer un sous-ticket enfant
            </button>
          </div>

          {#if currentSubTickets.length > 0}
            <div class="space-y-1.5 max-h-36 overflow-y-auto">
              {#each currentSubTickets as sub (sub.id)}
                <div
                  onclick={() => openEditTicket(sub)}
                  class="flex items-center justify-between px-3 py-2 rounded-lg bg-slate-950 border border-slate-800 hover:border-purple-500/50 cursor-pointer transition"
                >
                  <div class="flex items-center gap-2 truncate pr-2">
                    <span class="font-mono font-bold text-purple-400">{sub.ticket_key}</span>
                    <span class="text-slate-100 font-medium truncate">{sub.title}</span>
                    <span class="px-1.5 py-0.5 rounded bg-slate-800 text-[10px] uppercase text-slate-300">{sub.status}</span>
                  </div>
                  <div class="text-right font-mono text-[11px] shrink-0">
                    <span class="text-slate-400">RAF: {sub.remaining_hours}h •</span>
                    <strong class="text-white ml-1">{formatCurrency(sub.landing_cost, project.currency)}</strong>
                  </div>
                </div>
              {/each}
            </div>
          {:else}
            <p class="text-[11px] text-slate-400">
              Aucun sous-ticket pour le moment. Cliquez sur "+ Créer un sous-ticket enfant" pour découper ce ticket.
            </p>
          {/if}
        </div>
      {/if}

      <!-- Historique des Pointages sur ce Ticket -->
      {#if editingTicket.id && timeLogs[editingTicket.id]?.length > 0}
        <div class="border-t border-slate-800 pt-3">
          <div class="text-slate-400 font-medium mb-2">Historique des pointages sur ce ticket :</div>
          <div class="max-h-24 overflow-y-auto space-y-1">
            {#each timeLogs[editingTicket.id] as l (l.id)}
              <div class="flex items-center justify-between px-2.5 py-1.5 rounded bg-slate-950 text-[11px]">
                <span>
                  <strong class="text-slate-200">{l.member_name}</strong> • {l.work_date} : {l.comment || 'Travail sur le ticket'}
                </span>
                <span class="font-mono text-sky-300">
                  {l.hours}h × {l.hourly_rate_snapshot}€ = {formatCurrency(l.cost, project.currency)}
                </span>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <div class="flex items-center justify-between pt-3 border-t border-slate-800">
        {#if editingTicket.id}
          <button
            type="button"
            onclick={() => removeTicket(editingTicket.id)}
            class="px-3 py-2 rounded-lg bg-rose-500/20 hover:bg-rose-500/30 text-rose-300 font-medium cursor-pointer"
          >
            Supprimer le ticket
          </button>
        {:else}
          <div></div>
        {/if}
        <div class="flex gap-2">
          <button
            type="button"
            onclick={() => (showTicketModal = false)}
            class="px-4 py-2 rounded-lg bg-slate-800 text-slate-300 cursor-pointer"
          >
            Annuler
          </button>
          <button
            type="submit"
            class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium cursor-pointer"
          >
            Enregistrer & Mettre à jour l'Atterrissage
          </button>
        </div>
      </div>
    </form>
  </div>
{/if}

<!-- Modale : Pointage Rapide (TimeLog) + Ajustement du Reste à Faire (RAF) -->
{#if showTimeLogModal && activeTicketForLog}
  <div class="fixed inset-0 bg-black/70 backdrop-blur-xs flex items-center justify-center z-50 p-4">
    <form
      onsubmit={submitTimeLog}
      class="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-md p-6 space-y-4 shadow-2xl text-xs"
    >
      <div class="flex items-center justify-between border-b border-slate-800 pb-3">
        <div>
          <h3 class="text-sm font-semibold text-white">⏱️ Pointer du temps & Ajuster le RAF</h3>
          <p class="text-[11px] text-indigo-400">{activeTicketForLog.ticket_key} — {activeTicketForLog.title}</p>
        </div>
        <button type="button" onclick={() => (showTimeLogModal = false)} class="text-slate-400 hover:text-white cursor-pointer">✕</button>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Intervenant</label>
          <select bind:value={newLog.member_id} class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white">
            {#each members as m}
              <option value={m.id}>{m.name} ({m.hourly_rate}€/h)</option>
            {/each}
          </select>
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Date d'intervention</label>
          <input
            type="date"
            required
            bind:value={newLog.work_date}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3 p-3 rounded-xl bg-slate-950 border border-slate-800">
        <div>
          <label class="block text-sky-300 font-medium mb-1">Heures pointées (+Consommé)</label>
          <input
            type="number"
            step="0.5"
            min="0.5"
            required
            bind:value={newLog.hours}
            oninput={() => {
              newLog.remaining_hours = Math.max(0, (activeTicketForLog.remaining_hours || 0) - (newLog.hours || 0));
            }}
            class="w-full px-3 py-2 rounded-lg bg-slate-900 border border-sky-500/40 text-white font-semibold"
          />
        </div>
        <div>
          <label class="block text-amber-300 font-medium mb-1">Nouveau Reste à Faire (RAF h)</label>
          <input
            type="number"
            step="0.5"
            min="0"
            required
            bind:value={newLog.remaining_hours}
            class="w-full px-3 py-2 rounded-lg bg-slate-900 border border-amber-500/40 text-amber-300 font-semibold"
          />
        </div>
      </div>

      <div>
        <label class="block text-slate-400 mb-1">Commentaire d'avancement</label>
        <input
          type="text"
          bind:value={newLog.comment}
          placeholder="Ex: Développement terminé, reste les tests unitaires..."
          class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
        />
      </div>

      <div class="flex justify-end gap-2 pt-3 border-t border-slate-800">
        <button type="button" onclick={() => (showTimeLogModal = false)} class="px-4 py-2 rounded-lg bg-slate-800 text-slate-300 cursor-pointer">
          Annuler
        </button>
        <button type="submit" class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium cursor-pointer">
          Valider le pointage
        </button>
      </div>
    </form>
  </div>
{/if}
