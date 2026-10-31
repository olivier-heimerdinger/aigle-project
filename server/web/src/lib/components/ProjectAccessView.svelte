<script>
  import { onMount } from 'svelte';
  import { api, PROJECT_ROLE_PRESETS } from '../api.js';

  let { project, myPermissions = {}, onRefresh = () => {}, onOpenAccountModal = () => {} } = $props();

  let workspaceUsers = $state([]);
  let projectMembers = $state([]);
  let ownerUserId = $state('');
  let requireTotp = $state(false);
  let statusMsg = $state({ type: '', text: '' });

  // Formulaire d'ajout d'un membre au projet
  let selectedUserId = $state('');
  let selectedRole = $state('contributor');
  let customPerms = $state({ ...PROJECT_ROLE_PRESETS.contributor.perms });

  let canManage = $derived(Boolean(myPermissions?.can_manage_members));

  function applyRolePreset(roleKey) {
    selectedRole = roleKey;
    const preset = PROJECT_ROLE_PRESETS[roleKey] || PROJECT_ROLE_PRESETS.contributor;
    customPerms = { ...preset.perms };
  }

  async function loadAccessData() {
    if (!project?.id) return;
    try {
      const [uList, pmData] = await Promise.all([
        api.getWorkspaceUsers(),
        api.getProjectMembers(project.id),
      ]);
      workspaceUsers = uList || [];
      projectMembers = pmData.members || [];
      ownerUserId = pmData.owner_user_id || project.owner_user_id || '';
      requireTotp = Boolean(pmData.require_totp);
      if (!selectedUserId && workspaceUsers.length > 0) {
        selectedUserId = workspaceUsers[0].id;
      }
    } catch (err) {
      statusMsg = { type: 'error', text: err.message };
    }
  }

  async function handleAddOrUpdateMember(e) {
    e.preventDefault();
    statusMsg = { type: '', text: '' };
    if (!selectedUserId) return;
    const usr = workspaceUsers.find((u) => u.id === selectedUserId);
    try {
      await api.saveProjectMember(project.id, {
        user_id: selectedUserId,
        user_email: usr?.email || '',
        user_name: usr?.display_name || usr?.username || '',
        project_role: selectedRole,
        ...customPerms,
        apply_preset: false,
      });
      statusMsg = { type: 'success', text: `Droits enregistrés pour ${usr?.display_name || usr?.email}.` };
      await loadAccessData();
      onRefresh();
    } catch (err) {
      statusMsg = { type: 'error', text: err.message };
    }
  }

  async function toggleMemberPermission(member, field, value) {
    if (!canManage) return;
    const updated = { ...member, [field]: value };
    try {
      await api.saveProjectMember(project.id, updated);
      await loadAccessData();
      onRefresh();
    } catch (err) {
      statusMsg = { type: 'error', text: err.message };
    }
  }

  async function changeMemberRole(member, newRole) {
    if (!canManage) return;
    try {
      await api.saveProjectMember(project.id, {
        ...member,
        project_role: newRole,
        apply_preset: true,
      });
      statusMsg = { type: 'success', text: `Rôle de ${member.user_name || member.user_email} changé en ${PROJECT_ROLE_PRESETS[newRole]?.label || newRole}.` };
      await loadAccessData();
      onRefresh();
    } catch (err) {
      statusMsg = { type: 'error', text: err.message };
    }
  }

  async function removeMember(userId) {
    if (!canManage) return;
    try {
      await api.deleteProjectMember(project.id, userId);
      await loadAccessData();
      onRefresh();
    } catch (err) {
      statusMsg = { type: 'error', text: err.message };
    }
  }

  async function saveSecuritySettings() {
    statusMsg = { type: '', text: '' };
    try {
      await api.updateProjectSecurity(project.id, {
        owner_user_id: ownerUserId,
        require_totp: requireTotp,
      });
      statusMsg = {
        type: 'success',
        text: requireTotp
          ? '🔐 Politique de sécurité enregistrée : Double Authentification 2FA TOTP activée sur ce projet (et synchronisée avec Authelia).'
          : '✅ Politique de sécurité du projet mise à jour.',
      };
      await loadAccessData();
      onRefresh();
    } catch (err) {
      requireTotp = Boolean(project.require_totp);
      statusMsg = { type: 'error', text: err.message };
    }
  }

  $effect(() => {
    if (project?.id) {
      loadAccessData();
    }
  });

  onMount(loadAccessData);
