<script>
  import { api, formatCurrency } from '../api.js';

  let {
    project,
    docs = [],
    tickets = [],
    wbsNodes = [],
    onRefresh,
  } = $props();

  let selectedDocId = $state('');
  let isEditing = $state(false);
  let draftDoc = $state({
    id: '',
    title: '',
    category: 'spec',
    content_md: '',
    linked_ticket_ids: '',
    order_index: 1,
  });

  $effect(() => {
    if (docs.length > 0 && (!selectedDocId || !docs.some((d) => d.id === selectedDocId))) {
      selectedDocId = docs[0].id;
    }
  });

  let activeDoc = $derived(docs.find((d) => d.id === selectedDocId) || null);

  const CATEGORIES = {
    architecture: { label: 'Architecture', badge: 'bg-indigo-500/20 text-indigo-300 border-indigo-500/30' },
    spec: { label: 'Spécifications', badge: 'bg-sky-500/20 text-sky-300 border-sky-500/30' },
    guide: { label: 'Guide / Méthode', badge: 'bg-emerald-500/20 text-emerald-300 border-emerald-500/30' },
    risk: { label: 'Risques & ADR', badge: 'bg-amber-500/20 text-amber-300 border-amber-500/30' },
    meeting: { label: 'Compte-rendu (COPIL)', badge: 'bg-purple-500/20 text-purple-300 border-purple-500/30' },
  };

  function startNewDoc() {
    draftDoc = {
      id: '',
      project_id: project.id,
      title: 'Nouvelle Page de Documentation',
      category: 'spec',
      content_md: `# Objectif du document\n\nDécrivez ici les spécifications ou décisions liées au projet **${project.name}**.\n\n## Tickets associés\nVous pouvez référencer directement un ticket par sa clé (ex: #${tickets[0]?.ticket_key || 'AIG-1'}).`,
      linked_ticket_ids: tickets[0]?.ticket_key || '',
      order_index: docs.length + 1,
    };
    isEditing = true;
  }

  function startEditDoc(d) {
    draftDoc = { ...d };
    isEditing = true;
  }

  async function saveDoc(e) {
    e.preventDefault();
    if (draftDoc.id) {
      await api.updateDoc(draftDoc.id, draftDoc);
      selectedDocId = draftDoc.id;
    } else {
      const created = await api.createDoc(project.id, draftDoc);
      selectedDocId = created.id;
    }
    isEditing = false;
    onRefresh?.();
  }

  async function removeDoc(id) {
    await api.deleteDoc(id);
    isEditing = false;
    onRefresh?.();
  }

  function insertTicketRef(ticketKey) {
    draftDoc.content_md += `\n- Référence Ticket : **#${ticketKey}**`;
    const list = (draftDoc.linked_ticket_ids || '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    if (!list.includes(ticketKey)) {
      list.push(ticketKey);
      draftDoc.linked_ticket_ids = list.join(',');
    }
  }

  function renderMarkdown(md) {
    if (!md) return '';
    let html = md
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');

    html = html.replace(/^### (.*$)/gim, '<h3 class="text-sm font-semibold text-indigo-300 mt-4 mb-1.5">$1</h3>');
    html = html.replace(/^## (.*$)/gim, '<h2 class="text-base font-bold text-white mt-5 mb-2 border-b border-slate-800 pb-1">$1</h2>');
    html = html.replace(/^# (.*$)/gim, '<h1 class="text-xl font-bold text-white mb-3">$1</h1>');

    html = html.replace(/\*\*(.*?)\*\*/g, '<strong class="text-white font-semibold">$1</strong>');
    html = html.replace(/`([^`]+)`/g, '<code class="px-1.5 py-0.5 rounded bg-slate-800 text-indigo-300 font-mono text-xs">$1</code>');

    html = html.replace(
      /#([A-Z]+-\d+)/g,
      '<span class="inline-flex items-center px-2 py-0.5 rounded bg-indigo-500/20 border border-indigo-500/40 text-indigo-300 font-mono text-xs font-semibold">🎫 #$1</span>'
    );

    html = html.replace(/^- (.*$)/gim, '<li class="ml-4 list-disc text-slate-300 my-1">$1</li>');
    html = html.replace(/\n\n/g, '<div class="h-3"></div>');
    return html;
  }

  let linkedTicketsList = $derived.by(() => {
    if (!activeDoc) return [];
    const raw = (activeDoc.linked_ticket_ids || '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    return tickets.filter((t) => raw.includes(t.ticket_key) || activeDoc.content_md?.includes(`#${t.ticket_key}`));
  });
</script>

<div class="grid grid-cols-1 md:grid-cols-12 gap-4 h-full">
  <div class="md:col-span-3 bg-slate-900 border border-slate-800 rounded-xl p-3 flex flex-col space-y-3">
    <div class="flex items-center justify-between border-b border-slate-800 pb-2.5">
      <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-300">📚 Wiki Projet ({docs.length})</h3>
      <button
        onclick={startNewDoc}
        class="px-2.5 py-1 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium cursor-pointer"
      >
        + Page
      </button>
    </div>

    <div class="flex-1 space-y-1.5 overflow-y-auto">
      {#each docs as d (d.id)}
        {@const cat = CATEGORIES[d.category] || CATEGORIES.spec}
        <button
          onclick={() => {
            selectedDocId = d.id;
            isEditing = false;
          }}
          class="w-full text-left p-2.5 rounded-xl border transition cursor-pointer {selectedDocId === d.id && !isEditing
            ? 'bg-indigo-600/15 border-indigo-500/50 text-white'
            : 'bg-slate-950/60 border-slate-800/80 text-slate-300 hover:bg-slate-800/60'}"
        >
          <div class="flex items-center justify-between gap-1 mb-1">
            <span class="px-1.5 py-0.5 rounded text-[10px] border {cat.badge}">{cat.label}</span>
          </div>
          <div class="text-xs font-medium line-clamp-2">{d.title}</div>
        </button>
      {/each}
    </div>
  </div>

  <div class="md:col-span-9 bg-slate-900 border border-slate-800 rounded-xl p-6 overflow-y-auto">
    {#if isEditing}
      <form onsubmit={saveDoc} class="space-y-4 text-xs">
        <div class="flex items-center justify-between border-b border-slate-800 pb-3">
          <h3 class="text-base font-semibold text-white">
            {draftDoc.id ? 'Édition de la page documentaire' : 'Nouvelle page documentaire'}
          </h3>
          <div class="flex gap-2">
            {#if draftDoc.id}
              <button
                type="button"
                onclick={() => removeDoc(draftDoc.id)}
                class="px-3 py-1.5 rounded-lg bg-rose-500/20 text-rose-300 cursor-pointer"
              >
                Supprimer
              </button>
            {/if}
            <button
              type="button"
              onclick={() => (isEditing = false)}
              class="px-3 py-1.5 rounded-lg bg-slate-800 text-slate-300 cursor-pointer"
            >
              Annuler
            </button>
            <button
              type="submit"
              class="px-4 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium cursor-pointer"
            >
              Enregistrer la page
            </button>
          </div>
        </div>

        <div class="grid grid-cols-3 gap-3">
          <div class="col-span-2">
            <label class="block text-slate-400 mb-1">Titre du document</label>
            <input
              type="text"
              required
              bind:value={draftDoc.title}
              class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white"
            />
          </div>
          <div>
            <label class="block text-slate-400 mb-1">Catégorie</label>
            <select bind:value={draftDoc.category} class="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-white">
              {#each Object.entries(CATEGORIES) as [k, v]}
                <option value={k}>{v.label}</option>
              {/each}
            </select>
          </div>
        </div>

        <div class="flex items-center gap-2 flex-wrap p-2.5 rounded-lg bg-slate-950 border border-slate-800">
          <span class="text-slate-400">Insérer un lien vers un ticket du projet :</span>
          {#each tickets as t (t.id)}
            <button
              type="button"
              onclick={() => insertTicketRef(t.ticket_key)}
              class="px-2 py-0.5 rounded bg-indigo-500/15 hover:bg-indigo-500/30 border border-indigo-500/30 text-indigo-300 font-mono text-[11px] cursor-pointer"
            >
              +#{t.ticket_key}
            </button>
          {/each}
        </div>

        <div>
          <label class="block text-slate-400 mb-1">Contenu (Markdown supporté)</label>
          <textarea
            rows="14"
            bind:value={draftDoc.content_md}
            class="w-full p-3 rounded-xl bg-slate-950 border border-slate-700 text-slate-100 font-mono text-xs leading-relaxed"
          ></textarea>
        </div>
      </form>
    {:else if activeDoc}
      {@const cat = CATEGORIES[activeDoc.category] || CATEGORIES.spec}
      <div class="space-y-6">
        <div class="flex items-start justify-between border-b border-slate-800 pb-4">
          <div class="space-y-1.5">
            <span class="px-2 py-0.5 rounded text-[11px] border {cat.badge}">{cat.label}</span>
            <h1 class="text-xl font-bold text-white">{activeDoc.title}</h1>
          </div>
          <button
            onclick={() => startEditDoc(activeDoc)}
            class="px-3.5 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-medium border border-slate-700 cursor-pointer"
          >
            ✎ Éditer la page
          </button>
        </div>

        <div class="text-sm text-slate-300 leading-relaxed space-y-2">
          {@html renderMarkdown(activeDoc.content_md)}
        </div>

        {#if linkedTicketsList.length > 0}
          <div class="pt-4 border-t border-slate-800 space-y-2.5">
            <h4 class="text-xs font-semibold uppercase tracking-wider text-slate-400">
              🎫 Tickets & Coûts rattachés à cette page documentaire ({linkedTicketsList.length})
            </h4>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-2.5">
              {#each linkedTicketsList as t (t.id)}
                <div class="p-3 rounded-xl bg-slate-950 border border-slate-800 flex items-center justify-between text-xs">
                  <div class="truncate pr-2">
                    <div class="flex items-center gap-1.5">
                      <span class="font-mono font-bold text-indigo-400">{t.ticket_key}</span>
                      <span class="text-white font-medium truncate">{t.title}</span>
                    </div>
                    <div class="text-[11px] text-slate-400 mt-0.5">
                      Statut: <strong class="uppercase text-slate-300">{t.status}</strong> •
                      {t.wbs_node_id ? `Gantt ${t.wbs_node_code}` : 'Projet Global'}
                    </div>
                  </div>
                  <div class="text-right font-mono shrink-0">
                    <div class="text-xs font-semibold text-white">
                      Att: {formatCurrency(t.landing_cost, project.currency)}
                    </div>
                    <div class="text-[10px] text-slate-500">RAF: {t.remaining_hours}h</div>
                  </div>
                </div>
              {/each}
            </div>
          </div>
        {/if}
      </div>
    {:else}
      <div class="text-center py-12 text-slate-500 text-sm">
        Aucune page de documentation. Cliquez sur "+ Page" pour commencer.
      </div>
    {/if}
  </div>
</div>

