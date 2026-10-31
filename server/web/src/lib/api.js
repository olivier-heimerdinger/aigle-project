// Client API HTTP pour Aigle Server (Édition SaaS Multi-Tenant & Authelia SSO).
// Gère automatiquement l'en-tête X-Workspace-ID, le compte personnel (Argon2id + 2FA TOTP)
// et la matrice de permissions granulaires par projet.
const BASE = '/api';

let currentWorkspaceId = '';

export function setActiveWorkspaceId(wsId) {
  currentWorkspaceId = wsId || '';
}

export function getActiveWorkspaceId() {
  return currentWorkspaceId;
}

async function request(method, path, body = undefined) {
  const headers = { 'Content-Type': 'application/json' };
  if (currentWorkspaceId) {
    headers['X-Workspace-ID'] = currentWorkspaceId;
  }
  const opts = { method, headers };
  if (body !== undefined) {
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`${BASE}${path}`, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.error || `Erreur HTTP ${res.status}`);
    err.status = res.status;
    err.code = data.code || '';
    err.data = data;
    throw err;
  }
  return data;
}

export const api = {
  // Santé & Identité SSO Authelia
  getHealth: () => request('GET', '/health'),
  getMe: () => request('GET', '/me'),

  // Gestion du Compte Personnel : Mot de passe Argon2id / Authelia & 2FA TOTP (TODO #1 & #3)
  getAccount: () => request('GET', '/account'),
  updateAccountProfile: (payload) => request('PUT', '/account/profile', payload),
  changePassword: (payload) => request('POST', '/account/password', payload),
  setupTOTP: () => request('POST', '/account/totp/setup'),
  activateTOTP: (code) => request('POST', '/account/totp/activate', { code }),
  disableTOTP: (code) => request('POST', '/account/totp/disable', { code }),

  // Annuaire des utilisateurs du Workspace actif
  getWorkspaceUsers: () => request('GET', '/workspace/users'),

  // Console d'Administration SaaS (/api/admin/...)
  getAdminOverview: () => request('GET', '/admin/overview'),
  getAdminWorkspaces: () => request('GET', '/admin/workspaces'),
  createAdminWorkspace: (ws) => request('POST', '/admin/workspaces', ws),
  updateAdminWorkspace: (id, ws) => request('PUT', `/admin/workspaces/${id}`, ws),
  getAdminUsers: () => request('GET', '/admin/users'),
  createAdminUser: (u) => request('POST', '/admin/users', u),
  updateAdminUser: (id, u) => request('PUT', `/admin/users/${id}`, u),
  getAdminAuditLogs: () => request('GET', '/admin/audit-logs'),

  // Portefeuille Projets Multi-Tenant, Maîtres de Projet, Droits Fins & 2FA Projet (TODO #2 & #3)
  getProjects: () => request('GET', '/projects'),
  createProject: (p) => request('POST', '/projects', p),
  updateProject: (id, p) => request('PUT', `/projects/${id}`, p),
  deleteProject: (id) => request('DELETE', `/projects/${id}`),
  getProjectFull: (id) => request('GET', `/projects/${id}/full`),
  getProjectMembers: (projectId) => request('GET', `/projects/${projectId}/members`),
  saveProjectMember: (projectId, member) => request('POST', `/projects/${projectId}/members`, member),
  deleteProjectMember: (projectId, userId) => request('DELETE', `/projects/${projectId}/members/${userId}`),
  updateProjectSecurity: (projectId, sec) => request('PUT', `/projects/${projectId}/security`, sec),
  verifyProjectTOTP: (projectId, code) => request('POST', `/projects/${projectId}/verify-totp`, { code }),

  // Intervenants & TJM (Members)
  getMembers: () => request('GET', '/members'),
  createMember: (m) => request('POST', '/members', m),
  updateMember: (id, m) => request('PUT', `/members/${id}`, m),

  // WBS & Gantt (MS Project)
  createWBSNode: (projectId, n) => request('POST', `/projects/${projectId}/wbs`, n),
  updateWBSNode: (nodeId, n) => request('PUT', `/wbs/${nodeId}`, n),
  deleteWBSNode: (nodeId) => request('DELETE', `/wbs/${nodeId}`),
  createDependency: (projectId, d) => request('POST', `/projects/${projectId}/dependencies`, d),
  deleteDependency: (depId) => request('DELETE', `/dependencies/${depId}`),

  // Tickets Kanban & Sous-tickets Enfants
  createTicket: (projectId, t) => request('POST', `/projects/${projectId}/tickets`, t),
  updateTicket: (ticketId, t) => request('PUT', `/tickets/${ticketId}`, t),
  moveTicket: (ticketId, payload) => request('PATCH', `/tickets/${ticketId}/move`, payload),
  deleteTicket: (ticketId) => request('DELETE', `/tickets/${ticketId}`),
  addTimeLog: (ticketId, log) => request('POST', `/tickets/${ticketId}/timelogs`, log),

  // Documentation (Wiki Markdown)
  createDoc: (projectId, d) => request('POST', `/projects/${projectId}/docs`, d),
  updateDoc: (docId, d) => request('PUT', `/docs/${docId}`, d),
  deleteDoc: (docId) => request('DELETE', `/docs/${docId}`),
};

export function formatCurrency(amount, currency = 'EUR') {
  const num = Number(amount || 0);
  return new Intl.NumberFormat('fr-FR', {
    style: 'currency',
    currency: currency || 'EUR',
    maximumFractionDigits: 0,
  }).format(num);
}

