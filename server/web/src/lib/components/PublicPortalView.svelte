<script>
  import { onMount } from 'svelte';

  let { onEnterSaaS = () => {}, isAuthenticated = false } = $props();

  let releaseData = $state({
    app_name: 'Aigle — Gestion de Projet Tout-en-Un',
    version: '2.0.0',
    app_domain: 'aigle.heimerdinger.studio',
    auth_domain: 'airgle-auth.heimerdinger.studio',
    saas_url: '/app',
    assets: [],
  });

  let loading = $state(true);
  let activePresentationTab = $state('cases'); // 'cases' (2. Comment on l'utilise) | 'features' (1. Ce que ça fait)
  let copiedSha = $state('');

  onMount(async () => {
    try {
      const res = await fetch('/public/api/releases');
      if (res.ok) {
        releaseData = await res.json();
      }
    } catch {}
    loading = false;
  });

  let osAssets = $derived((releaseData.assets || []).filter((a) => a.os !== 'presentation'));
  let presAssets = $derived((releaseData.assets || []).filter((a) => a.os === 'presentation'));

  function copyChecksum(sha, id) {
    if (!sha) return;
    navigator.clipboard?.writeText(sha);
    copiedSha = id;
    setTimeout(() => {
      if (copiedSha === id) copiedSha = '';
    }, 2000);
  }

  const OS_ICONS = {
    windows: { icon: '🪟', badge: 'bg-sky-500/15 text-sky-300 border-sky-500/30', accent: 'from-sky-500/20 to-indigo-500/10' },
    macos: { icon: '🍎', badge: 'bg-slate-200/15 text-slate-100 border-slate-400/30', accent: 'from-slate-500/20 to-indigo-500/10' },
    linux: { icon: '🐧', badge: 'bg-amber-500/15 text-amber-300 border-amber-500/30', accent: 'from-amber-500/20 to-indigo-500/10' },
  };
</script>

