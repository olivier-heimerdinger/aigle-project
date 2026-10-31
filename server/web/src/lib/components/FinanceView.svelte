<script>
  import { api, formatCurrency, formatHours } from '../api.js';

  let {
    project,
    financials = {},
    wbsNodes = [],
    tickets = [],
    members = [],
    onRefresh,
  } = $props();

  let showMemberModal = $state(false);
  let editingMember = $state({
    id: '',
    name: '',
    email: '',
    role: '',
    hourly_rate: 70,
    avatar_color: '#6366f1',
  });

  let editingBudget = $state(false);
  let budgetDraft = $state(0);
  let rateDraft = $state(65);

  function startEditProjectBudget() {
    budgetDraft = project.initial_budget;
    rateDraft = project.default_hourly_rate;
    editingBudget = true;
  }

  async function saveProjectBudget() {
    await api.updateProject(project.id, {
      ...project,
      initial_budget: Number(budgetDraft),
      default_hourly_rate: Number(rateDraft),
    });
    editingBudget = false;
    onRefresh?.();
  }

  function openNewMember() {
    editingMember = { id: '', name: '', email: '', role: 'Ingénieur', hourly_rate: 70, avatar_color: '#6366f1' };
    showMemberModal = true;
  }

  function openEditMember(m) {
    editingMember = { ...m };
    showMemberModal = true;
  }

  async function saveMember(e) {
    e.preventDefault();
    if (editingMember.id) {
      await api.updateMember(editingMember.id, editingMember);
    } else {
      await api.createMember(editingMember);
    }
    showMemberModal = false;
    onRefresh?.();
  }

  let unpinnedTickets = $derived(tickets.filter((t) => !t.wbs_node_id));
</script>