export function formatHours(h) {
  const num = Number(h || 0);
  const days = (num / 8).toFixed(1).replace('.0', '');
  return `${num}h (${days}j)`;
}

export const ROLE_BADGES = {
  superadmin: { label: 'SuperAdmin SaaS', class: 'bg-rose-500/20 text-rose-300 border-rose-500/40' },
  admin: { label: 'Admin Tenant', class: 'bg-amber-500/20 text-amber-300 border-amber-500/40' },
  manager: { label: 'Chef de Projet', class: 'bg-indigo-500/20 text-indigo-300 border-indigo-500/40' },
  member: { label: 'Collaborateur', class: 'bg-emerald-500/20 text-emerald-300 border-emerald-500/40' },
  viewer: { label: 'Lecture Seule', class: 'bg-slate-500/20 text-slate-300 border-slate-500/40' },
};

export const PROJECT_ROLE_PRESETS = {
  owner: {
    label: '👑 Maître du Projet (Owner)',
    badge: 'bg-amber-500/20 text-amber-300 border-amber-500/40',
    desc: 'Contrôle total sur le projet, ses membres, leurs droits fins et la politique 2FA TOTP.',
    perms: {
      can_read: true,
      can_write_tickets: true,
      can_delete_tickets: true,
      can_execute: true,
      can_edit_gantt: true,
      can_edit_wiki: true,
      can_view_finance: true,
      can_manage_members: true,
    },
  },
  manager: {
    label: '🎯 Chef de Projet',
    badge: 'bg-indigo-500/20 text-indigo-300 border-indigo-500/40',
    desc: 'Pilote le planning Gantt, le budget, les tickets et les membres du projet.',
    perms: {
      can_read: true,
      can_write_tickets: true,
      can_delete_tickets: true,
      can_execute: true,
      can_edit_gantt: true,
      can_edit_wiki: true,
      can_view_finance: true,
      can_manage_members: true,
    },
  },
  contributor: {
    label: '🛠️ Contributeur',
    badge: 'bg-emerald-500/20 text-emerald-300 border-emerald-500/40',
    desc: 'Crée/modifie les tickets, exécute et pointe ses heures, édite le Gantt et le Wiki.',
    perms: {
      can_read: true,
      can_write_tickets: true,
      can_delete_tickets: false,
      can_execute: true,
      can_edit_gantt: true,
      can_edit_wiki: true,
      can_view_finance: true,
      can_manage_members: false,
    },
  },
  executor: {
    label: '⚡ Exécutant (Opérateur)',
    badge: 'bg-sky-500/20 text-sky-300 border-sky-500/40',
    desc: 'Consulte le projet, déplace les tickets Kanban et pointe ses heures (sans modifier la structure).',
    perms: {
      can_read: true,
      can_write_tickets: false,
      can_delete_tickets: false,
      can_execute: true,
      can_edit_gantt: false,
      can_edit_wiki: false,
      can_view_finance: false,
      can_manage_members: false,
    },
  },
  viewer: {
    label: '👁️ Lecteur (Observateur)',
    badge: 'bg-slate-500/20 text-slate-300 border-slate-500/40',
    desc: 'Consultation en lecture seule (aucune écriture, aucun déplacement ni pointage).',
    perms: {
      can_read: true,
      can_write_tickets: false,
      can_delete_tickets: false,
      can_execute: false,
      can_edit_gantt: false,
      can_edit_wiki: false,
      can_view_finance: false,
      can_manage_members: false,
    },
  },
};

export const TICKET_TYPES = {
  task: { label: 'Tâche', badge: 'bg-sky-500/15 text-sky-300 border-sky-500/30', icon: '⚡' },
  feature: { label: 'Évolution', badge: 'bg-indigo-500/15 text-indigo-300 border-indigo-500/30', icon: '✨' },
  bug: { label: 'Bug', badge: 'bg-rose-500/15 text-rose-300 border-rose-500/30', icon: '🐞' },
  risk: { label: 'Risque', badge: 'bg-amber-500/15 text-amber-300 border-amber-500/30', icon: '⚠️' },
  question: { label: 'Question', badge: 'bg-purple-500/15 text-purple-300 border-purple-500/30', icon: '❓' },
};

export const PRIORITIES = {
  low: { label: 'Basse', color: 'text-slate-400', dot: 'bg-slate-400' },
  medium: { label: 'Normale', color: 'text-sky-400', dot: 'bg-sky-400' },
  high: { label: 'Haute', color: 'text-amber-400', dot: 'bg-amber-400' },
  critical: { label: 'Critique', color: 'text-rose-400', dot: 'bg-rose-500' },
};

export const KANBAN_COLUMNS = [
  { id: 'backlog', label: 'Backlog', color: 'border-slate-700 bg-slate-900/60', dot: 'bg-slate-400' },
  { id: 'todo', label: 'À faire', color: 'border-sky-500/30 bg-slate-900/60', dot: 'bg-sky-400' },
  { id: 'in_progress', label: 'En cours', color: 'border-indigo-500/40 bg-indigo-950/20', dot: 'bg-indigo-400' },
  { id: 'review', label: 'En revue / QA', color: 'border-amber-500/30 bg-slate-900/60', dot: 'bg-amber-400' },
  { id: 'done', label: 'Terminé', color: 'border-emerald-500/30 bg-emerald-950/15', dot: 'bg-emerald-400' },
];
