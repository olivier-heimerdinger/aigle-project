const BASE = '/api';

async function request(method, path, body = undefined) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
  };
  if (body !== undefined) {
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`${BASE}${path}`, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || `Erreur HTTP ${res.status}`);
  }
  return data;
}

export const api = {
  getHealth: () => request('GET', '/health'),
  getProjects: () => request('GET', '/projects'),
  createProject: (p) => request('POST', '/projects', p),
  updateProject: (id, p) => request('PUT', `/projects/${id}`, p),
  deleteProject: (id) => request('DELETE', `/projects/${id}`),
  getProjectFull: (id) => request('GET', `/projects/${id}/full`),

  // Members
  getMembers: () => request('GET', '/members'),
  createMember: (m) => request('POST', '/members', m),
  updateMember: (id, m) => request('PUT', `/members/${id}`, m),

  // WBS & Gantt
  createWBSNode: (projectId, n) => request('POST', `/projects/${projectId}/wbs`, n),
  updateWBSNode: (nodeId, n) => request('PUT', `/wbs/${nodeId}`, n),
  deleteWBSNode: (nodeId) => request('DELETE', `/wbs/${nodeId}`),
  createDependency: (projectId, d) => request('POST', `/projects/${projectId}/dependencies`, d),
  deleteDependency: (depId) => request('DELETE', `/dependencies/${depId}`),

  // Tickets & Kanban
  createTicket: (projectId, t) => request('POST', `/projects/${projectId}/tickets`, t),
  updateTicket: (ticketId, t) => request('PUT', `/tickets/${ticketId}`, t),
  moveTicket: (ticketId, payload) => request('PATCH', `/tickets/${ticketId}/move`, payload),
  deleteTicket: (ticketId) => request('DELETE', `/tickets/${ticketId}`),
  addTimeLog: (ticketId, log) => request('POST', `/tickets/${ticketId}/timelogs`, log),

  // Docs (Wiki)
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