<div class="space-y-6 pb-8">
  <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
    <div class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-1">
      <div class="flex items-center justify-between text-xs text-slate-400">
        <span>Budget Initial Alloué</span>
        <button onclick={startEditProjectBudget} class="text-indigo-400 hover:underline text-[11px] cursor-pointer">
          Modifier
        </button>
      </div>
      {#if editingBudget}
        <div class="space-y-2 pt-1">
          <input
            type="number"
            step="500"
            bind:value={budgetDraft}
            class="w-full px-2 py-1 rounded bg-slate-950 border border-indigo-500 text-sm text-white font-mono"
          />
          <div class="flex gap-1">
            <button onclick={saveProjectBudget} class="px-2 py-1 rounded bg-indigo-600 text-white text-[11px] cursor-pointer">OK</button>
            <button onclick={() => (editingBudget = false)} class="px-2 py-1 rounded bg-slate-800 text-slate-300 text-[11px] cursor-pointer">✕</button>
          </div>
        </div>
      {:else}
        <div class="text-2xl font-bold font-mono text-white">
          {formatCurrency(financials.initial_budget, project.currency)}
        </div>
        <div class="text-[11px] text-slate-500">
          Estimé tickets : {formatCurrency(financials.estimated_cost, project.currency)} ({formatHours(financials.estimated_hours)})
        </div>
      {/if}
    </div>

    <div class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-1">
      <div class="text-xs text-slate-400">Consommé Réel (AC)</div>
      <div class="text-2xl font-bold font-mono text-sky-400">
        {formatCurrency(financials.consumed_cost, project.currency)}
      </div>
      <div class="text-[11px] text-slate-400">
        {formatHours(financials.logged_hours)} pointées ({financials.consumption_ratio || 0}% du budget)
      </div>
    </div>

    <div class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-1">
      <div class="text-xs text-slate-400">Reste à Faire — RAF (ETC)</div>
      <div class="text-2xl font-bold font-mono text-amber-400">
        {formatCurrency(financials.remaining_cost, project.currency)}
      </div>
      <div class="text-[11px] text-slate-400">
        {formatHours(financials.remaining_hours)} restantes estimées
      </div>
    </div>

    <div class="p-4 rounded-xl bg-slate-900 border border-indigo-500/40 space-y-1">
      <div class="text-xs text-indigo-300 font-medium">Atterrissage Prévu (Consommé + RAF)</div>
      <div class="text-2xl font-bold font-mono text-white">
        {formatCurrency(financials.landing_cost, project.currency)}
      </div>
      <div class="text-[11px] text-slate-400">
        Total {formatHours(financials.landing_hours)} ({financials.landing_ratio || 0}% du budget)
      </div>
    </div>

    <div
      class="p-4 rounded-xl border space-y-1 {financials.variance >= 0
        ? 'bg-emerald-950/20 border-emerald-500/40'
        : 'bg-rose-950/25 border-rose-500/40'}"
    >
      <div class="text-xs text-slate-300">Écart à Terminaison (Marge / Dérive)</div>
      <div class="text-2xl font-bold font-mono {financials.variance >= 0 ? 'text-emerald-400' : 'text-rose-400'}">
        {financials.variance >= 0 ? '+' : ''}{formatCurrency(financials.variance, project.currency)}
      </div>
      <div class="text-[11px] {financials.variance >= 0 ? 'text-emerald-300/80' : 'text-rose-300'}">
        {financials.variance >= 0
          ? `Sous le budget (${financials.variance_percent}%)`
          : `Dépassement prévu (${financials.variance_percent}%)`}
      </div>
    </div>
  </div>

  <div class="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-2">
    <div class="flex items-center justify-between text-xs">
      <span class="font-semibold text-slate-200">
        Jauge d'Atterrissage Financier (Consommé + Reste à faire vs Budget Initial)
      </span>
      <span class="text-slate-400">
        Consommé: <strong class="text-sky-400">{formatCurrency(financials.consumed_cost, project.currency)}</strong> + RAF:
        <strong class="text-amber-400">{formatCurrency(financials.remaining_cost, project.currency)}</strong> = Atterrissage:
        <strong class="text-white">{formatCurrency(financials.landing_cost, project.currency)}</strong>
      </span>
    </div>
    {#if financials.initial_budget > 0}
      {@const maxRef = Math.max(financials.initial_budget, financials.landing_cost, 1)}
      {@const consumedW = Math.round((financials.consumed_cost / maxRef) * 100)}
      {@const rafW = Math.round((financials.remaining_cost / maxRef) * 100)}
      {@const budgetMarker = Math.round((financials.initial_budget / maxRef) * 100)}
      <div class="relative h-5 bg-slate-950 rounded-lg overflow-hidden border border-slate-800 flex">
        <div style="width: {consumedW}%" class="h-full bg-sky-500 transition-all" title="Consommé"></div>
        <div style="width: {rafW}%" class="h-full bg-amber-500/85 transition-all" title="Reste à faire (RAF)"></div>
        <div
          style="left: {budgetMarker}%"
          class="absolute top-0 bottom-0 w-0.5 bg-white shadow"
          title="Plafond Budget Initial"
        ></div>
      </div>
    {/if}
  </div>

  <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
    <div class="px-4 py-3 border-b border-slate-800 flex items-center justify-between">
      <div>
        <h3 class="text-sm font-semibold text-white">
          Ventilation Financière par Phase / Lot du Gantt & Tickets Transverses
        </h3>
        <p class="text-xs text-slate-400">
          Consolidation automatique des tickets épinglés sur chaque ligne WBS et des tickets globaux rattachés au projet.
        </p>
      </div>
    </div>

    <table class="w-full text-left border-collapse text-xs">
      <thead>
        <tr class="border-b border-slate-800 bg-slate-950/70 text-[11px] uppercase text-slate-400">
          <th class="py-3 px-4">Code & Lot WBS / Périmètre</th>
          <th class="py-3 px-3 text-center">Tickets</th>
          <th class="py-3 px-3 text-right">Budget Alloué</th>
          <th class="py-3 px-3 text-right">Consommé (Heures)</th>
          <th class="py-3 px-3 text-right">RAF (Heures)</th>
          <th class="py-3 px-3 text-right">Atterrissage Prévu</th>
          <th class="py-3 px-4 text-right">Écart à Terminaison</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-slate-800/60">
        {#each wbsNodes as n (n.id)}
          {@const f = n.financials || {}}
          {@const isPhase = n.node_type === 'phase'}
          <tr class="{isPhase ? 'bg-slate-950/40 font-semibold text-white' : 'text-slate-300'} hover:bg-slate-800/40">
            <td class="py-2.5 px-4" style="padding-left: {n.parent_id ? '32px' : '16px'}">
              <span class="font-mono text-indigo-400 mr-1.5">{n.wbs_code}</span>
              <span>{n.title}</span>
            </td>
            <td class="py-2.5 px-3 text-center font-mono text-[11px]">
              {n.pinned_ticket_cnt > 0 ? `${n.done_ticket_cnt}/${n.pinned_ticket_cnt}` : '—'}
            </td>
            <td class="py-2.5 px-3 text-right font-mono">
              {formatCurrency(f.initial_budget, project.currency)}
            </td>
            <td class="py-2.5 px-3 text-right font-mono text-sky-300">
              {formatCurrency(f.consumed_cost, project.currency)}
              <span class="text-slate-500 text-[10px]">({f.logged_hours || 0}h)</span>
            </td>
            <td class="py-2.5 px-3 text-right font-mono text-amber-300">
              {formatCurrency(f.remaining_cost, project.currency)}
              <span class="text-slate-500 text-[10px]">({f.remaining_hours || 0}h)</span>
            </td>
            <td class="py-2.5 px-3 text-right font-mono font-semibold text-white">
              {formatCurrency(f.landing_cost, project.currency)}
            </td>
            <td
              class="py-2.5 px-4 text-right font-mono font-semibold {(f.variance || 0) >= 0
                ? 'text-emerald-400'
                : 'text-rose-400'}"
            >
              {(f.variance || 0) >= 0 ? '+' : ''}{formatCurrency(f.variance, project.currency)}
            </td>
          </tr>
        {/each}

        <tr class="bg-indigo-950/20 font-medium text-indigo-200 border-t-2 border-slate-700">
          <td class="py-3 px-4">
            🌐 Tickets Transverses (Rattachés globalement au Projet hors tâche Gantt)
          </td>
          <td class="py-3 px-3 text-center font-mono">{unpinnedTickets.length}</td>
          <td class="py-3 px-3 text-right font-mono text-slate-400">Sur budget global</td>
          <td class="py-3 px-3 text-right font-mono text-sky-300">
            {formatCurrency(financials.unpinned_consumed, project.currency)}
          </td>
          <td class="py-3 px-3 text-right font-mono text-amber-300">
            {formatCurrency(financials.unpinned_remaining, project.currency)}
          </td>
          <td class="py-3 px-3 text-right font-mono font-semibold text-white">
            {formatCurrency(financials.unpinned_landing, project.currency)}
          </td>
          <td class="py-3 px-4 text-right font-mono text-slate-400">—</td>
        </tr>
      </tbody>
    </table>
  </div>

  <div class="bg-slate-900 border border-slate-800 rounded-xl p-4 space-y-3">
    <div class="flex items-center justify-between">
      <div>
        <h3 class="text-sm font-semibold text-white">Équipe & Grille Tarifaire (THM / TJM)</h3>
        <p class="text-xs text-slate-400">
          Les coûts consommés et le Reste à faire (RAF) des tickets sont valorisés automatiquement selon le taux de l'intervenant assigné.
        </p>
      </div>
      <button
        onclick={openNewMember}
        class="px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium cursor-pointer"
      >
        + Nouvel Intervenant
      </button>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-3">
      {#each members as m (m.id)}
        <div
          onclick={() => openEditMember(m)}
          class="p-3 rounded-xl bg-slate-950 border border-slate-800 hover:border-indigo-500/40 flex items-center justify-between cursor-pointer transition"
        >
          <div class="flex items-center gap-2.5">
            <div
              style="background-color: {m.avatar_color}"
              class="w-8 h-8 rounded-full flex items-center justify-center text-white font-bold text-xs"
            >
              {m.name.slice(0, 2).toUpperCase()}
            </div>
            <div>
              <div class="text-xs font-semibold text-white">{m.name}</div>
              <div class="text-[11px] text-slate-400">{m.role}</div>
            </div>
          </div>
          <div class="text-right font-mono">
            <div class="text-xs font-bold text-indigo-400">{m.hourly_rate} €/h</div>
            <div class="text-[10px] text-slate-500">{m.hourly_rate * 8} €/j</div>
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>

{#if showMemberModal}
  <div class="fixed inset-0 bg-black/70 backdrop-blur-xs flex items-center justify-center z-50 p-4">
    <form
      onsubmit={saveMember}
      class="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-md p-6 space-y-4 shadow-2xl text-xs"
    >
      <div class="flex items-center justify-between border-b border-slate-800 pb-3">
        <h3 class="text-base font-semibold text-white">
          {editingMember.id ? `Modifier ${editingMember.name}` : 'Nouvel Intervenant'}
        </h3>
        <button type="button" onclick={() => (showMemberModal = false)} class="text-slate-400 hover:text-white cursor-pointer">✕</button>
      </div>

      <div>
        <label class="block text-slate-400 mb-1">Nom complet</label>
        <input
          type="text"
          required
          bind:value={editingMember.name}
          class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
        />
      </div>

      <div>
        <label class="block text-slate-400 mb-1">Rôle / Profil</label>
        <input
          type="text"
          bind:value={editingMember.role}
          class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
        />
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Taux Horaire (€/h)</label>
          <input
            type="number"
            step="1"
            required
            bind:value={editingMember.hourly_rate}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white font-mono"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Équivalent TJM (8h/j)</label>
          <input
            type="text"
            disabled
            value="{(editingMember.hourly_rate || 0) * 8} € / jour"
            class="w-full px-3 py-2 rounded-lg bg-slate-950/50 border border-slate-800 text-slate-400 font-mono"
          />
        </div>
      </div>

      <div class="flex justify-end gap-2 pt-3 border-t border-slate-800">
        <button type="button" onclick={() => (showMemberModal = false)} class="px-4 py-2 rounded-lg bg-slate-800 text-slate-300 cursor-pointer">
          Annuler
        </button>
        <button type="submit" class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium cursor-pointer">
          Enregistrer & Recalculer les coûts
        </button>
      </div>
    </form>
  </div>
{/if}