<div class="min-h-screen bg-slate-950 text-slate-100 flex flex-col">
  <!-- Barre de Navigation Publique -->
  <header class="sticky top-0 z-30 backdrop-blur-md bg-slate-950/85 border-b border-slate-800/80 px-6 py-3.5">
    <div class="max-w-7xl mx-auto flex flex-wrap items-center justify-between gap-4">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-xl bg-indigo-600 flex items-center justify-center font-black text-xl text-white shadow-lg shadow-indigo-600/30">
          🦅
        </div>
        <div>
          <div class="flex items-center gap-2">
            <span class="font-extrabold text-lg tracking-tight text-white">AIGLE</span>
            <span class="px-2 py-0.5 rounded-full bg-emerald-500/15 border border-emerald-500/30 text-emerald-300 text-[10px] font-mono uppercase">
              v{releaseData.version} • Portail Public
            </span>
          </div>
          <p class="text-[11px] text-slate-400">
            MS Project (WBS/Gantt CPM) • Kanban Hybride & Sous-tickets • Moteur Financier EVM • Wiki Markdown
          </p>
        </div>
      </div>

      <div class="flex items-center gap-3 text-xs">
        <a
          href="#presentation"
          class="px-3.5 py-2 rounded-xl bg-slate-900 hover:bg-slate-800 text-slate-200 border border-slate-800 transition"
        >
          📽️ Présentation & Cas Concrets
        </a>
        <a
          href="#downloads"
          class="px-3.5 py-2 rounded-xl bg-slate-900 hover:bg-slate-800 text-slate-200 border border-slate-800 transition"
        >
          ⬇️ Télécharger (Win / Mac / Linux)
        </a>
        <button
          onclick={onEnterSaaS}
          class="px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white font-semibold shadow-md shadow-indigo-600/25 transition cursor-pointer flex items-center gap-2"
        >
          <span>🔐 {isAuthenticated ? 'Ouvrir mon Workspace SaaS' : 'Connexion Espace Équipe SaaS (Authelia)'}</span>
        </button>
      </div>
    </div>
  </header>

  <main class="flex-1 max-w-7xl w-full mx-auto px-6 py-10 space-y-16">
    <!-- HERO SECTION : Vitrine Publique + Double Mode (Local Téléchargeable vs Cloud SaaS Protégé) -->
    <section class="relative rounded-3xl bg-gradient-to-br from-slate-900 via-indigo-950/40 to-slate-900 border border-indigo-500/30 p-8 md:p-12 overflow-hidden shadow-2xl">
      <div class="max-w-3xl space-y-5">
        <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-indigo-500/15 border border-indigo-500/30 text-indigo-300 text-xs font-medium">
          <span>✨ Tout-en-un en Go & Svelte 5</span>
          <span>•</span>
          <span>Édition Locale Autonome & Édition SaaS Multi-Tenant</span>
        </div>

        <h1 class="text-3xl md:text-4xl font-extrabold text-white tracking-tight leading-tight">
          Pilotez vos plannings <span class="text-indigo-400">MS Project</span>, vos <span class="text-sky-400">Tickets Kanban</span> et votre <span class="text-emerald-400">Atterrissage Financier</span> dans un seul outil.
        </h1>

        <p class="text-sm md:text-base text-slate-300 leading-relaxed">
          Contrairement aux outils cloisonnés où le planning Gantt, les tickets opérationnels et le suivi budgétaire vivent dans trois logiciels séparés, <strong>Aigle</strong> unifie en temps réel l'ordonnancement par chemin critique (CPM), le Kanban (avec sous-tickets enfants), le calcul du Reste à Faire (RAF) et la documentation projet.
        </p>

        <div class="flex flex-wrap items-center gap-3 pt-2">
          <a
            href="#downloads"
            class="px-5 py-3 rounded-xl bg-white text-slate-950 hover:bg-slate-200 font-bold text-xs shadow-lg transition flex items-center gap-2"
          >
            <span>⬇️ Télécharger l'Application Locale (Windows, macOS, Linux)</span>
          </a>

          <button
            onclick={onEnterSaaS}
            class="px-5 py-3 rounded-xl bg-indigo-600/90 hover:bg-indigo-500 text-white font-semibold text-xs border border-indigo-400/30 transition cursor-pointer flex items-center gap-2"
          >
            <span>🛡️ Accéder au Serveur d'Équipe SaaS (Protégé par Authelia SSO)</span>
          </button>
        </div>
      </div>

      <!-- Bandeau des 4 Piliers Synthétiques -->
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4 mt-10 pt-8 border-t border-slate-800/80 text-xs">
        <div class="p-4 rounded-2xl bg-slate-950/70 border border-slate-800/90 space-y-1.5">
          <div class="text-indigo-400 font-bold flex items-center gap-1.5">📅 1. MS Project & Gantt CPM</div>
          <p class="text-slate-400 leading-relaxed">
            Découpage WBS (Phases, Tâches, Jalons), liens FS/SS/FF/SF, recalcul automatique des dates et du Chemin Critique (marge 0j).
          </p>
        </div>
        <div class="p-4 rounded-2xl bg-slate-950/70 border border-slate-800/90 space-y-1.5">
          <div class="text-sky-400 font-bold flex items-center gap-1.5">📋 2. Kanban Hybride & Enfants</div>
          <p class="text-slate-400 leading-relaxed">
            Tickets épinglés sur une tâche Gantt ou globaux au projet, découpage en sous-tickets enfants avec consolidation automatique des coûts.
          </p>
        </div>
        <div class="p-4 rounded-2xl bg-slate-950/70 border border-slate-800/90 space-y-1.5">
          <div class="text-emerald-400 font-bold flex items-center gap-1.5">💰 3. Moteur Financier EVM</div>
          <p class="text-slate-400 leading-relaxed">
            Grille TJM/THM par intervenant, suivi temps réel du Budget Initial, du Consommé, du Reste à Faire (RAF) et de l'Atterrissage.
          </p>
        </div>
        <div class="p-4 rounded-2xl bg-slate-950/70 border border-slate-800/90 space-y-1.5">
          <div class="text-purple-400 font-bold flex items-center gap-1.5">📚 4. Wiki & Gouvernance</div>
          <p class="text-slate-400 leading-relaxed">
            Documentation Markdown intégrée avec références directes aux tickets (`#AIG-1`) et visualisation de leur coût d'atterrissage.
          </p>
        </div>
      </div>
    </section>

    <!-- SECTION TÉLÉCHARGEMENTS MULTI-OS (WINDOWS, MACOS, LINUX) -->
    <section id="downloads" class="space-y-6 scroll-mt-20">
      <div class="flex flex-wrap items-end justify-between gap-4 border-b border-slate-800 pb-4">
        <div>
          <span class="text-xs font-mono uppercase tracking-wider text-indigo-400 font-semibold">
            Distribution Exécutable Autonome (Zero-Dependency)
          </span>
          <h2 class="text-2xl font-bold text-white mt-1">
            ⬇️ Télécharger Aigle Desktop / Local (Windows, macOS & Linux)
          </h2>
          <p class="text-xs text-slate-400 mt-1">
            Un seul fichier exécutable contenant le moteur Go, la base SQLite WAL locale et l'interface graphique Svelte 5. Aucune installation de serveur ni de Node.js requise.
          </p>
        </div>

        <div class="text-right text-xs text-slate-400 font-mono">
          <div>Intégrité vérifiée par empreinte <span class="text-emerald-400">SHA-256</span></div>
          <div>Licence : <span class="text-white">Heimerdinger Studio</span></div>
        </div>
      </div>

      <!-- Grille des 4 binaires OS -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        {#each osAssets as asset (asset.id)}
          {@const style = OS_ICONS[asset.os] || OS_ICONS.linux}
          <div class="p-6 rounded-2xl bg-gradient-to-br {style.accent} bg-slate-900 border border-slate-800 hover:border-indigo-500/40 transition flex flex-col justify-between space-y-4">
            <div class="space-y-3">
              <div class="flex items-start justify-between gap-2">
                <div class="flex items-center gap-3">
                  <div class="w-11 h-11 rounded-2xl bg-slate-950 border border-slate-800 flex items-center justify-center text-2xl">
                    {style.icon}
                  </div>
                  <div>
                    <h3 class="text-base font-bold text-white">{asset.label}</h3>
                    <div class="flex items-center gap-2 mt-0.5">
                      <span class="px-2 py-0.5 rounded text-[10px] font-mono border {style.badge}">
                        {asset.os.toUpperCase()} • {asset.arch}
                      </span>
                      <span class="text-xs font-mono text-slate-400">{asset.size_formatted}</span>
                    </div>
                  </div>
                </div>

                {#if asset.available}
                  <a
                    href={asset.download_url}
                    download={asset.filename}
                    class="px-4 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-bold shadow-md shadow-indigo-600/20 transition shrink-0"
                  >
                    ⬇️ Télécharger
                  </a>
                {:else}
                  <span class="px-3 py-1.5 rounded-xl bg-slate-800 text-slate-400 text-xs font-mono">
                    Indisponible
                  </span>
                {/if}
              </div>

              <p class="text-xs text-slate-300 leading-relaxed">{asset.description}</p>
            </div>

            <div class="space-y-2 pt-3 border-t border-slate-800/80 text-[11px]">
              <div class="flex items-center justify-between bg-slate-950 px-3 py-2 rounded-lg border border-slate-800 font-mono">
                <span class="text-slate-400 select-all">$ {asset.command_hint}</span>
                <span class="text-[10px] text-indigo-400 ml-2 shrink-0">{asset.filename}</span>
              </div>

              {#if asset.sha256}
                <div class="flex items-center justify-between text-[10px] text-slate-500 font-mono px-1">
                  <span class="truncate pr-2" title={asset.sha256}>
                    SHA-256 : <span class="text-slate-400">{asset.sha256.slice(0, 24)}…{asset.sha256.slice(-12)}</span>
                  </span>
                  <button
                    type="button"
                    onclick={() => copyChecksum(asset.sha256, asset.id)}
                    class="text-indigo-400 hover:text-indigo-300 cursor-pointer shrink-0"
                  >
                    {copiedSha === asset.id ? '✓ Copié' : 'Copier SHA-256'}
                  </button>
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>

      <!-- Supports de Présentation Téléchargeables (.pptx & .html) -->
      {#if presAssets.length > 0}
        <div class="p-5 rounded-2xl bg-slate-900/90 border border-slate-800 flex flex-wrap items-center justify-between gap-4">
          <div class="space-y-1">
            <h4 class="text-sm font-bold text-white flex items-center gap-2">
              <span>📊</span> Supports de Présentation Officiels (PowerPoint .PPTX & Diaporama .HTML)
            </h4>
            <p class="text-xs text-slate-400">
              Téléchargez la présentation complète d'Aigle (Partie 1 : Cas concrets d'utilisation • Partie 2 : Fonctionnalités & Architecture).
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2.5 text-xs">
            {#each presAssets as pa (pa.id)}
              {#if pa.available}
                {#if pa.arch === 'html'}
                  <a
                    href="{pa.download_url}?view=1"
                    target="_blank"
                    rel="noopener"
                    class="px-3.5 py-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-indigo-300 font-semibold border border-indigo-500/30 transition"
                  >
                    👁️ Ouvrir le Diaporama Web (.html) ↗
                  </a>
                {/if}
                <a
                  href={pa.download_url}
                  download={pa.filename}
                  class="px-3.5 py-2 rounded-xl bg-indigo-600/20 hover:bg-indigo-600/30 text-indigo-200 font-semibold border border-indigo-500/40 transition"
                >
                  ⬇️ {pa.label} ({pa.size_formatted})
                </a>
              {/if}
            {/each}
          </div>
        </div>
      {/if}
    </section>

    <!-- SECTION MINI-PRÉSENTATION INTERACTIVE (CAS CONCRETS & FONCTIONNALITÉS) -->
    <section id="presentation" class="space-y-6 scroll-mt-20">
      <div class="flex flex-wrap items-center justify-between gap-4 border-b border-slate-800 pb-4">
        <div>
          <span class="text-xs font-mono uppercase tracking-wider text-emerald-400 font-semibold">
            Présentation Interactive Intégrée
          </span>
          <h2 class="text-2xl font-bold text-white mt-1">
            📽️ Découvrir Aigle : Comment on l'utilise & Ce qu'il fait
          </h2>
        </div>

        <div class="flex items-center gap-2 bg-slate-900 p-1 rounded-xl border border-slate-800 text-xs">
          <button
            onclick={() => (activePresentationTab = 'cases')}
            class="px-4 py-2 rounded-lg font-semibold transition cursor-pointer {activePresentationTab === 'cases'
              ? 'bg-indigo-600 text-white'
              : 'text-slate-400 hover:text-white'}"
          >
            1. Comment on l'utilise (4 Cas Concrets)
          </button>
          <button
            onclick={() => (activePresentationTab = 'features')}
            class="px-4 py-2 rounded-lg font-semibold transition cursor-pointer {activePresentationTab === 'features'
              ? 'bg-indigo-600 text-white'
              : 'text-slate-400 hover:text-white'}"
          >
            2. Ce que ça fait & Comparatif Local vs SaaS
          </button>
        </div>
      </div>

      {#if activePresentationTab === 'cases'}
        <div class="grid grid-cols-1 md:grid-cols-2 gap-5 text-xs">
          <!-- Cas Concret A -->
          <div class="p-6 rounded-2xl bg-slate-900 border border-slate-800 space-y-3">
            <div class="flex items-center justify-between">
              <span class="px-2.5 py-1 rounded-full bg-indigo-500/20 text-indigo-300 font-bold">
                Cas Concret A — Cadrage & Planning MS Project
              </span>
              <span class="font-mono text-slate-400">Chef de Projet</span>
            </div>
            <h3 class="text-base font-bold text-white">
              Construire le WBS, lier les tâches et absorber un retard automatiquement
            </h3>
            <p class="text-slate-300 leading-relaxed">
              Vous créez un projet doté d'un budget initial de <strong>45 000 €</strong>. Dans l'onglet <strong>Timeline MS Project</strong>, vous découpez le projet en 3 lots WBS et reliez la tâche <em>1.1 Architecture</em> à <em>1.2 Développement</em> en lien <strong>Fin-à-Début (FS)</strong>.
            </p>
            <div class="p-3 rounded-xl bg-slate-950 border border-slate-800 text-slate-300">
              👉 <strong>Résultat concret :</strong> Si la tâche 1.1 prend +2 jours (`+1j`), le moteur CPM recalcule instantanément les dates de toutes les tâches suivantes et met en rouge le <strong>Chemin Critique</strong>.
            </div>
          </div>

          <!-- Cas Concret B -->
          <div class="p-6 rounded-2xl bg-slate-900 border border-slate-800 space-y-3">
            <div class="flex items-center justify-between">
              <span class="px-2.5 py-1 rounded-full bg-purple-500/20 text-purple-300 font-bold">
                Cas Concret B — Kanban Hybride & Sous-tickets Enfants
              </span>
              <span class="font-mono text-slate-400">Lead Tech & Équipe</span>
            </div>
            <h3 class="text-base font-bold text-white">
              Découper un ticket complexe en sous-tickets enfants tout en gardant le coût consolidé
            </h3>
            <p class="text-slate-300 leading-relaxed">
              Un ticket principal <code>#AIG-2</code> (<em>Moteur d'atterrissage financier</em>) est épinglé sur la ligne Gantt <code>2.1</code>. L'équipe le découpe en 2 <strong>sous-tickets enfants</strong> (<code>#AIG-6</code> Formules EVM et <code>#AIG-7</code> Consolidation WBS) assignés à deux développeurs différents.
            </p>
            <div class="p-3 rounded-xl bg-slate-950 border border-slate-800 text-slate-300">
              👉 <strong>Résultat concret :</strong> La carte parente affiche l'avancement des enfants (<code>1/2 sous-tickets</code>) et le <strong>Coût Consolidé (Parent + Enfants)</strong>. Les tickets de support hors planning restent quant à eux rattachés au budget global du projet.
            </div>
          </div>

          <!-- Cas Concret C -->
          <div class="p-6 rounded-2xl bg-slate-900 border border-slate-800 space-y-3">
            <div class="flex items-center justify-between">
              <span class="px-2.5 py-1 rounded-full bg-amber-500/20 text-amber-300 font-bold">
                Cas Concret C — Pointage Temps, RAF & Alerte Dérive
              </span>
              <span class="font-mono text-slate-400">Pilotage Financier</span>
            </div>
            <h3 class="text-base font-bold text-white">
              Détecter un dépassement budgétaire avant qu'il ne soit trop tard
            </h3>
            <p class="text-slate-300 leading-relaxed">
              Un développeur (TJM 600 €/j soit 75 €/h) pointe <strong>8h</strong> sur un ticket initialement estimé à 16h, mais réévalue son <strong>Reste à Faire (RAF)</strong> à <strong>14h</strong> suite à une complexité technique imprévue.
            </p>
            <div class="p-3 rounded-xl bg-slate-950 border border-slate-800 text-slate-300">
              👉 <strong>Résultat concret :</strong> L'atterrissage du ticket passe automatiquement à <code>(8h + 14h) × 75€ = 1 650 €</code> (contre 1 200 € prévus). Le ticket passe en badge <strong>⚠️ Dépassement</strong> et l'atterrissage global du projet se met à jour en direct dans l'en-tête.
            </div>
          </div>

          <!-- Cas Concret D -->
          <div class="p-6 rounded-2xl bg-slate-900 border border-slate-800 space-y-3">
            <div class="flex items-center justify-between">
              <span class="px-2.5 py-1 rounded-full bg-emerald-500/20 text-emerald-300 font-bold">
                Cas Concret D — Gouvernance Multi-Projets & SaaS Multi-Tenant
              </span>
              <span class="font-mono text-slate-400">Direction / DSI</span>
            </div>
            <h3 class="text-base font-bold text-white">
              Isoler les espaces clients (Workspaces) et auditer la sécurité avec Authelia
            </h3>
            <p class="text-slate-300 leading-relaxed">
              En édition SaaS, chaque client ou pôle dispose de son propre <strong>Workspace isolé</strong>. Les accès sont protégés en amont par <strong>Authelia (SSO + 2FA)</strong> et régis par 5 rôles RBAC (du <code>superadmin</code> au <code>viewer</code> en lecture seule).
            </p>
            <div class="p-3 rounded-xl bg-slate-950 border border-slate-800 text-slate-300">
              👉 <strong>Résultat concret :</strong> La direction consolide ses portefeuilles par organisation et dispose d'un <strong>Journal d'Audit de Sécurité immuable</strong> traçant chaque action sensible et adresse IP.
            </div>
          </div>
        </div>
      {:else}
        <!-- Comparatif Édition Locale Téléchargeable vs Édition SaaS Serveur -->
        <div class="rounded-2xl bg-slate-900 border border-slate-800 overflow-hidden text-xs">
          <table class="w-full text-left border-collapse">
            <thead class="bg-slate-950 text-slate-400 uppercase text-[11px] border-b border-slate-800">
              <tr>
                <th class="py-3.5 px-5">Fonctionnalité / Architecture</th>
                <th class="py-3.5 px-5 text-sky-400">💻 Aigle Édition Locale (Téléchargement Ci-dessus)</th>
                <th class="py-3.5 px-5 text-indigo-400">☁️ Aigle Édition SaaS Server (Espace Protégé /app)</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/80 text-slate-300">
              <tr>
                <td class="py-3 px-5 font-semibold text-white">Planning MS Project (WBS, Gantt, CPM, Chemin Critique)</td>
                <td class="py-3 px-5 text-emerald-400">✓ Inclus à 100%</td>
                <td class="py-3 px-5 text-emerald-400">✓ Inclus à 100% (Temps réel multi-utilisateurs SSE)</td>
              </tr>
              <tr>
                <td class="py-3 px-5 font-semibold text-white">Kanban Hybride + Sous-tickets Enfants + Pointage RAF</td>
                <td class="py-3 px-5 text-emerald-400">✓ Inclus à 100%</td>
                <td class="py-3 px-5 text-emerald-400">✓ Inclus à 100%</td>
              </tr>
              <tr>
                <td class="py-3 px-5 font-semibold text-white">Moteur Financier (Budget, Consommé, RAF, Atterrissage)</td>
                <td class="py-3 px-5 text-emerald-400">✓ Inclus à 100%</td>
                <td class="py-3 px-5 text-emerald-400">✓ Inclus à 100%</td>
              </tr>
              <tr>
                <td class="py-3 px-5 font-semibold text-white">Base de données</td>
                <td class="py-3 px-5">Fichier local <code>aigle.db</code> (SQLite WAL embarqué)</td>
                <td class="py-3 px-5"><strong>PostgreSQL 16</strong> en conteneur isolé (ou SQLite WAL)</td>
              </tr>
              <tr>
                <td class="py-3 px-5 font-semibold text-white">Authentification & Sécurité</td>
                <td class="py-3 px-5">Exécution locale directe sur votre poste (Win/Mac/Linux)</td>
                <td class="py-3 px-5"><strong>Authelia SSO (ForwardAuth + 2FA)</strong>, RBAC 5 rôles, Rate-Limiting, Audit Logs</td>
              </tr>
              <tr>
                <td class="py-3 px-5 font-semibold text-white">Multi-Tenancy & Console d'Administration</td>
                <td class="py-3 px-5">Multi-projets sur un espace local</td>
                <td class="py-3 px-5"><strong>Multi-Workspaces (Tenants isolés)</strong> + Console Back-Office SuperAdmin</td>
              </tr>
            </tbody>
          </table>
        </div>
      {/if}
    </section>
  </main>

  <!-- Pied de page Public -->
  <footer class="border-t border-slate-900 bg-slate-950 px-6 py-6 text-xs text-slate-500">
    <div class="max-w-7xl mx-auto flex flex-wrap items-center justify-between gap-4">
      <div>
        🦅 <strong class="text-slate-300">Aigle Project Management Suite</strong> — Propulsé par Go & Svelte 5 • Heimerdinger Studio
      </div>
      <div class="flex items-center gap-4">
        <a href="#downloads" class="hover:text-slate-300">Téléchargements (Win/Mac/Linux)</a>
        <span>•</span>
        <button onclick={onEnterSaaS} class="text-indigo-400 hover:underline cursor-pointer">
          Accès SaaS Protégé (/app)
        </button>
      </div>
    </div>
  </footer>
</div>

