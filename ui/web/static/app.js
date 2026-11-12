// GoTodo front end: a single Alpine.js component that talks to the /api/ endpoints.

const TOKEN_KEY = 'gotodo.token';

const STATUSES = [
  { value: 'not_started', label: 'Not started', dot: 'bg-slate-400',   ring: 'ring-slate-400',   badge: 'bg-slate-100 text-slate-700' },
  { value: 'in_progress', label: 'In progress', dot: 'bg-sky-500',     ring: 'ring-sky-500',     badge: 'bg-sky-100 text-sky-700' },
  { value: 'in_review',   label: 'In review',   dot: 'bg-amber-500',   ring: 'ring-amber-500',   badge: 'bg-amber-100 text-amber-800' },
  { value: 'completed',   label: 'Completed',   dot: 'bg-emerald-500', ring: 'ring-emerald-500', badge: 'bg-emerald-100 text-emerald-700' },
];

class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

// Parses "YYYY-MM-DD" as a local calendar date (new Date("YYYY-MM-DD") would be UTC midnight).
function parseLocalDate(s) {
  const [y, m, d] = s.split('-').map(Number);
  return new Date(y, m - 1, d);
}

function startOfToday() {
  const now = new Date();
  return new Date(now.getFullYear(), now.getMonth(), now.getDate());
}