</script>

<div class="space-y-6 text-xs">
  {#if statusMsg.text}
    <div
      class="p-3.5 rounded-xl border flex items-center justify-between {statusMsg.type === 'success'
        ? 'bg-emerald-950/40 border-emerald-500/40 text-emerald-300'
        : 'bg-rose-950/40 border-rose-500/40 text-rose-300'}"
    >
      <span>{statusMsg.text}</span>
      {#if statusMsg.type === 'error' && statusMsg.text.includes('Mon Compte')}
        <button
          onclick={onOpenAccountModal}
          class="px-3 py-1 rounded-lg bg-indigo-600 text-white font-semibold cursor-pointer shrink-0"
        >
          👤 Ouvrir Mon Compte (Activer 2FA)
        </button>
      {/if}
    </div>
  {/if}

  <!-- 1. Gouvernance : Maître du Projet & Double Authentification 2FA TOTP -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
    <!-- Maître Principal du Projet -->
    <div class="p-5 rounded-2xl bg-slate-900 border border-slate-800 space-y-3">
      <div class="flex items-center justify-between">
        <div>
          <h3 class="text-sm font-bold text-white flex items-center gap-2">
            <span>👑 Maître du Projet (Owner Principal)</span>
          </h3>
          <p class="text-[11px] text-slate-400">
            Le Maître du Projet dispose de l'intégralité des droits d'administration, de gestion financière et d'attribution des accès sur ce projet.
          </p>
        </div>
      </div>

      <div class="flex items-center gap-2 pt-1">
        <select
          disabled={!canManage}
          bind:value={ownerUserId}
          class="flex-1 px-3 py-2 rounded-xl bg-slate-950 border border-slate-700 text-white font-medium"
        >
          {#each workspaceUsers as u (u.id)}
            <option value={u.id}>
              {u.display_name || u.username} ({u.email}) — [{u.role}]
            </option>
          {/each}
        </select>
        {#if canManage}
          <button
            onclick={saveSecuritySettings}
            class="px-4 py-2 rounded-xl bg-amber-600 hover:bg-amber-500 text-white font-semibold cursor-pointer shrink-0"
          >
            Définir Maître
          </button>
        {/if}
      </div>
    </div>

    <!-- Protection 2FA TOTP par Projet -->
    <div class="p-5 rounded-2xl bg-slate-900 border {requireTotp ? 'border-emerald-500/50' : 'border-slate-800'} space-y-3">
      <div class="flex items-start justify-between gap-3">
        <div>
          <h3 class="text-sm font-bold text-white flex items-center gap-2">
            <span>🔐 Double Authentification (2FA TOTP) sur ce Projet</span>
            {#if requireTotp}
              <span class="px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 font-mono text-[10px]">2FA OBLIGATOIRE</span>
            {:else}
              <span class="px-2 py-0.5 rounded bg-slate-800 text-slate-400 font-mono text-[10px]">1FA STANDARD</span>
            {/if}
          </h3>
          <p class="text-[11px] text-slate-400 mt-0.5">
            Lorsque cette option est activée, Aigle synchronise une règle <code class="text-indigo-300">two_factor</code> dans Authelia et exige la validation d'un code TOTP à 6 chiffres (Google Authenticator / Authy) avant d'autoriser la lecture ou la modification de ce projet.
          </p>
        </div>
      </div>

      <div class="flex items-center justify-between pt-1 border-t border-slate-800/80">
        <label class="flex items-center gap-2.5 cursor-pointer select-none">
          <input
            type="checkbox"
            disabled={!canManage}
            bind:checked={requireTotp}
            onchange={saveSecuritySettings}
            class="w-4 h-4 accent-emerald-500 rounded cursor-pointer"
          />
          <span class="font-semibold text-slate-200">
            Exiger un code 2FA TOTP à 6 chiffres pour accéder au projet « {project.code} »
          </span>
        </label>
        <button
          onclick={onOpenAccountModal}
          class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-indigo-300 text-[11px] cursor-pointer shrink-0"
        >
          📱 Configurer mon TOTP
        </button>
      </div>
    </div>
  </div>

  <!-- 2. Formulaire d'ajout d'un utilisateur au projet avec Rôle Prédéfini + Cases à cocher fines -->
  {#if canManage}
    <form onsubmit={handleAddOrUpdateMember} class="p-5 rounded-2xl bg-slate-900 border border-indigo-500/30 space-y-4">
      <div class="flex items-center justify-between">
        <div>
          <h3 class="text-sm font-bold text-white">➕ Ajouter ou Configurer un Utilisateur sur ce Projet</h3>
          <p class="text-[11px] text-slate-400">
            Choisissez un profil prédéfini (Maître, Chef de projet, Contributeur, Exécutant, Lecteur) puis ajustez librement ses permissions fines case par case.
          </p>
        </div>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label for="sel-ws-user" class="block text-slate-400 mb-1">Utilisateur du Workspace</label>
          <select
            id="sel-ws-user"
            bind:value={selectedUserId}
            class="w-full px-3 py-2 rounded-xl bg-slate-950 border border-slate-700 text-white"
          >
            {#each workspaceUsers as u (u.id)}
              <option value={u.id}>
                {u.display_name || u.username} — {u.email} (Rôle global : {u.role})
              </option>
            {/each}
          </select>
        </div>

        <div>
          <label for="sel-proj-role" class="block text-slate-400 mb-1">Rôle Prédéfini sur le Projet</label>
          <select
            id="sel-proj-role"
            value={selectedRole}
            onchange={(e) => applyRolePreset(e.currentTarget.value)}
            class="w-full px-3 py-2 rounded-xl bg-slate-950 border border-slate-700 text-white font-semibold"
          >
            {#each Object.entries(PROJECT_ROLE_PRESETS) as [roleKey, info]}
              <option value={roleKey}>{info.label}</option>
            {/each}
          </select>
          <div class="text-[10px] text-indigo-300 mt-1">
            {PROJECT_ROLE_PRESETS[selectedRole]?.desc}
          </div>
        </div>
      </div>

      <!-- Cases à cocher des 8 permissions granulaires -->
      <div class="p-3.5 rounded-xl bg-slate-950 border border-slate-800 grid grid-cols-2 md:grid-cols-4 gap-3">
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={customPerms.can_read} class="accent-indigo-500" />
          <span>👁️ <strong>Lecture</strong> (Accès projet)</span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={customPerms.can_write_tickets} class="accent-indigo-500" />
          <span>✏️ <strong>Écriture Tickets</strong> (Créer/Éditer)</span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={customPerms.can_execute} class="accent-indigo-500" />
          <span>⚡ <strong>Exécution</strong> (Kanban & Pointage)</span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={customPerms.can_delete_tickets} class="accent-indigo-500" />
          <span>🗑️ <strong>Suppr. Tickets</strong></span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={customPerms.can_edit_gantt} class="accent-indigo-500" />
          <span>📅 <strong>Planning Gantt</strong> (WBS)</span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={customPerms.can_edit_wiki} class="accent-indigo-500" />
          <span>📚 <strong>Wiki Markdown</strong></span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={customPerms.can_view_finance} class="accent-indigo-500" />
          <span>💰 <strong>Finances & EVM</strong></span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={customPerms.can_manage_members} class="accent-amber-500" />
          <span>👑 <strong>Gestion Membres & 2FA</strong></span>
        </label>
      </div>

      <div class="flex justify-end">
        <button
          type="submit"
          class="px-5 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white font-semibold cursor-pointer"
        >
          💾 Attribuer ces droits sur le projet
        </button>
      </div>
    </form>
  {/if}

  <!-- 3. Matrice des Utilisateurs & Permissions Fines du Projet -->
  <div class="rounded-2xl bg-slate-900 border border-slate-800 overflow-hidden">
    <div class="px-5 py-3.5 bg-slate-950 border-b border-slate-800 flex items-center justify-between">
      <div>
        <h3 class="text-sm font-bold text-white">👥 Matrice des Droits par Utilisateur sur « {project.name} »</h3>
        <p class="text-[11px] text-slate-400">
          Cochez ou décochez directement une permission pour ajuster en temps réel les droits d'un utilisateur sur ce projet.
        </p>
      </div>
      <span class="px-2.5 py-1 rounded-full bg-slate-800 text-slate-300 font-mono text-[11px]">
        {projectMembers.length} membre(s) configuré(s)
      </span>
    </div>

    <div class="overflow-x-auto">
      <table class="w-full text-left border-collapse">
        <thead>
          <tr class="border-b border-slate-800 text-[10px] uppercase tracking-wider text-slate-400 bg-slate-950/50">
            <th class="py-3 px-4">Utilisateur</th>
            <th class="py-3 px-3">Rôle Projet</th>
            <th class="py-3 px-2 text-center" title="Consulter le projet">👁️ Lecture</th>
            <th class="py-3 px-2 text-center" title="Créer et modifier des tickets">✏️ Écrit. Ticket</th>
            <th class="py-3 px-2 text-center" title="Déplacer les cartes Kanban et pointer ses heures">⚡ Exécution</th>
            <th class="py-3 px-2 text-center" title="Supprimer des tickets">🗑️ Suppr. Ticket</th>
            <th class="py-3 px-2 text-center" title="Modifier le planning WBS / Gantt">📅 Gantt</th>
            <th class="py-3 px-2 text-center" title="Créer et éditer les pages Wiki">📚 Wiki</th>
            <th class="py-3 px-2 text-center" title="Consulter et éditer les budgets et TJM">💰 Finances</th>
            <th class="py-3 px-2 text-center" title="Gérer les membres, maîtres et la 2FA du projet">👑 Maître/Admin</th>
            {#if canManage}
              <th class="py-3 px-3 text-right">Action</th>
            {/if}
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/70">
          {#each projectMembers as m (m.id || m.user_id)}
            {@const roleInfo = PROJECT_ROLE_PRESETS[m.project_role] || PROJECT_ROLE_PRESETS.contributor}
            <tr class="hover:bg-slate-800/30">
              <td class="py-3 px-4">
                <div class="font-semibold text-white flex items-center gap-1.5">
                  <span>{m.user_name || 'Utilisateur'}</span>
                  {#if project.owner_user_id === m.user_id}
                    <span class="px-1.5 py-0.5 rounded bg-amber-500/20 text-amber-300 text-[9px] font-mono">MAÎTRE PRINCIPAL</span>
                  {/if}
                </div>
                <div class="text-[10px] text-slate-400 font-mono">{m.user_email}</div>
              </td>

              <td class="py-3 px-3">
                {#if canManage}
                  <select
                    value={m.project_role}
                    onchange={(e) => changeMemberRole(m, e.currentTarget.value)}
                    class="px-2 py-1 rounded-lg bg-slate-950 border border-slate-700 text-xs text-white font-medium"
                  >
                    {#each Object.entries(PROJECT_ROLE_PRESETS) as [rk, rInfo]}
                      <option value={rk}>{rInfo.label}</option>
                    {/each}
                  </select>
                {:else}
                  <span class="px-2 py-0.5 rounded border text-[10px] font-semibold {roleInfo.badge}">
                    {roleInfo.label}
                  </span>
                {/if}
              </td>

              <td class="py-3 px-2 text-center">
                <input
                  type="checkbox"
                  disabled={!canManage || m.project_role === 'owner'}
                  checked={m.can_read}
                  onchange={(e) => toggleMemberPermission(m, 'can_read', e.currentTarget.checked)}
                  class="accent-indigo-500 cursor-pointer"
                />
              </td>
              <td class="py-3 px-2 text-center">
                <input
                  type="checkbox"
                  disabled={!canManage || m.project_role === 'owner'}
                  checked={m.can_write_tickets}
                  onchange={(e) => toggleMemberPermission(m, 'can_write_tickets', e.currentTarget.checked)}
                  class="accent-indigo-500 cursor-pointer"
                />
              </td>
              <td class="py-3 px-2 text-center">
                <input
                  type="checkbox"
                  disabled={!canManage || m.project_role === 'owner'}
                  checked={m.can_execute}
                  onchange={(e) => toggleMemberPermission(m, 'can_execute', e.currentTarget.checked)}
                  class="accent-indigo-500 cursor-pointer"
                />
              </td>
              <td class="py-3 px-2 text-center">
                <input
                  type="checkbox"
                  disabled={!canManage || m.project_role === 'owner'}
                  checked={m.can_delete_tickets}
                  onchange={(e) => toggleMemberPermission(m, 'can_delete_tickets', e.currentTarget.checked)}
                  class="accent-indigo-500 cursor-pointer"
                />
              </td>
              <td class="py-3 px-2 text-center">
                <input
                  type="checkbox"
                  disabled={!canManage || m.project_role === 'owner'}
                  checked={m.can_edit_gantt}
                  onchange={(e) => toggleMemberPermission(m, 'can_edit_gantt', e.currentTarget.checked)}
                  class="accent-indigo-500 cursor-pointer"
                />
              </td>
              <td class="py-3 px-2 text-center">
                <input
                  type="checkbox"
                  disabled={!canManage || m.project_role === 'owner'}
                  checked={m.can_edit_wiki}
                  onchange={(e) => toggleMemberPermission(m, 'can_edit_wiki', e.currentTarget.checked)}
                  class="accent-indigo-500 cursor-pointer"
                />
              </td>
              <td class="py-3 px-2 text-center">
                <input
                  type="checkbox"
                  disabled={!canManage || m.project_role === 'owner'}
                  checked={m.can_view_finance}
                  onchange={(e) => toggleMemberPermission(m, 'can_view_finance', e.currentTarget.checked)}
                  class="accent-indigo-500 cursor-pointer"
                />
              </td>
              <td class="py-3 px-2 text-center">
                <input
                  type="checkbox"
                  disabled={!canManage || m.project_role === 'owner'}
                  checked={m.can_manage_members}
                  onchange={(e) => toggleMemberPermission(m, 'can_manage_members', e.currentTarget.checked)}
                  class="accent-amber-500 cursor-pointer"
                />
              </td>

              {#if canManage}
                <td class="py-3 px-3 text-right">
                  {#if project.owner_user_id !== m.user_id}
                    <button
                      onclick={() => removeMember(m.user_id)}
                      class="px-2 py-1 rounded bg-rose-950/60 hover:bg-rose-900 text-rose-300 text-[10px] cursor-pointer"
                    >
                      Retirer
                    </button>
                  {/if}
                </td>
              {/if}
            </tr>
          {:else}
            <tr>
              <td colspan="11" class="py-6 text-center text-slate-400">
                Aucun membre spécifique configuré : les droits par défaut du Workspace s'appliquent. Utilisez le formulaire ci-dessus pour désigner des Maîtres du Projet, Contributeurs, Exécutants ou Lecteurs.
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</div>

