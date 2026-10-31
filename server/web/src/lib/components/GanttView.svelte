<script>
  import { api, formatCurrency } from '../api.js';

  let {
    project,
    wbsNodes = [],
    dependencies = [],
    members = [],
    onRefresh,
    onJumpToTickets,
  } = $props();

  let zoomMode = $state('day'); // 'day' | 'week'
  let showCriticalPath = $state(true);
  let showNodeModal = $state(false);
  let showDepModal = $state(false);
  let errorMsg = $state('');

  let editingNode = $state({
    id: '',
    parent_id: '',
    wbs_code: '',
    title: '',
    node_type: 'task',
    start_date: '',
    duration_days: 5,
    progress: 0,
    allocated_budget: 0,
    estimated_hours: 40,
    assigned_member_id: '',
    order_index: 1,
  });

  let newDep = $state({
    predecessor_id: '',
    successor_id: '',
    dep_type: 'FS',
    lag_days: 0,
  });

  const DAY_MS = 86400000;
  let dayWidth = $derived(zoomMode === 'day' ? 30 : 14);
  const ROW_HEIGHT = 44;

  function parseDateUTC(str) {
    if (!str) return new Date();
    const [y, m, d] = str.split('-').map(Number);
    return new Date(Date.UTC(y, (m || 1) - 1, d || 1));
  }

  function formatDateUTC(dt) {
    return dt.toISOString().slice(0, 10);
  }

  let timelineBounds = $derived.by(() => {
    if (!wbsNodes || wbsNodes.length === 0) {
      const start = parseDateUTC(project?.start_date || '2026-10-15');
      const end = new Date(start.getTime() + 45 * DAY_MS);
      return { minDate: start, maxDate: end, totalDays: 45 };
    }
    let minMs = Infinity;
    let maxMs = -Infinity;
    for (const n of wbsNodes) {
      const s = parseDateUTC(n.start_date).getTime();
      const e = parseDateUTC(n.end_date).getTime();
      if (s < minMs) minMs = s;
      if (e > maxMs) maxMs = e;
    }
    minMs -= 4 * DAY_MS;
    maxMs += 10 * DAY_MS;
    const totalDays = Math.max(30, Math.round((maxMs - minMs) / DAY_MS) + 1);
    return {
      minDate: new Date(minMs),
      maxDate: new Date(minMs + totalDays * DAY_MS),
      totalDays,
    };
  });

  let calendarDays = $derived.by(() => {
    const list = [];
    const { minDate, totalDays } = timelineBounds;
    for (let i = 0; i < totalDays; i++) {
      const dt = new Date(minDate.getTime() + i * DAY_MS);
      const dayOfWeek = dt.getUTCDay();
      list.push({
        index: i,
        dateStr: formatDateUTC(dt),
        dayNum: dt.getUTCDate(),
        monthShort: dt.toLocaleDateString('fr-FR', { month: 'short', timeZone: 'UTC' }),
        isWeekend: dayOfWeek === 0 || dayOfWeek === 6,
        isMonday: dayOfWeek === 1,
      });
    }
    return list;
  });

  let nodePositions = $derived.by(() => {
    const map = {};
    const minMs = timelineBounds.minDate.getTime();
    wbsNodes.forEach((n, idx) => {
      const sMs = parseDateUTC(n.start_date).getTime();
      const eMs = parseDateUTC(n.end_date).getTime();
      const startOffsetDays = Math.round((sMs - minMs) / DAY_MS);
      const spanDays = Math.max(1, Math.round((eMs - sMs) / DAY_MS) + 1);
      const x = startOffsetDays * dayWidth;
      const w = n.node_type === 'milestone' ? dayWidth : spanDays * dayWidth;
      const y = idx * ROW_HEIGHT + ROW_HEIGHT / 2;
      map[n.id] = { x, w, y, rowIndex: idx, spanDays };
    });
    return map;
  });

  function openNewNode(parentNode = null) {
    errorMsg = '';
    const nextIdx = wbsNodes.length + 1;
    let nextCode = `${nextIdx}`;
    if (parentNode) {
      const childCount = wbsNodes.filter((x) => x.parent_id === parentNode.id).length;
      nextCode = `${parentNode.wbs_code}.${childCount + 1}`;
    }
    editingNode = {
      id: '',
      project_id: project.id,
      parent_id: parentNode ? parentNode.id : '',
      wbs_code: nextCode,
      title: '',
      node_type: parentNode ? 'task' : 'phase',
      start_date: parentNode ? parentNode.start_date : project.start_date,
      duration_days: parentNode ? 5 : 10,
      progress: 0,
      allocated_budget: 3000,
      estimated_hours: 40,
      assigned_member_id: members[0]?.id || '',
      order_index: nextIdx,
    };
    showNodeModal = true;
  }

  function openEditNode(n) {
    errorMsg = '';
    editingNode = { ...n };
    showNodeModal = true;
  }

  async function saveNode(e) {
    e.preventDefault();
    errorMsg = '';
    try {
      if (editingNode.id) {
        await api.updateWBSNode(editingNode.id, editingNode);
      } else {
        await api.createWBSNode(project.id, editingNode);
      }
      showNodeModal = false;
      onRefresh?.();
    } catch (err) {
      errorMsg = err.message;
    }
  }

  async function removeNode(nodeId) {
    try {
      await api.deleteWBSNode(nodeId);
      showNodeModal = false;
      onRefresh?.();
    } catch (err) {
      errorMsg = err.message;
    }
  }

  async function shiftNodeDays(n, deltaDays) {
    const dt = parseDateUTC(n.start_date);
    dt.setUTCDate(dt.getUTCDate() + deltaDays);
    const updated = { ...n, start_date: formatDateUTC(dt) };
    await api.updateWBSNode(n.id, updated);
    onRefresh?.();
  }

  async function addDependency(e) {
    e.preventDefault();
    errorMsg = '';
    try {
      await api.createDependency(project.id, newDep);
      showDepModal = false;
      onRefresh?.();
    } catch (err) {
      errorMsg = err.message;
    }
  }

  async function removeDependency(depId) {
    await api.deleteDependency(depId);
    onRefresh?.();
  }

  function getMemberName(id) {
    const m = members.find((x) => x.id === id);
    return m ? m.name : '—';
  }