document.addEventListener('alpine:init', () => {
  Alpine.data('app', () => ({
    statuses: STATUSES,
    token: localStorage.getItem(TOKEN_KEY),
    user: null,
    todos: [],
    loading: false,
    busy: false,

    authMode: 'login',
    authForm: { first_name: '', last_name: '', email: '', password: '' },
    authError: '',

    filter: 'all',
    search: '',
    sort: 'created',

    editor: { open: false, id: null, title: '', description: '', status: 'not_started', due_date: '', error: '' },
    profile: { open: false, first_name: '', last_name: '', error: '' },
    confirmDelete: null,
    toasts: [],

    async init() {
      if (this.token) await this.loadSession();
    },

    // ── HTTP ──────────────────────────────────────────────
    async api(method, path, body) {
      const headers = { 'Content-Type': 'application/json' };
      if (this.token) headers.Authorization = `Bearer ${this.token}`;
      const res = await fetch(`/api${path}`, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      if (res.status === 204) return null;
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        if (res.status === 401 && this.token && !path.startsWith('/auth/')) {
          this.logout();
          this.authError = 'Your session has expired. Please log in again.';
        }
        throw new ApiError(res.status, data.error || `Request failed (${res.status})`);
      }
      return data;
    },

    toast(message, kind = 'info') {
      const id = Date.now() + Math.random();
      this.toasts.push({ id, message, kind });
      setTimeout(() => { this.toasts = this.toasts.filter((t) => t.id !== id); }, 3500);
    },

    // ── Auth ──────────────────────────────────────────────
    async login() {
      this.busy = true;
      this.authError = '';
      try {
        const res = await this.api('POST', '/auth/login', {
          email: this.authForm.email,
          password: this.authForm.password,
        });
        this.token = res.token;
        localStorage.setItem(TOKEN_KEY, res.token);
        this.user = res.user;
        this.authForm = { first_name: '', last_name: '', email: '', password: '' };
        await this.loadTodos();
      } catch (e) {
        this.authError = e.message;
      } finally {
        this.busy = false;
      }
    },

    async register() {
      this.busy = true;
      this.authError = '';
      try {
        await this.api('POST', '/auth/register', { ...this.authForm });
      } catch (e) {
        this.authError = e.message;
        this.busy = false;
        return;
      }
      this.busy = false;
      await this.login();
      if (this.token) this.toast('Welcome aboard! 🎉');
    },

    async loadSession() {
      try {
        this.user = await this.api('GET', '/users/me');
        await this.loadTodos();
      } catch (e) {
        if (e.status !== 401) this.toast(e.message, 'error');
      }
    },

    logout() {
      localStorage.removeItem(TOKEN_KEY);
      this.token = null;
      this.user = null;
      this.todos = [];
      this.filter = 'all';
      this.search = '';
      this.editor.open = false;
      this.profile.open = false;
      this.confirmDelete = null;
    },

    initials() {
      if (!this.user) return '';
      return (this.user.first_name[0] + this.user.last_name[0]).toUpperCase();
    },

    openProfile() {
      this.profile = { open: true, first_name: this.user.first_name, last_name: this.user.last_name, error: '' };
    },

    async saveProfile() {
      this.busy = true;
      this.profile.error = '';
      try {
        this.user = await this.api('PUT', '/users/me', {
          first_name: this.profile.first_name,
          last_name: this.profile.last_name,
        });
        this.profile.open = false;
        this.toast('Profile updated');
      } catch (e) {
        this.profile.error = e.message;
      } finally {
        this.busy = false;
      }
    },

    // ── Todos ─────────────────────────────────────────────
    async loadTodos() {
      this.loading = true;
      try {
        this.todos = await this.api('GET', '/todos');
      } catch (e) {
        if (e.status !== 401) this.toast(e.message, 'error');
      } finally {
        this.loading = false;
      }
    },

    visibleTodos() {
      const q = this.search.trim().toLowerCase();
      const list = this.todos.filter((t) =>
        (this.filter === 'all' || t.status === this.filter) &&
        (!q || t.title.toLowerCase().includes(q) || t.description.toLowerCase().includes(q)));

      const byDue = (a, b) => {
        if (!a.due_date && !b.due_date) return 0;
        if (!a.due_date) return 1;
        if (!b.due_date) return -1;
        return a.due_date.localeCompare(b.due_date);
      };
      const sorters = {
        // Go trims trailing zeros in RFC 3339 fractions, so compare parsed times, not strings.
        created: (a, b) => Date.parse(b.created_at) - Date.parse(a.created_at) || b.id - a.id,
        updated: (a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at),
        due: byDue,
      };
      return list.sort(sorters[this.sort]);
    },

    countBy(status) {
      return this.todos.filter((t) => t.status === status).length;
    },

    summary() {
      const open = this.todos.filter((t) => t.status !== 'completed').length;
      const overdue = this.todos.filter((t) => this.isOverdue(t)).length;
      if (this.todos.length === 0) return 'Your list is empty.';
      let s = `${open} open of ${this.todos.length} total`;
      if (overdue) s += ` · ${overdue} overdue`;
      return s;
    },

    statusMeta(value) {
      return STATUSES.find((s) => s.value === value) || STATUSES[0];
    },

    openCreate() {
      this.editor = {
        open: true, id: null, title: '', description: '',
        status: this.filter === 'all' ? 'not_started' : this.filter,
        due_date: '', error: '',
      };
      this.$nextTick(() => this.$refs.titleInput && this.$refs.titleInput.focus());
    },

    openEdit(todo) {
      this.editor = {
        open: true, id: todo.id, title: todo.title, description: todo.description,
        status: todo.status, due_date: todo.due_date || '', error: '',
      };
      this.$nextTick(() => this.$refs.titleInput && this.$refs.titleInput.focus());
    },

    replaceTodo(updated) {
      this.todos = this.todos.map((t) => (t.id === updated.id ? updated : t));
    },

    async saveTodo() {
      this.busy = true;
      this.editor.error = '';
      const body = {
        title: this.editor.title,
        description: this.editor.description,
        status: this.editor.status,
        due_date: this.editor.due_date || null,
      };
      try {
        if (this.editor.id) {
          this.replaceTodo(await this.api('PUT', `/todos/${this.editor.id}`, body));
          this.toast('Todo updated');
        } else {
          this.todos.unshift(await this.api('POST', '/todos', body));
          this.toast('Todo created');
        }
        this.editor.open = false;
      } catch (e) {
        this.editor.error = e.message;
      } finally {
        this.busy = false;
      }
    },

    async setStatus(todo, status) {
      if (todo.status === status) return;
      try {
        this.replaceTodo(await this.api('PUT', `/todos/${todo.id}`, {
          title: todo.title,
          description: todo.description,
          status,
          due_date: todo.due_date,
        }));
      } catch (e) {
        this.toast(e.message, 'error');
        this.todos = [...this.todos]; // re-render to reset the select
      }
    },

    toggleComplete(todo) {
      return this.setStatus(todo, todo.status === 'completed' ? 'not_started' : 'completed');
    },

    async deleteTodo(todo) {
      try {
        await this.api('DELETE', `/todos/${todo.id}`);
        this.todos = this.todos.filter((t) => t.id !== todo.id);
        this.toast('Todo deleted');
      } catch (e) {
        this.toast(e.message, 'error');
      } finally {
        this.confirmDelete = null;
      }
    },

    // ── Dates ─────────────────────────────────────────────
    isOverdue(todo) {
      return !!todo.due_date && todo.status !== 'completed' && parseLocalDate(todo.due_date) < startOfToday();
    },

    dueLabel(todo) {
      if (!todo.due_date) return '';
      const days = Math.round((parseLocalDate(todo.due_date) - startOfToday()) / 86400000);
      const date = parseLocalDate(todo.due_date).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
      if (todo.status === 'completed') return `Due ${date}`;
      if (days < 0) return `Overdue · ${date}`;
      if (days === 0) return 'Due today';
      if (days === 1) return 'Due tomorrow';
      if (days <= 7) return `Due in ${days} days`;
      return `Due ${date}`;
    },

    dueClass(todo) {
      if (!todo.due_date || todo.status === 'completed') return 'bg-slate-100 text-slate-500';
      if (this.isOverdue(todo)) return 'bg-rose-100 text-rose-700';
      const days = Math.round((parseLocalDate(todo.due_date) - startOfToday()) / 86400000);
      return days <= 2 ? 'bg-orange-100 text-orange-700' : 'bg-indigo-50 text-indigo-700';
    },

    fromNow(iso) {
      const diff = (Date.now() - new Date(iso).getTime()) / 1000;
      const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
      const units = [['year', 31536000], ['month', 2592000], ['week', 604800], ['day', 86400], ['hour', 3600], ['minute', 60]];
      for (const [unit, secs] of units) {
        if (Math.abs(diff) >= secs) return rtf.format(-Math.round(diff / secs), unit);
      }
      return 'just now';
    },

    fullDate(iso) {
      return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
    },
  }));
});