</script>

<div class="flex flex-col h-full bg-slate-950 rounded-xl border border-slate-800 overflow-hidden">
  <div class="flex flex-wrap items-center justify-between gap-3 px-4 py-3 bg-slate-900/90 border-b border-slate-800">
    <div class="flex items-center gap-2">
      <button
        onclick={() => openNewNode(null)}
        class="px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium flex items-center gap-1.5 shadow-sm transition cursor-pointer"
      >
        <span>+ Nouvelle Phase / Tâche WBS</span>
      </button>
      <button
        onclick={() => {
          errorMsg = '';
          newDep = {
            predecessor_id: wbsNodes[0]?.id || '',
            successor_id: wbsNodes[1]?.id || '',
            dep_type: 'FS',
            lag_days: 0,
          };
          showDepModal = true;
        }}
        class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-medium border border-slate-700 transition cursor-pointer"
      >
        🔗 Lier 2 tâches (Dépendance MS Project)
      </button>
    </div>

    <div class="flex items-center gap-4 text-xs">
      <label class="flex items-center gap-2 cursor-pointer text-slate-300">
        <input type="checkbox" bind:checked={showCriticalPath} class="accent-rose-500 rounded" />
        <span class="flex items-center gap-1.5">
          <span class="w-2.5 h-2.5 rounded-full bg-rose-500 inline-block"></span>
          Chemin Critique (CPM)
        </span>
      </label>

      <div class="flex items-center bg-slate-800 p-0.5 rounded-lg border border-slate-700">
        <button
          onclick={() => (zoomMode = 'day')}
          class="px-2.5 py-1 rounded-md transition cursor-pointer {zoomMode === 'day'
            ? 'bg-indigo-600 text-white font-medium'
            : 'text-slate-400 hover:text-slate-200'}"
        >
          Vue Jour
        </button>
        <button
          onclick={() => (zoomMode = 'week')}
          class="px-2.5 py-1 rounded-md transition cursor-pointer {zoomMode === 'week'
            ? 'bg-indigo-600 text-white font-medium'
            : 'text-slate-400 hover:text-slate-200'}"
        >
          Vue Compacte
        </button>
      </div>
    </div>
  </div>

  <div class="flex-1 flex overflow-auto relative">
    <div class="w-[620px] shrink-0 border-r border-slate-800 bg-slate-950/95 z-10">
      <div
        class="h-[52px] grid grid-cols-12 items-center px-3 bg-slate-900 border-b border-slate-800 text-[11px] font-semibold uppercase tracking-wider text-slate-400 sticky top-0 z-20"
      >
        <div class="col-span-5">Code WBS & Intitulé</div>
        <div class="col-span-2 text-center">Dates / Durée</div>
        <div class="col-span-2 text-right">Budget / Atter.</div>
        <div class="col-span-1 text-center">Tickets</div>
        <div class="col-span-2 text-right">Actions</div>
      </div>

      {#each wbsNodes as n (n.id)}
        {@const isPhase = n.node_type === 'phase'}
        {@const isMile = n.node_type === 'milestone'}
        {@const fin = n.financials || {}}
        {@const isOver = fin.landing_cost > fin.initial_budget && fin.initial_budget > 0}
        <div
          style="height: {ROW_HEIGHT}px"
          class="grid grid-cols-12 items-center px-3 border-b border-slate-900/90 text-xs transition hover:bg-slate-900/70 {isPhase
            ? 'bg-slate-900/40 font-semibold text-slate-100'
            : 'text-slate-300'}"
        >
          <div class="col-span-5 flex items-center gap-1.5 truncate pr-2" style="padding-left: {n.parent_id ? '18px' : '0px'}">
            <span class="font-mono text-[11px] px-1.5 py-0.5 rounded bg-slate-800 text-slate-400 border border-slate-700/60">
              {n.wbs_code}
            </span>
            {#if isMile}
              <span class="text-amber-400" title="Jalon (Milestone)">◆</span>
            {:else if isPhase}
              <span class="text-indigo-400" title="Phase / Lot">▾</span>
            {/if}
            <button
              onclick={() => openEditNode(n)}
              class="truncate text-left hover:text-indigo-400 transition cursor-pointer"
              title="{n.title} (Marge: {n.total_slack_days}j)"
            >
              {n.title}
            </button>
            {#if showCriticalPath && n.is_critical}
              <span class="w-2 h-2 rounded-full bg-rose-500 shrink-0" title="Critique (Marge 0j)"></span>
            {/if}
          </div>

          <div class="col-span-2 text-center text-[11px] leading-tight text-slate-400">
            <div>{n.start_date.slice(5)} → {n.end_date.slice(5)}</div>
            <div class="text-[10px] text-slate-500">
              {isMile ? 'Jalon (0j)' : `${n.duration_days}j • ${Math.round(n.progress)}%`}
            </div>
          </div>

          <div class="col-span-2 text-right text-[11px] leading-tight pr-1">
            {#if fin.initial_budget > 0 || fin.landing_cost > 0}
              <div class="text-slate-300 font-medium">{formatCurrency(fin.initial_budget, project.currency)}</div>
              <div class="text-[10px] {isOver ? 'text-rose-400 font-semibold' : 'text-emerald-400'}" title="Atterrissage des tickets épinglés">
                Att: {formatCurrency(fin.landing_cost, project.currency)}
              </div>
            {:else}
              <span class="text-slate-600">—</span>
            {/if}
          </div>

          <div class="col-span-1 flex justify-center">
            {#if n.pinned_ticket_cnt > 0}
              <button
                onclick={() => onJumpToTickets?.(n.id)}
                class="px-1.5 py-0.5 rounded-full text-[10px] font-medium bg-indigo-500/20 text-indigo-300 border border-indigo-500/30 hover:bg-indigo-500/30 cursor-pointer"
                title="Voir les {n.pinned_ticket_cnt} tickets épinglés sur cette ligne Gantt"
              >
                🎫 {n.done_ticket_cnt}/{n.pinned_ticket_cnt}
              </button>
            {:else}
              <button
                onclick={() => onJumpToTickets?.(n.id)}
                class="text-[10px] text-slate-600 hover:text-slate-400 cursor-pointer"
                title="Épingler ou filtrer les tickets sur cette ligne"
              >
                +Ticket
              </button>
            {/if}
          </div>

          <div class="col-span-2 flex items-center justify-end gap-1">
            {#if !isPhase}
              <button
                onclick={() => shiftNodeDays(n, -1)}
                class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] cursor-pointer"
                title="Avancer de 1 jour"
              >
                -1j
              </button>
              <button
                onclick={() => shiftNodeDays(n, 1)}
                class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] cursor-pointer"
                title="Décaler de +1 jour"
              >
                +1j
              </button>
            {:else}
              <button
                onclick={() => openNewNode(n)}
                class="px-1.5 py-0.5 rounded bg-indigo-500/20 hover:bg-indigo-500/30 text-indigo-300 text-[10px] cursor-pointer"
                title="Ajouter une sous-tâche dans cette phase"
              >
                +Sous-tâche
              </button>
            {/if}
            <button
              onclick={() => openEditNode(n)}
              class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-white text-[10px] cursor-pointer"
              title="Modifier la ligne"
            >
              ✎
            </button>
          </div>
        </div>
      {/each}
    </div>

    <div class="flex-1 overflow-x-auto bg-slate-950">
      <div style="width: {calendarDays.length * dayWidth}px">
        <div class="h-[52px] flex border-b border-slate-800 bg-slate-900 sticky top-0 z-10 select-none">
          {#each calendarDays as d (d.index)}
            <div
              style="width: {dayWidth}px"
              class="shrink-0 border-r border-slate-800/70 flex flex-col items-center justify-center text-[10px] {d.isWeekend
                ? 'bg-slate-950/60 text-slate-600'
                : 'text-slate-400'}"
            >
              {#if d.dayNum === 1 || d.isMonday || zoomMode === 'day'}
                <span class="text-[9px] uppercase text-slate-500">{d.monthShort}</span>
              {/if}
              <span class="font-mono font-medium">{d.dayNum}</span>
            </div>
          {/each}
        </div>

        <svg
          width={calendarDays.length * dayWidth}
          height={Math.max(wbsNodes.length * ROW_HEIGHT, 320)}
          class="block select-none"
        >
          <defs>
            <marker id="arrow-normal" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
              <path d="M0,1 L7,4 L0,7 Z" fill="#6366f1" />
            </marker>
            <marker id="arrow-crit" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
              <path d="M0,1 L7,4 L0,7 Z" fill="#f43f5e" />
            </marker>
          </defs>

          {#each calendarDays as d (d.index)}
            <rect
              x={d.index * dayWidth}
              y="0"
              width={dayWidth}
              height={wbsNodes.length * ROW_HEIGHT}
              fill={d.isWeekend ? 'rgba(15, 23, 42, 0.55)' : 'transparent'}
              stroke="rgba(30, 41, 59, 0.45)"
              stroke-width="1"
            />
          {/each}

          {#each wbsNodes as n, idx (n.id)}
            <line
              x1="0"
              y1={(idx + 1) * ROW_HEIGHT}
              x2={calendarDays.length * dayWidth}
              y2={(idx + 1) * ROW_HEIGHT}
              stroke="rgba(30, 41, 59, 0.7)"
              stroke-width="1"
            />
          {/each}

          {#each dependencies as dep (dep.id)}
            {@const from = nodePositions[dep.predecessor_id]}
            {@const to = nodePositions[dep.successor_id]}
            {#if from && to}
              {@const x1 = dep.dep_type === 'SS' ? from.x : from.x + from.w}
              {@const y1 = from.y}
              {@const x2 = dep.dep_type === 'FF' ? to.x + to.w : to.x}
              {@const y2 = to.y}
              {@const midX = x1 + 10}
              {@const pathD =
                x2 >= x1 + 8
                  ? `M ${x1} ${y1} L ${midX} ${y1} L ${midX} ${y2} L ${x2 - 4} ${y2}`
                  : `M ${x1} ${y1} L ${x1 + 8} ${y1} L ${x1 + 8} ${(y1 + y2) / 2} L ${x2 - 12} ${(y1 + y2) / 2} L ${x2 - 12} ${y2} L ${x2 - 4} ${y2}`}
              <path
                d={pathD}
                fill="none"
                stroke={showCriticalPath ? '#f43f5e' : '#6366f1'}
                stroke-width="1.8"
                marker-end={showCriticalPath ? 'url(#arrow-crit)' : 'url(#arrow-normal)'}
                opacity="0.85"
              />
            {/if}
          {/each}

          {#each wbsNodes as n (n.id)}
            {@const pos = nodePositions[n.id]}
            {#if pos}
              {@const isCrit = showCriticalPath && n.is_critical}
              {#if n.node_type === 'milestone'}
                <g
                  transform="translate({pos.x + dayWidth / 2}, {pos.y})"
                  class="cursor-pointer"
                  onclick={() => openEditNode(n)}
                >
                  <polygon
                    points="0,-9 9,0 0,9 -9,0"
                    fill={isCrit ? '#f43f5e' : '#f59e0b'}
                    stroke="#fef3c7"
                    stroke-width="1.5"
                  />
                  <text x="14" y="4" fill="#e2e8f0" font-size="11" font-weight="600">
                    {n.title} ({n.start_date.slice(5)})
                  </text>
                </g>
              {:else if n.node_type === 'phase'}
                <g class="cursor-pointer" onclick={() => openEditNode(n)}>
                  <rect
                    x={pos.x}
                    y={pos.y - 8}
                    width={pos.w}
                    height="8"
                    rx="2"
                    fill={isCrit ? '#e11d48' : '#4f46e5'}
                  />
                  <polygon points="{pos.x},{pos.y} {pos.x + 6},{pos.y} {pos.x},{pos.y + 6}" fill={isCrit ? '#e11d48' : '#4f46e5'} />
                  <polygon
                    points="{pos.x + pos.w},{pos.y} {pos.x + pos.w - 6},{pos.y} {pos.x + pos.w},{pos.y + 6}"
                    fill={isCrit ? '#e11d48' : '#4f46e5'}
                  />
                  <text x={pos.x + pos.w + 8} y={pos.y - 1} fill="#cbd5e1" font-size="11" font-weight="600">
                    {n.wbs_code} {n.title} ({Math.round(n.progress)}%)
                  </text>
                </g>
              {:else}
                {@const progW = Math.round((pos.w * Math.min(100, Math.max(0, n.progress))) / 100)}
                <g class="cursor-pointer" onclick={() => openEditNode(n)}>
                  <rect
                    x={pos.x}
                    y={pos.y - 11}
                    width={pos.w}
                    height="22"
                    rx="5"
                    fill={isCrit ? 'rgba(244, 63, 94, 0.28)' : 'rgba(56, 189, 248, 0.22)'}
                    stroke={isCrit ? '#f43f5e' : '#38bdf8'}
                    stroke-width="1.5"
                  />
                  {#if progW > 0}
                    <rect
                      x={pos.x}
                      y={pos.y - 11}
                      width={progW}
                      height="22"
                      rx="4"
                      fill={isCrit ? '#e11d48' : '#0284c7'}
                    />
                  {/if}
                  <text x={pos.x + 6} y={pos.y + 4} fill="#f8fafc" font-size="10" font-weight="500">
                    {n.progress}% • {getMemberName(n.assigned_member_id)}
                  </text>
                </g>
              {/if}
            {/if}
          {/each}
        </svg>
      </div>
    </div>
  </div>

  {#if dependencies.length > 0}
    <div class="px-4 py-2 bg-slate-900 border-t border-slate-800 flex items-center gap-2 overflow-x-auto text-xs">
      <span class="text-slate-400 font-medium shrink-0">Liens MS Project actifs :</span>
      {#each dependencies as dep (dep.id)}
        {@const pred = wbsNodes.find((x) => x.id === dep.predecessor_id)}
        {@const succ = wbsNodes.find((x) => x.id === dep.successor_id)}
        {#if pred && succ}
          <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded bg-slate-800 border border-slate-700 text-slate-300 shrink-0">
            <span class="font-mono text-indigo-400">{pred.wbs_code}</span>
            <span>→ ({dep.dep_type}{dep.lag_days ? ` +${dep.lag_days}j` : ''}) →</span>
            <span class="font-mono text-indigo-400">{succ.wbs_code}</span>
            <button
              onclick={() => removeDependency(dep.id)}
              class="text-slate-500 hover:text-rose-400 ml-1 cursor-pointer"
              title="Supprimer ce lien"
            >
              ✕
            </button>
          </span>
        {/if}
      {/each}
    </div>
  {/if}
</div>

{#if showNodeModal}
  <div class="fixed inset-0 bg-black/70 backdrop-blur-xs flex items-center justify-center z-50 p-4">
    <form
      onsubmit={saveNode}
      class="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-lg p-6 space-y-4 shadow-2xl"
    >
      <div class="flex items-center justify-between border-b border-slate-800 pb-3">
        <h3 class="text-base font-semibold text-white">
          {editingNode.id ? `Modifier la ligne WBS ${editingNode.wbs_code}` : 'Nouvelle ligne WBS / Gantt'}
        </h3>
        <button type="button" onclick={() => (showNodeModal = false)} class="text-slate-400 hover:text-white cursor-pointer">✕</button>
      </div>

      {#if errorMsg}
        <div class="p-2.5 rounded-lg bg-rose-500/20 border border-rose-500/40 text-rose-300 text-xs">{errorMsg}</div>
      {/if}

      <div class="grid grid-cols-3 gap-3 text-xs">
        <div>
          <label class="block text-slate-400 mb-1">Code WBS</label>
          <input
            type="text"
            required
            bind:value={editingNode.wbs_code}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Nature</label>
          <select
            bind:value={editingNode.node_type}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          >
            <option value="phase">Phase / Lot (Parent)</option>
            <option value="task">Tâche planifiée</option>
            <option value="milestone">Jalon (Milestone 0j)</option>
          </select>
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Phase Parente</label>
          <select
            bind:value={editingNode.parent_id}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          >
            <option value="">— Racine du projet —</option>
            {#each wbsNodes.filter((x) => x.node_type === 'phase' && x.id !== editingNode.id) as ph}
              <option value={ph.id}>{ph.wbs_code} - {ph.title}</option>
            {/each}
          </select>
        </div>
      </div>

      <div class="text-xs">
        <label class="block text-slate-400 mb-1">Intitulé de la Phase / Tâche / Jalon</label>
        <input
          type="text"
          required
          bind:value={editingNode.title}
          placeholder="Ex: Développement API & Moteur Financier"
          class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
        />
      </div>

      <div class="grid grid-cols-3 gap-3 text-xs">
        <div>
          <label class="block text-slate-400 mb-1">Date de début</label>
          <input
            type="date"
            required
            bind:value={editingNode.start_date}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Durée (Jours)</label>
          <input
            type="number"
            min="0"
            bind:value={editingNode.duration_days}
            disabled={editingNode.node_type === 'milestone'}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white disabled:opacity-50"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Avancement (%)</label>
          <input
            type="number"
            min="0"
            max="100"
            bind:value={editingNode.progress}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3 text-xs">
        <div>
          <label class="block text-slate-400 mb-1">Budget Alloué à cette ligne (€)</label>
          <input
            type="number"
            step="100"
            bind:value={editingNode.allocated_budget}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Responsable</label>
          <select
            bind:value={editingNode.assigned_member_id}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          >
            <option value="">— Non assigné —</option>
            {#each members as m}
              <option value={m.id}>{m.name} ({m.hourly_rate}€/h)</option>
            {/each}
          </select>
        </div>
      </div>

      <div class="flex items-center justify-between pt-3 border-t border-slate-800">
        {#if editingNode.id}
          <button
            type="button"
            onclick={() => removeNode(editingNode.id)}
            class="px-3 py-2 rounded-lg bg-rose-500/20 hover:bg-rose-500/30 text-rose-300 text-xs font-medium cursor-pointer"
          >
            Supprimer
          </button>
        {:else}
          <div></div>
        {/if}
        <div class="flex gap-2">
          <button
            type="button"
            onclick={() => (showNodeModal = false)}
            class="px-4 py-2 rounded-lg bg-slate-800 text-slate-300 text-xs cursor-pointer"
          >
            Annuler
          </button>
          <button
            type="submit"
            class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium cursor-pointer"
          >
            Enregistrer & Recalculer le Gantt
          </button>
        </div>
      </div>
    </form>
  </div>
{/if}

{#if showDepModal}
  <div class="fixed inset-0 bg-black/70 backdrop-blur-xs flex items-center justify-center z-50 p-4">
    <form
      onsubmit={addDependency}
      class="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-md p-6 space-y-4 shadow-2xl text-xs"
    >
      <div class="flex items-center justify-between border-b border-slate-800 pb-3">
        <h3 class="text-base font-semibold text-white">Créer une dépendance MS Project</h3>
        <button type="button" onclick={() => (showDepModal = false)} class="text-slate-400 hover:text-white cursor-pointer">✕</button>
      </div>

      {#if errorMsg}
        <div class="p-2.5 rounded-lg bg-rose-500/20 border border-rose-500/40 text-rose-300">{errorMsg}</div>
      {/if}

      <div>
        <label class="block text-slate-400 mb-1">Tâche Prédécesseur</label>
        <select bind:value={newDep.predecessor_id} class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white">
          {#each wbsNodes as n}
            <option value={n.id}>{n.wbs_code} — {n.title}</option>
          {/each}
        </select>
      </div>

      <div>
        <label class="block text-slate-400 mb-1">Tâche Successeur (sera recalée automatiquement)</label>
        <select bind:value={newDep.successor_id} class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white">
          {#each wbsNodes as n}
            <option value={n.id}>{n.wbs_code} — {n.title}</option>
          {/each}
        </select>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-slate-400 mb-1">Type de lien</label>
          <select bind:value={newDep.dep_type} class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white">
            <option value="FS">FS — Fin à Début (Standard)</option>
            <option value="SS">SS — Début à Début</option>
            <option value="FF">FF — Fin à Fin</option>
            <option value="SF">SF — Début à Fin</option>
          </select>
        </div>
        <div>
          <label class="block text-slate-400 mb-1">Décalage (Lag en jours)</label>
          <input
            type="number"
            bind:value={newDep.lag_days}
            class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
          />
        </div>
      </div>

      <div class="flex justify-end gap-2 pt-3 border-t border-slate-800">
        <button type="button" onclick={() => (showDepModal = false)} class="px-4 py-2 rounded-lg bg-slate-800 text-slate-300 cursor-pointer">
          Annuler
        </button>
        <button type="submit" class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium cursor-pointer">
          Lier & Recalculer
        </button>
      </div>
    </form>
  </div>
{/if}

