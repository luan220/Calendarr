const { createApp } = Vue;

// Icônes Material (chemins SVG inline) — pas de police externe, marche offline.
const ICONS = {
    search: 'M15.5 14h-.79l-.28-.27A6.47 6.47 0 0 0 16 9.5 6.5 6.5 0 1 0 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z',
    share: 'M18 16.08c-.76 0-1.44.3-1.96.77L8.91 12.7c.05-.23.09-.46.09-.7s-.04-.47-.09-.7l7.05-4.11c.54.5 1.25.81 2.04.81 1.66 0 3-1.34 3-3s-1.34-3-3-3-3 1.34-3 3c0 .24.04.47.09.7L8.04 9.81C7.5 9.31 6.79 9 6 9c-1.66 0-3 1.34-3 3s1.34 3 3 3c.79 0 1.5-.31 2.04-.81l7.12 4.16c-.05.21-.08.43-.08.65 0 1.61 1.31 2.92 2.92 2.92s2.92-1.31 2.92-2.92-1.31-2.92-2.92-2.92z',
    chevron_left: 'M15.41 7.41 14 6l-6 6 6 6 1.41-1.41L10.83 12z',
    chevron_right: 'M10 6 8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6z',
    play: 'M8 5v14l11-7z',
    download: 'M5 20h14v-2H5v2zM19 9h-4V3H9v6H5l7 7 7-7z',
    schedule: 'M11.99 2C6.47 2 2 6.48 2 12s4.47 10 9.99 10C17.52 22 22 17.52 22 12S17.52 2 11.99 2zM12 20c-4.42 0-8-3.58-8-8s3.58-8 8-8 8 3.58 8 8-3.58 8-8 8zm.5-13H11v6l5.25 3.15.75-1.23-4.5-2.67z',
    close: 'M19 6.41 17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z',
    check: 'M9 16.17 4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z',
    external: 'M19 19H5V5h7V3H5c-1.11 0-2 .9-2 2v14c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2v-7h-2v7zM14 3v2h3.59l-9.83 9.83 1.41 1.41L19 6.41V10h2V3h-7z',
    movie: 'M18 4l2 4h-3l-2-4h-2l2 4h-3l-2-4H8l2 4H7L5 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V4h-4z',
    pause: 'M6 19h4V5H6v14zm8-14v14h4V5h-4z',
    trash: 'M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z',
    refresh: 'M17.65 6.35A7.958 7.958 0 0 0 12 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08A5.99 5.99 0 0 1 12 18c-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z',
    bolt: 'M7 2v11h3v9l7-12h-4l4-8z',
    plus: 'M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z',
    star: 'M12 17.27L18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z',
    settings: 'M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z',
    storage: 'M2 20h20v-4H2v4zm2-3h2v2H4v-2zM2 4v4h20V4H2zm4 3H4V5h2v2zm-4 7h20v-4H2v4zm2-3h2v2H4v-2z',
    sync: 'M12 4V1L8 5l4 4V6c3.31 0 6 2.69 6 6 0 1.01-.25 1.97-.7 2.8l1.46 1.46C19.54 15.03 20 13.57 20 12c0-4.42-3.58-8-8-8zm0 14c-3.31 0-6-2.69-6-6 0-1.01.25-1.97.7-2.8L5.24 7.74C4.46 8.97 4 10.43 4 12c0 4.42 3.58 8 8 8v3l4-4-4-4v3z',
};

const app = createApp({
    data() {
        return {
            year: 0, month: 0, monthLabel: '',
            today: '', now: Date.now(), prev: {}, next: {},
            disk: null, optsOpen: false, navStuck: false, slideDir: '', autoScroll: localStorage.getItem('autoScroll') !== 'false',
            days: {}, watched: [], stats: { episodes: 0, downloaded: 0, watched: 0 },
            loading: true, share: '', sonarrUrl: '', shareOpen: false,
            selected: null, searching: false, releases: null, grabbed: {},
            view: 'calendar',
            // Disponibilité des services (rempli par /api/status). true par défaut
            // pour ne rien masquer le temps que le statut arrive. (qBittorrent n'y
            // est pas : la page Torrents se base directement sur /api/torrents.)
            services: { sonarr: true, radarr: true, prowlarr: true },
            tabs: [
                { id: 'calendar', key: 'nav_calendar' },
                { id: 'films', key: 'nav_films' },
                { id: 'torrents', key: 'nav_torrents' },
                { id: 'prowlarr', key: 'nav_prowlarr' },
            ],
            lang: (window.detectLocale ? window.detectLocale() : 'en'),
            langs: (window.LANGS || []),
            underline: { left: 0, width: 0 },
            seriesQuery: '', seriesResults: null, seriesSearching: false,
            addTarget: null, addType: '', addForm: {}, adding: false, seriesOptions: null, movieOptions: null,
            torrents: [], torrentsLoading: false, torrentsError: '', deleteTarget: null, deleteFiles: false,
            qbitStatus: null, qbitUserInput: '', qbitPass: '', qbitConnecting: false, qbitMsg: '',
            indexers: [], prowlarrLoading: false, prowlarrError: '', prowlarrApps: [], radarrConfigured: false, connectOpen: false, connecting: false,
            schemaList: null, schemaQuery: '', schemaOpen: false, addingIndexer: '',
            films: [], filmsLoading: false, filmsError: '', radarrUrl: '',
            selectedFilm: null, filmReleases: null, filmSearching: false, filmGrabbed: {},
            progress: {}, liveProgress: false, ws: null,
        };
    },
    computed: {
        watchedSet() { return new Set(this.watched); },
        monthName() {
            if (!this.year) return this.t('nav_calendar');
            const s = new Date(this.year, this.month - 1, 1).toLocaleDateString(this.lang, { month: 'long' });
            return s.charAt(0).toUpperCase() + s.slice(1);
        },
        weekDayNames() {
            const fmt = new Intl.DateTimeFormat(this.lang, { weekday: 'long' });
            const out = [];
            for (let i = 0; i < 7; i++) { // 2024-01-01 = lundi
                const s = fmt.format(new Date(2024, 0, 1 + i));
                out.push(s.charAt(0).toUpperCase() + s.slice(1));
            }
            return out;
        },
        diskUsedPct() {
            if (!this.disk || !this.disk.total) return 0;
            return Math.round((this.disk.total - this.disk.free) / this.disk.total * 100);
        },
        diskClass() {
            const p = this.diskUsedPct;
            return p >= 90 ? 'disk-crit' : (p >= 75 ? 'disk-warn' : 'disk-ok');
        },
        weeks() {
            if (!this.year) return [];
            const first = new Date(this.year, this.month - 1, 1);
            const daysInMonth = new Date(this.year, this.month, 0).getDate();
            const startOffset = (first.getDay() + 6) % 7; // lundi = 0
            const total = Math.ceil((startOffset + daysInMonth) / 7) * 7;
            const gridStart = new Date(this.year, this.month - 1, 1 - startOffset);
            const cells = [];
            for (let i = 0; i < total; i++) {
                const d = new Date(gridStart);
                d.setDate(gridStart.getDate() + i);
                const iso = this.iso(d);
                cells.push({
                    iso, day: d.getDate(),
                    weekday: this.weekDayNames[i % 7],
                    inMonth: d.getMonth() === this.month - 1,
                    isToday: iso === this.today,
                    episodes: this.days[iso] || [],
                });
            }
            return cells;
        },
        torrentsActive() { return this.torrents.filter((t) => !this.tInfo(t.state).paused).length; },
        filteredSchema() {
            if (!this.schemaList) return [];
            const q = this.schemaQuery.trim().toLowerCase();
            const list = q ? this.schemaList.filter((x) => (x.name || '').toLowerCase().includes(q)) : this.schemaList;
            return list.slice(0, 200);
        },
    },
    methods: {
        t(key, vars) {
            const dict = window.MESSAGES[this.lang] || window.MESSAGES.en;
            let s = dict[key];
            if (s === undefined) s = window.MESSAGES.en[key];
            if (s === undefined) s = key;
            if (vars) for (const k in vars) s = s.split('{' + k + '}').join(vars[k]);
            return s;
        },
        setLang(code) {
            this.lang = code;
            try { localStorage.setItem('lang', code); } catch (e) {}
            document.documentElement.lang = code;
        },
        // Repositionne le soulignement glissant sous l'onglet actif.
        updateUnderline() {
            const tabs = document.querySelector('.tabs');
            const active = tabs && tabs.querySelector('.tab.active');
            if (!active) return;
            this.underline = { left: active.offsetLeft + 2, width: Math.max(0, active.offsetWidth - 4) };
        },
        // Ferme le menu réglages si on clique en dehors (le backdrop fixe ne marche
        // pas : la topbar a un backdrop-filter qui confine le position:fixed).
        onDocClick(e) {
            if (this.optsOpen && !e.target.closest('.opts-wrap')) this.optsOpen = false;
        },
        pad(n) { return String(n).padStart(2, '0'); },
        iso(d) { return `${d.getFullYear()}-${this.pad(d.getMonth() + 1)}-${this.pad(d.getDate())}`; },
        isWatched(ep) { return this.watchedSet.has(ep.episodeId); },
        epsFor(cell) { return cell.episodes; },
        dl(ep) {
            if (this.liveProgress) return this.progress[ep.episodeId] || null;
            if (ep.downloadStatus) {
                return { status: ep.downloadStatus, percent: ep.downloadPercent, timeleft: ep.downloadTimeleft };
            }
            return null;
        },
        statusKey(ep) {
            if (this.isWatched(ep)) return 'watched';
            if (this.dl(ep)) return 'downloading';
            return ep.hasFile ? 'available' : 'upcoming';
        },
        statusLabel(ep) {
            const d = this.dl(ep);
            if (this.isWatched(ep)) return this.t('watched');
            if (d) return this.t('status_downloading') + (d.percent ? ` · ${d.percent}%` : '');
            return ep.hasFile ? this.t('status_available') : this.t('status_upcoming');
        },
        noop() {},
        async loadDisk() {
            try { this.disk = await (await fetch('/api/diskspace')).json(); } catch (e) { this.disk = null; }
        },
        async loadStatus() {
            try {
                const r = await fetch('/api/status');
                if (r.ok) Object.assign(this.services, await r.json());
            } catch (e) {}
        },
        fmtGB(bytes) {
            const gb = (bytes || 0) / 1073741824;
            return gb >= 1024 ? (gb / 1024).toFixed(2) + ' To' : gb.toFixed(1) + ' Go';
        },
        saveAutoScroll() { localStorage.setItem('autoScroll', this.autoScroll); if (this.autoScroll) this.scrollToToday(); },
        onScroll() {
            const ph = document.querySelector('.cal-head');
            this.navStuck = ph ? ph.getBoundingClientRect().top <= 66 : false;
        },
        scrollToToday() {
            this.$nextTick(() => { const t = document.querySelector('.cell.today'); if (t) t.scrollIntoView({ block: 'center', behavior: 'smooth' }); });
        },
        relTime(cell, ep) {
            const d = this.dl(ep);
            if (d) {
                if (d.status === 'importing') return this.t('st_moving');
                if (d.status === 'pending') return this.t('st_pending');
                return d.status === 'paused' ? this.t('st_paused') : this.t('st_downloading');
            }
            if (ep.hasFile) return this.t('status_available');
            if (!cell || !cell.iso) return ep.time || '';
            const air = new Date(cell.iso + 'T' + (ep.time || '00:00'));
            const ms = air - this.now;
            if (ms <= 0) return this.t('aired');
            if (ms >= 86400000) return Math.floor(ms / 86400000) + this.t('u_d');
            const h = Math.floor(ms / 3600000);
            const m = Math.floor((ms % 3600000) / 60000);
            return h > 0 ? `${h}${this.t('u_h')}${this.pad(m)}` : `${m}${this.t('u_min')}`;
        },
        goTab(id) {
            this.view = id;
            if (id === 'torrents') this.loadTorrents();
            else if (id === 'prowlarr') this.loadProwlarr();
            else if (id === 'films') this.loadFilms();
        },
        goHome() {
            this.view = 'calendar';
            this.load(); // mois courant + scroll sur aujourd'hui
        },
        openModal(ep) { this.selected = ep; this.releases = null; this.searching = false; this.grabbed = {}; },
        closeModal() { this.selected = null; },
        fmtSize(b) {
            if (!b) return '—';
            const gb = b / 1e9;
            return gb >= 1 ? gb.toFixed(2) + ' Go' : (b / 1e6).toFixed(0) + ' Mo';
        },
        qBadge(q) {
            const s = (q || '').toLowerCase();
            if (s.includes('2160') || s.includes('4k')) return 'q-2160';
            if (s.includes('1080')) return 'q-1080';
            if (s.includes('720')) return 'q-720';
            return 'q-sd';
        },
        seedClass(n) {
            if (n >= 20) return 'seed-hi';
            if (n >= 5) return 'seed-mid';
            return 'seed-lo';
        },
        async searchTorrents() {
            if (!this.selected) return;
            this.searching = true;
            this.releases = null;
            try {
                const r = await fetch('/api/search?episodeId=' + this.selected.episodeId);
                if (!r.ok) throw new Error('search');
                this.releases = (await r.json()).releases || [];
            } catch (e) {
                this.releases = [];
                alert(this.t('alert_search_failed_sonarr'));
            } finally {
                this.searching = false;
            }
        },
        async grab(rel) {
            const st = this.grabbed[rel.guid];
            if (st === 'wait' || st === 'ok') return;
            this.grabbed[rel.guid] = 'wait';
            try {
                const r = await fetch('/api/grab', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ guid: rel.guid, indexerId: rel.indexerId }),
                });
                if (!r.ok) throw new Error('grab');
                this.grabbed[rel.guid] = 'ok';
            } catch (e) {
                this.grabbed[rel.guid] = 'err';
            }
        },
        // --- Recherche / ajout de série (Sonarr) ---
        onSeriesInput() {
            clearTimeout(this._seriesTimer);
            if (!this.seriesQuery.trim()) { this.seriesResults = null; return; }
            this._seriesTimer = setTimeout(() => this.searchSeries(), 400);
        },
        async searchSeries() {
            const term = this.seriesQuery.trim();
            if (!term) { this.seriesResults = null; return; }
            this.seriesSearching = true;
            try {
                const r = await fetch('/api/search/add?term=' + encodeURIComponent(term));
                this.seriesResults = (await r.json()).results || [];
            } catch (e) { this.seriesResults = []; }
            finally { this.seriesSearching = false; }
        },
        openAdd(r) {
            if (r.type === 'movie') this.openAddMovie(r);
            else this.openAddSeries(r);
        },
        async loadSeriesOptions() {
            try {
                const r = await fetch('/api/series/options');
                this.seriesOptions = await r.json();
            } catch (e) {
                this.seriesOptions = { qualityProfiles: [], tags: [], rootFolders: [], seriesTypes: ['standard', 'anime', 'daily'], monitorOptions: ['all', 'none'] };
            }
        },
        async openAddSeries(r) {
            this.seriesResults = null; this.seriesQuery = '';
            if (!this.seriesOptions) await this.loadSeriesOptions();
            const o = this.seriesOptions;
            let saved = {};
            try { saved = JSON.parse(localStorage.getItem('addDefaults') || '{}'); } catch (e) {}
            this.addForm = {
                monitor: saved.monitor || 'all',
                qualityProfileId: saved.qualityProfileId || (o.qualityProfiles[0] && o.qualityProfiles[0].id) || 1,
                seriesType: saved.seriesType || 'standard',
                rootFolderPath: saved.rootFolderPath || (o.rootFolders[0] && o.rootFolders[0].path) || '',
                tags: Array.isArray(saved.tags) ? [...saved.tags] : [],
                searchNow: saved.searchNow !== undefined ? saved.searchNow : true,
            };
            this.addType = 'series';
            this.addTarget = r;
        },
        async loadMovieOptions() {
            try {
                const r = await fetch('/api/movies/options');
                this.movieOptions = await r.json();
            } catch (e) {
                this.movieOptions = { qualityProfiles: [], tags: [], rootFolders: [], availabilityOptions: ['announced', 'inCinemas', 'released'] };
            }
        },
        async openAddMovie(r) {
            this.seriesResults = null; this.seriesQuery = '';
            if (!this.movieOptions) await this.loadMovieOptions();
            const o = this.movieOptions;
            let saved = {};
            try { saved = JSON.parse(localStorage.getItem('addMovieDefaults') || '{}'); } catch (e) {}
            this.addForm = {
                monitored: saved.monitored !== undefined ? saved.monitored : true,
                minimumAvailability: saved.minimumAvailability || 'released',
                qualityProfileId: saved.qualityProfileId || (o.qualityProfiles[0] && o.qualityProfiles[0].id) || 1,
                rootFolderPath: saved.rootFolderPath || (o.rootFolders[0] && o.rootFolders[0].path) || '',
                tags: Array.isArray(saved.tags) ? [...saved.tags] : [],
                searchNow: saved.searchNow !== undefined ? saved.searchNow : true,
            };
            this.addType = 'movie';
            this.addTarget = r;
        },
        toggleAddTag(id) {
            const i = this.addForm.tags.indexOf(id);
            if (i >= 0) this.addForm.tags.splice(i, 1); else this.addForm.tags.push(id);
        },
        async addItem() {
            if (!this.addTarget) return;
            const isMovie = this.addType === 'movie';
            if (!this.addForm.rootFolderPath) {
                alert(this.t('alert_no_root', { app: isMovie ? 'Radarr' : 'Sonarr' }));
                return;
            }
            this.adding = true;
            const title = this.addTarget.title;
            try {
                const url = isMovie ? '/api/movies/add' : '/api/series/add';
                const payload = isMovie ? {
                    tmdbId: this.addTarget.tmdbId,
                    qualityProfileId: this.addForm.qualityProfileId,
                    rootFolderPath: this.addForm.rootFolderPath,
                    monitored: this.addForm.monitored,
                    minimumAvailability: this.addForm.minimumAvailability,
                    searchNow: this.addForm.searchNow,
                    tags: this.addForm.tags,
                } : {
                    tvdbId: this.addTarget.tvdbId,
                    qualityProfileId: this.addForm.qualityProfileId,
                    rootFolderPath: this.addForm.rootFolderPath,
                    monitored: this.addForm.monitor !== 'none',
                    seriesType: this.addForm.seriesType,
                    monitor: this.addForm.monitor,
                    searchNow: this.addForm.searchNow,
                    tags: this.addForm.tags,
                };
                const r = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
                if (!r.ok) throw new Error((await r.text()) || 'erreur');
                localStorage.setItem(isMovie ? 'addMovieDefaults' : 'addDefaults', JSON.stringify(this.addForm));
                this.addTarget = null;
                if (isMovie) { if (this.view === 'films') this.loadFilms(); } else { this.load(this.year, this.month); }
                alert(isMovie ? this.t('alert_movie_added', { title }) : this.t('alert_series_added', { title }));
            } catch (e) { alert(this.t('alert_add_failed', { msg: e.message })); }
            finally { this.adding = false; }
        },
        // --- Torrents (qBittorrent) ---
        async loadTorrents() {
            this.torrentsLoading = true; this.torrentsError = '';
            try {
                const st = await (await fetch('/api/qbit/status')).json();
                this.qbitStatus = st;
                if (st.username && !this.qbitUserInput) this.qbitUserInput = st.username;
                if (st.connected) {
                    const r = await fetch('/api/torrents');
                    this.torrents = r.ok ? ((await r.json()).torrents || []) : [];
                } else {
                    this.torrents = [];
                }
            } catch (e) { this.qbitStatus = null; this.torrents = []; this.torrentsError = String(e); }
            finally { this.torrentsLoading = false; }
        },
        async connectQbit() {
            this.qbitConnecting = true; this.qbitMsg = '';
            try {
                const r = await fetch('/api/qbit/connect', {
                    method: 'POST', headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username: this.qbitUserInput, password: this.qbitPass }),
                });
                const d = await r.json();
                if (d.connected) { this.qbitPass = ''; this.qbitMsg = ''; await this.loadTorrents(); }
                else { this.qbitMsg = d.banned ? this.t('qbit_banned') : this.t('qbit_bad_pass'); }
            } catch (e) { this.qbitMsg = String(e); }
            finally { this.qbitConnecting = false; }
        },
        async torrentAction(action, hash, deleteFiles = false) {
            try {
                await fetch('/api/torrents/action', {
                    method: 'POST', headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ action, hash, deleteFiles }),
                });
            } catch (e) {}
            setTimeout(() => this.loadTorrents(), 600);
        },
        askDelete(hash, label) { this.deleteTarget = { hash, label }; this.deleteFiles = false; },
        confirmDelete() {
            if (!this.deleteTarget) return;
            this.torrentAction('delete', this.deleteTarget.hash, this.deleteFiles);
            this.deleteTarget = null;
        },
        tInfo(state) {
            const map = {
                downloading: ['st_downloading', 'st-dl', false], forcedDL: ['st_downloading', 'st-dl', false],
                metaDL: ['st_metadata', 'st-dl', false], stalledDL: ['st_stalled', 'st-stalled', false],
                uploading: ['st_seeding', 'st-up', false], forcedUP: ['st_seeding', 'st-up', false],
                stalledUP: ['st_seeding_idle', 'st-up', false],
                pausedDL: ['st_paused', 'st-paused', true], stoppedDL: ['st_paused', 'st-paused', true],
                pausedUP: ['st_done', 'st-done', true], stoppedUP: ['st_done', 'st-done', true],
                queuedDL: ['st_queued', 'st-queued', false], queuedUP: ['st_queued', 'st-queued', false],
                checkingDL: ['st_checking', 'st-check', false], checkingUP: ['st_checking', 'st-check', false],
                checkingResumeData: ['st_checking', 'st-check', false], moving: ['st_moving', 'st-check', false],
                error: ['st_error', 'st-error', false], missingFiles: ['st_missing', 'st-error', false],
            };
            const m = map[state] || [null, 'st-queued', false];
            return { label: m[0] ? this.t(m[0]) : state, cls: m[1], paused: m[2] };
        },
        pct(p) { return Math.round((p || 0) * 100); },
        fmtSpeed(b) {
            if (!b) return '—';
            if (b >= 1e6) return (b / 1e6).toFixed(1) + ' Mo/s';
            return (b / 1e3).toFixed(0) + ' Ko/s';
        },
        // --- Prowlarr ---
        async loadProwlarr() {
            if (!this.services.prowlarr) { this.indexers = []; return; }
            this.prowlarrLoading = true; this.prowlarrError = '';
            try {
                const r = await fetch('/api/prowlarr/indexers');
                if (!r.ok) { this.prowlarrError = (await r.text()) || this.t('prowlarr_unreachable'); this.indexers = []; }
                else { const d = await r.json(); this.indexers = d.indexers || []; this.prowlarrApps = d.apps || []; this.radarrConfigured = !!d.radarrConfigured; }
            } catch (e) { this.prowlarrError = String(e); this.indexers = []; }
            finally { this.prowlarrLoading = false; }
        },
        isAppConnected(name) { return this.prowlarrApps.some((a) => a.name === name || a.implementation === name); },
        async connectApp(app) {
            this.connecting = true;
            try {
                const r = await fetch('/api/prowlarr/connect', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ app }) });
                if (!r.ok) throw new Error((await r.text()) || 'erreur');
                const d = await r.json();
                alert(d.created ? this.t('alert_app_connected', { app }) : this.t('alert_app_already', { app }));
                this.connectOpen = false;
                this.loadProwlarr();
            } catch (e) { alert(this.t('alert_failed', { msg: e.message })); }
            finally { this.connecting = false; }
        },
        async syncApps() {
            this.connecting = true;
            try {
                const r = await fetch('/api/prowlarr/sync', { method: 'POST' });
                if (!r.ok) throw new Error((await r.text()) || 'erreur');
                alert(this.t('alert_sync_started'));
            } catch (e) { alert(this.t('alert_failed', { msg: e.message })); }
            finally { this.connecting = false; }
        },
        async openAddIndexer() {
            this.schemaOpen = true; this.schemaQuery = '';
            if (!this.schemaList) await this.loadSchema();
        },
        async loadSchema() {
            try {
                const r = await fetch('/api/prowlarr/schema');
                this.schemaList = (await r.json()).indexers || [];
            } catch (e) { this.schemaList = []; }
        },
        async addIndexer(name) {
            this.addingIndexer = name;
            try {
                const r = await fetch('/api/prowlarr/add', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }) });
                if (!r.ok) throw new Error((await r.text()) || 'erreur');
                alert(this.t('alert_indexer_added', { name }));
                this.schemaOpen = false;
                this.loadProwlarr();
            } catch (e) { alert(this.t('alert_indexer_add_failed', { name, msg: e.message })); }
            finally { this.addingIndexer = ''; }
        },
        async prowlarrToggle(ix) {
            const enable = !ix.enable;
            try {
                const r = await fetch('/api/prowlarr/toggle', {
                    method: 'POST', headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ id: ix.id, enable }),
                });
                if (r.ok) ix.enable = enable; else alert(this.t('alert_failed', { msg: await r.text() }));
            } catch (e) { alert(this.t('alert_failed', { msg: e.message })); }
        },
        async load(y, m, scroll = true) {
            if (!this.services.sonarr) { this.loading = false; return; }
            const oldKey = this.year * 12 + this.month;
            this.loading = true;
            const query = (y && m) ? `?year=${y}&month=${m}` : '';
            const r = await fetch('/api/calendar' + query);
            Object.assign(this, await r.json());
            const newKey = this.year * 12 + this.month;
            if (oldKey && newKey !== oldKey) this.slideDir = newKey > oldKey ? 'next' : 'prev';
            this.loading = false;
            if (!scroll) return;
            if (y && m) window.scrollTo({ top: 0, behavior: 'smooth' }); // navigation par mois → haut de page
            else if (this.autoScroll) this.scrollToToday();              // « Aujourd'hui » / ouverture
        },
        // Recharge le calendrier suite à un événement serveur, mais en regroupant
        // les rafales (grab + download + import d'un même DL) et SANS re-scroller,
        // pour ne pas faire sauter la vue pendant un téléchargement.
        scheduleCalendarReload() {
            clearTimeout(this._calReloadTimer);
            this._calReloadTimer = setTimeout(() => this.load(this.year, this.month, false), 700);
        },
        async toggle(ep) {
            const next = !this.isWatched(ep);
            this.watched = next
                ? [...this.watched, ep.episodeId]
                : this.watched.filter((id) => id !== ep.episodeId);
            try {
                await fetch('/api/watched', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ episode_id: ep.episodeId, watched: next }),
                });
            } catch (e) {
                this.watched = next
                    ? this.watched.filter((id) => id !== ep.episodeId)
                    : [...this.watched, ep.episodeId];
            }
        },
        async play(ep) {
            // Le navigateur ne peut pas lancer MPC-BE : on passe par le helper
            // client.exe (localhost:8788) qui ouvre MPC-BE sur l'URL HTTP du
            // fichier servie par CE serveur. location.host = l'adresse tapée par
            // l'utilisateur → rien de codé en dur, marche partout.
            const name = ep.fileName || `${ep.series}.mkv`;
            const fileUrl = `${location.protocol}//${location.host}/play/${ep.episodeId}/${encodeURIComponent(name)}`;
            try {
                const r = await fetch(`http://127.0.0.1:8788/play?url=${encodeURIComponent(fileUrl)}`);
                if (!r.ok) throw new Error('helper');
            } catch (e) {
                alert(this.t('alert_no_player'));
            }
        },
        // --- Films (Radarr) ---
        async loadFilms() {
            if (!this.services.radarr) { this.films = []; return; }
            this.filmsLoading = true; this.filmsError = '';
            try {
                const r = await fetch('/api/films');
                if (!r.ok) { this.filmsError = (await r.text()) || this.t('radarr_unreachable'); this.films = []; }
                else { const d = await r.json(); this.films = d.movies || []; this.radarrUrl = d.radarrUrl || ''; }
            } catch (e) { this.filmsError = String(e); this.films = []; }
            finally { this.filmsLoading = false; }
        },
        filmStatusLabel(m) { return m.status === 'available' ? this.t('status_available') : (m.status === 'missing' ? this.t('status_missing') : this.t('status_unmonitored')); },
        openFilm(m) { this.selectedFilm = m; this.filmReleases = null; this.filmSearching = false; this.filmGrabbed = {}; },
        closeFilm() { this.selectedFilm = null; },
        async playFilm(m) {
            const name = (m.title || 'film') + '.mkv';
            const fileUrl = location.protocol + '//' + location.host + '/play/movie/' + m.id + '/' + encodeURIComponent(name);
            try {
                const r = await fetch('http://127.0.0.1:8788/play?url=' + encodeURIComponent(fileUrl));
                if (!r.ok) throw new Error('helper');
            } catch (e) {
                alert(this.t('alert_no_player'));
            }
        },
        async filmSearch() {
            if (!this.selectedFilm) return;
            this.filmSearching = true; this.filmReleases = null;
            try {
                const r = await fetch('/api/films/search?movieId=' + this.selectedFilm.id);
                if (!r.ok) throw new Error('search');
                this.filmReleases = (await r.json()).releases || [];
            } catch (e) { this.filmReleases = []; alert(this.t('alert_search_failed_radarr')); }
            finally { this.filmSearching = false; }
        },
        async filmGrab(rel) {
            const st = this.filmGrabbed[rel.guid];
            if (st === 'wait' || st === 'ok') return;
            this.filmGrabbed[rel.guid] = 'wait';
            try {
                const r = await fetch('/api/films/grab', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ guid: rel.guid, indexerId: rel.indexerId }) });
                if (!r.ok) throw new Error('grab');
                this.filmGrabbed[rel.guid] = 'ok';
            } catch (e) { this.filmGrabbed[rel.guid] = 'err'; }
        },
        connectWS() {
            const proto = location.protocol === 'https:' ? 'wss' : 'ws';
            const ws = new WebSocket(`${proto}://${location.host}/ws`);
            ws.onmessage = (e) => {
                let m;
                try { m = JSON.parse(e.data); } catch (_) { return; }
                if (m.type === 'progress') {
                    const map = {};
                    (m.items || []).forEach((it) => {
                        map[it.episodeId] = { status: it.status, percent: it.percent, timeleft: it.timeleft };
                    });
                    this.liveProgress = true;
                    this.progress = map;
                } else if (m.type === 'calendar') {
                    this.scheduleCalendarReload();
                }
            };
            ws.onclose = () => { setTimeout(() => this.connectWS(), 3000); };
            this.ws = ws;
        },
    },
    watch: {
        view() { this.$nextTick(() => this.updateUnderline()); },
        lang() { this.$nextTick(() => this.updateUnderline()); },
    },
    async mounted() {
        document.documentElement.lang = this.lang;
        await this.loadStatus();
        this.load();
        this.loadDisk();
        this.connectWS();
        setInterval(() => { this.now = Date.now(); }, 30000);
        window.addEventListener('scroll', () => this.onScroll(), { passive: true });
        window.addEventListener('resize', () => this.updateUnderline());
        document.addEventListener('click', (e) => this.onDocClick(e));
        this.$nextTick(() => this.updateUnderline());
    },
    template: `
    <header class="topbar">
        <div class="brand-group">
            <img src="/logo.png" class="brand-logo" alt="Calendarr" @click="goHome" :title="t('nav_calendar')">
            <nav class="tabs">
                <a v-for="tab in tabs" :key="tab.id" class="tab" :class="{ active: view === tab.id }"
                   href="#" @click.prevent="goTab(tab.id)">{{ t(tab.key) }}</a>
                <span class="tab-underline" :style="{ transform: 'translateX(' + underline.left + 'px)', width: underline.width + 'px' }"></span>
            </nav>
        </div>
        <div class="search-wrap topbar-search">
            <label class="search">
                <icon name="search"></icon>
                <input v-model="seriesQuery" @input="onSeriesInput" @keyup.enter="searchSeries"
                       type="text" :placeholder="t('search_placeholder')">
            </label>
            <div class="search-results" v-if="seriesQuery && (seriesSearching || seriesResults !== null)">
                <div v-if="seriesSearching" class="search-loading">{{ t('search_loading') }}</div>
                <template v-else>
                    <div v-if="seriesResults.length === 0" class="search-empty">{{ t('search_none') }}</div>
                    <button v-for="(r, i) in seriesResults" :key="i" class="search-item" @click="openAdd(r)">
                        <img v-if="r.poster" :src="r.poster" class="search-poster" alt="">
                        <span v-else class="search-poster ph"></span>
                        <span class="search-info">
                            <span class="search-title">{{ r.title }}<span v-if="r.year" class="search-year"> ({{ r.year }})</span></span>
                            <span class="search-sub">
                                <span class="type-badge" :class="r.type">{{ r.type === 'movie' ? t('type_movie') : t('type_series') }}</span>
                                <span v-if="r.sub">{{ r.sub }}</span>
                            </span>
                        </span>
                    </button>
                </template>
            </div>
        </div>
        <div class="top-actions">
            <div class="disk" v-if="disk && disk.total" :title="t('disk_free_on', { path: disk.path })">
                <div class="disk-top">
                    <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M10 16h.01"></path><path d="M2.212 11.577a2 2 0 0 0-.212.896V18a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-5.527a2 2 0 0 0-.212-.896L18.55 5.11A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"></path><path d="M21.946 12.013H2.054"></path><path d="M6 16h.01"></path></svg>
                    <div class="disk-text"><span class="disk-free">{{ fmtGB(disk.free) }}</span> <span class="disk-sep">/</span> {{ fmtGB(disk.total) }}</div>
                </div>
                <div class="disk-bar"><div class="disk-fill" :class="diskClass" :style="{ width: diskUsedPct + '%' }"></div></div>
            </div>
            <div class="top-sep"></div>
            <div class="opts-wrap">
                <button class="icon-btn" @click="optsOpen = !optsOpen" :title="t('opts_title')"><icon name="settings"></icon></button>
                <template v-if="optsOpen">
                    <div class="opts-pop">
                        <label class="opts-row">
                            <span>{{ t('opt_autoscroll') }}</span>
                            <input type="checkbox" class="opts-toggle" v-model="autoScroll" @change="saveAutoScroll">
                        </label>
                        <label class="opts-row">
                            <span>{{ t('opt_language') }}</span>
                            <select class="opts-lang" :value="lang" @change="setLang($event.target.value)">
                                <option v-for="l in langs" :key="l.code" :value="l.code">{{ l.name }}</option>
                            </select>
                        </label>
                        <template v-if="share">
                            <div class="opts-sep"></div>
                            <h3>{{ t('opt_access_title') }}</h3>
                            <p>{{ t('opt_access_desc') }}</p>
                            <code class="share-url">{{ share }}</code>
                        </template>
                    </div>
                </template>
            </div>
        </div>
    </header>

    <div v-if="seriesQuery && (seriesResults !== null || seriesSearching)" class="search-backdrop" @click="seriesQuery=''; seriesResults=null"></div>

    <main class="content">
        <template v-if="view === 'calendar'">
        <div v-if="!services.sonarr" class="page-empty">
            <icon name="schedule"></icon>
            <p><b>{{ t('sonarr_not_installed') }}</b><br>{{ t('sonarr_not_installed_desc') }}</p>
        </div>
        <template v-else>
        <div class="page-head cal-head">
            <div class="stats">
                <span class="stat"><b>{{ stats.episodes }}</b> {{ t('stat_episodes') }}</span>
                <span class="stat stat-ok"><b>{{ stats.downloaded }}</b> {{ t('stat_available') }}</span>
                <span class="stat stat-seen"><b>{{ stats.watched }}</b> {{ t('stat_watched') }}</span>
            </div>
            <div class="ph-title">
                <h1 class="month">{{ monthName }}</h1>
                <span class="ph-year" v-if="year">{{ year }}</span>
            </div>
            <div class="month-nav cal-nav" :class="{ stuck: navStuck }">
                <button @click="load(prev.year, prev.month)" :title="t('cal_prev')"><icon name="chevron_left"></icon></button>
                <button @click="load()">{{ t('cal_today') }}</button>
                <button @click="load(next.year, next.month)" :title="t('cal_next')"><icon name="chevron_right"></icon></button>
            </div>
        </div>

        <div class="calendar">
            <div v-if="loading && !weeks.length" class="loading">{{ t('loading') }}</div>
            <div v-else class="grid" :key="year + '-' + month" :class="slideDir && 'slide-' + slideDir">
                <div class="cell" v-for="cell in weeks" :key="cell.iso"
                     :class="{ out: !cell.inMonth, today: cell.isToday }">
                    <div class="daynum-row"><span class="daynum">{{ cell.day }}</span><span class="dayname">{{ cell.weekday }}</span></div>
                    <div class="cards">
                        <article v-for="ep in epsFor(cell)" :key="ep.id"
                                 class="card" :class="{ watched: isWatched(ep), premiere: ep.episode === 1 && ep.season >= 1, finale: ep.finaleType }">
                            <div class="banner" @click="openModal(ep)">
                                <img v-if="ep.banner || ep.poster" :src="ep.banner || ep.poster" loading="lazy" alt="">
                                <div class="banner-overlay"></div>
                                <div v-if="dl(ep)" class="action dl" :class="{ paused: dl(ep).status === 'paused', importing: dl(ep).status === 'importing', pending: dl(ep).status === 'pending' }" @click.stop="noop"
                                     :title="dl(ep).status === 'importing' ? t('st_moving') : (dl(ep).status === 'pending' ? t('st_pending') : (dl(ep).status === 'paused' ? t('st_paused') : t('downloading_title')))">
                                    <span v-if="dl(ep).status === 'importing'" class="dl-spin"></span>
                                    <icon v-else-if="dl(ep).status === 'pending'" name="schedule"></icon>
                                    <icon v-else-if="dl(ep).status === 'paused'" name="pause"></icon>
                                    <span v-else class="dl-icon"><span class="downloading"><span class="custom-arrow"></span></span><span class="bar"></span></span>
                                </div>
                                <button v-else-if="ep.hasFile" class="action play"
                                        @click.stop="play(ep)" :title="t('play_mpc')">
                                    <span class="play-tri"></span>
                                </button>
                            </div>
                            <div class="card-body">
                                <input type="checkbox" class="watch"
                                       :checked="isWatched(ep)" @change="toggle(ep)" :title="t('mark_watched')">
                                <div class="card-info">
                                    <button class="card-title" @click="openModal(ep)">{{ ep.series }}</button>
                                    <div class="card-meta">
                                        <span class="se">S{{ pad(ep.season) }}E{{ pad(ep.episode) }}</span>
                                        <span class="timer"><icon :name="dl(ep) ? (dl(ep).status === 'importing' ? 'sync' : (dl(ep).status === 'pending' ? 'schedule' : (dl(ep).status === 'paused' ? 'pause' : 'download'))) : (ep.hasFile ? 'check' : 'schedule')" :class="{ spin: dl(ep) && dl(ep).status === 'importing' }"></icon>{{ relTime(cell, ep) }}</span>
                                    </div>
                                    <div class="progress" v-if="dl(ep)">
                                        <div class="progress-top">
                                            <span class="progress-pct">{{ dl(ep).percent || 0 }}%<template v-if="dl(ep).timeleft"> · {{ dl(ep).timeleft }}</template></span>
                                        </div>
                                        <div class="progress-track"><div class="progress-fill" :style="{ width: (dl(ep).percent || 0) + '%' }"></div></div>
                                    </div>
                                </div>
                            </div>
                        </article>
                    </div>
                </div>
            </div>
        </div>
        </template>
        </template>

        <section v-else-if="view === 'films'" class="page">
            <div class="page-head">
                <div>
                    <h1 class="month">{{ t('nav_films') }}</h1>
                    <p class="subtitle">{{ t('films_count', { n: films.length }) }}</p>
                </div>
                <div class="month-nav">
                    <button @click="loadFilms"><icon name="refresh"></icon> {{ t('refresh') }}</button>
                </div>
            </div>
            <div v-if="!services.radarr" class="page-empty"><icon name="movie"></icon><p><b>{{ t('radarr_not_installed') }}</b><br>{{ t('radarr_not_installed_desc') }}</p></div>
            <div v-else-if="filmsLoading" class="loading">{{ t('loading') }}</div>
            <div v-else-if="filmsError" class="page-empty"><icon name="movie"></icon><p>{{ filmsError }}</p></div>
            <div v-else-if="films.length === 0" class="page-empty"><icon name="movie"></icon><p>{{ t('films_empty') }}</p></div>
            <div v-else class="film-grid">
                <article v-for="m in films" :key="m.id" class="film-card" @click="openFilm(m)">
                    <div class="film-poster">
                        <img v-if="m.poster" :src="m.poster" loading="lazy" alt="">
                        <span v-else class="film-ph"><icon name="movie"></icon></span>
                        <span class="film-badge" :class="m.status">{{ filmStatusLabel(m) }}</span>
                        <button v-if="m.hasFile" class="film-play" @click.stop="playFilm(m)" :title="t('play_mpc')"><icon name="play"></icon></button>
                    </div>
                    <div class="film-meta">
                        <div class="film-title" :title="m.title">{{ m.title }}</div>
                        <div class="film-year">{{ m.year }}</div>
                    </div>
                </article>
            </div>
        </section>

        <section v-else-if="view === 'torrents'" class="page">
            <div class="page-head">
                <div>
                    <h1 class="month">{{ t('nav_torrents') }}</h1>
                    <p class="subtitle">{{ t('torrents_count', { n: torrents.length, a: torrentsActive }) }}</p>
                </div>
                <div class="month-nav">
                    <button @click="loadTorrents"><icon name="refresh"></icon> {{ t('refresh') }}</button>
                    <template v-if="qbitStatus && qbitStatus.connected">
                    <button @click="torrentAction('pause','all')"><icon name="pause"></icon> {{ t('torrents_pause_all') }}</button>
                    <button @click="torrentAction('resume','all')"><icon name="play"></icon> {{ t('torrents_resume_all') }}</button>
                    <button class="nav-danger" v-if="torrents.length" @click="askDelete('all', t('torrents_all_label', { n: torrents.length }))"><icon name="trash"></icon> {{ t('torrents_delete_all') }}</button>
                    </template>
                </div>
            </div>
            <div v-if="torrentsLoading" class="loading">{{ t('loading') }}</div>
            <template v-else-if="qbitStatus && qbitStatus.connected">
            <div v-if="torrents.length === 0" class="page-empty"><icon name="download"></icon><p>{{ t('torrents_empty') }}</p></div>
            <div v-else class="table-wrap">
                <table class="data-table">
                    <thead><tr>
                        <th>{{ t('th_name') }}</th><th>{{ t('th_state') }}</th><th>{{ t('th_progress') }}</th><th>{{ t('th_size') }}</th><th>↓ DOWN</th><th>↑ UP</th><th>{{ t('th_ratio') }}</th><th>{{ t('th_seeds') }}</th><th></th>
                    </tr></thead>
                    <tbody>
                        <tr v-for="tr in torrents" :key="tr.hash">
                            <td class="d-name" :title="tr.name">{{ tr.name }}</td>
                            <td><span class="state" :class="tInfo(tr.state).cls">{{ tInfo(tr.state).label }}</span></td>
                            <td class="d-prog">
                                <div class="progress-track"><div class="progress-fill" :style="{ width: pct(tr.progress) + '%' }"></div></div>
                                <span class="d-pct">{{ pct(tr.progress) }}%</span>
                            </td>
                            <td class="t-nowrap">{{ fmtSize(tr.size) }}</td>
                            <td class="t-nowrap">{{ fmtSpeed(tr.dlspeed) }}</td>
                            <td class="t-nowrap">{{ fmtSpeed(tr.upspeed) }}</td>
                            <td>{{ (tr.ratio || 0).toFixed(2) }}</td>
                            <td>{{ tr.num_seeds }}</td>
                            <td class="d-actions">
                                <button @click="torrentAction(tInfo(tr.state).paused ? 'resume' : 'pause', tr.hash)" :title="tInfo(tr.state).paused ? t('resume') : t('pause')">
                                    <icon :name="tInfo(tr.state).paused ? 'play' : 'pause'"></icon>
                                </button>
                                <button class="danger" @click="askDelete(tr.hash, tr.name)" :title="t('delete')">
                                    <icon name="trash"></icon>
                                </button>
                            </td>
                        </tr>
                    </tbody>
                </table>
            </div>
            </template>
            <div v-else-if="qbitStatus && qbitStatus.reachable" class="page-empty">
                <icon name="download"></icon>
                <p><b>{{ t('qbit_password_title') }}</b><br>{{ t('qbit_password_desc') }}</p>
                <form class="qbit-form" @submit.prevent="connectQbit">
                    <input type="text" class="qbit-input qbit-user" v-model="qbitUserInput" autocomplete="username">
                    <input type="password" class="qbit-input" v-model="qbitPass" :placeholder="t('qbit_password_ph')" autocomplete="current-password">
                    <button class="btn btn-play" type="submit" :disabled="qbitConnecting || !qbitPass">{{ qbitConnecting ? t('loading') : t('connect') }}</button>
                </form>
                <p v-if="qbitMsg" class="qbit-msg">{{ qbitMsg }}</p>
            </div>
            <div v-else-if="qbitStatus && qbitStatus.installed" class="page-empty"><icon name="download"></icon><p><b>{{ t('qbit_webui_title') }}</b><br>{{ t('qbit_webui_desc') }}</p></div>
            <div v-else class="page-empty"><icon name="download"></icon><p><b>{{ t('qbit_error') }}</b><br>{{ t('qbit_error_desc') }}</p></div>
        </section>

        <section v-else-if="view === 'prowlarr'" class="page">
            <div class="page-head">
                <div>
                    <h1 class="month">Prowlarr</h1>
                    <p class="subtitle">{{ t('prowlarr_count', { n: indexers.length }) }}</p>
                </div>
                <div class="month-nav">
                    <template v-if="services.prowlarr && !prowlarrError">
                        <button class="conn-badge" :class="{ on: isAppConnected('Sonarr') }" :disabled="connecting || isAppConnected('Sonarr')" @click="connectApp('sonarr')"
                                :title="isAppConnected('Sonarr') ? t('synced_in_sonarr') : t('app_connect', { app: 'Sonarr' })">
                            <icon :name="isAppConnected('Sonarr') ? 'check' : 'bolt'"></icon> {{ isAppConnected('Sonarr') ? t('app_connected', { app: 'Sonarr' }) : t('app_connect', { app: 'Sonarr' }) }}
                        </button>
                        <button class="conn-badge" v-if="radarrConfigured" :class="{ on: isAppConnected('Radarr') }" :disabled="connecting || isAppConnected('Radarr')" @click="connectApp('radarr')"
                                :title="isAppConnected('Radarr') ? t('synced_in_radarr') : t('app_connect', { app: 'Radarr' })">
                            <icon :name="isAppConnected('Radarr') ? 'check' : 'bolt'"></icon> {{ isAppConnected('Radarr') ? t('app_connected', { app: 'Radarr' }) : t('app_connect', { app: 'Radarr' }) }}
                        </button>
                        <button @click="openAddIndexer"><icon name="plus"></icon> {{ t('prowlarr_add_indexer') }}</button>
                    </template>
                </div>
            </div>
            <div v-if="!services.prowlarr" class="page-empty"><icon name="bolt"></icon><p><b>{{ t('prowlarr_not_installed') }}</b><br>{{ t('prowlarr_not_installed_desc') }}</p></div>
            <div v-else-if="prowlarrLoading" class="loading">{{ t('loading') }}</div>
            <div v-else-if="prowlarrError" class="page-empty"><icon name="bolt"></icon><p>{{ prowlarrError }}</p></div>
            <div v-else-if="indexers.length === 0" class="page-empty"><p>{{ t('prowlarr_empty') }}</p></div>
            <div v-else class="table-wrap">
                <table class="data-table">
                    <thead><tr><th>{{ t('th_indexer') }}</th><th>{{ t('th_protocol') }}</th><th>{{ t('th_privacy') }}</th><th>{{ t('th_sync') }}</th><th>{{ t('th_priority') }}</th><th>{{ t('th_enabled') }}</th></tr></thead>
                    <tbody>
                        <tr v-for="ix in indexers" :key="ix.id" :class="{ disabled: !ix.enable }">
                            <td class="d-name">{{ ix.name }}</td>
                            <td>{{ ix.protocol }}</td>
                            <td><span class="pick-priv" :class="ix.privacy">{{ ix.privacy }}</span></td>
                            <td class="d-apps">
                                <span class="app-badge s" :class="{ on: ix.inSonarr }" :title="t('synced_in_sonarr')">S</span>
                                <span class="app-badge r" :class="{ on: ix.inRadarr }" v-if="radarrConfigured" :title="t('synced_in_radarr')">R</span>
                            </td>
                            <td>{{ ix.priority }}</td>
                            <td>
                                <button class="toggle" :class="{ on: ix.enable }" @click="prowlarrToggle(ix)" :title="ix.enable ? t('disable') : t('enable')">
                                    <span class="toggle-knob"></span>
                                </button>
                            </td>
                        </tr>
                    </tbody>
                </table>
            </div>
        </section>
    </main>

    <div class="modal-backdrop" v-if="selected" @click.self="closeModal">
        <div class="modal">
            <div class="modal-banner">
                <img v-if="selected.banner || selected.poster" :src="selected.banner || selected.poster" alt="">
                <div class="modal-grad"></div>
                <button class="modal-close" @click="closeModal" :title="t('close')"><icon name="close"></icon></button>
                <div class="modal-head">
                    <h2>{{ selected.series }}</h2>
                    <div class="modal-badges">
                        <span class="badge se">S{{ pad(selected.season) }}E{{ pad(selected.episode) }}</span>
                        <span class="badge" :class="'status-' + statusKey(selected)">{{ statusLabel(selected) }}</span>
                        <span class="modal-eptitle" v-if="selected.episodeTitle">{{ selected.episodeTitle }}</span>
                    </div>
                </div>
            </div>
            <div class="modal-body">
                <div class="modal-meta">
                    <span v-if="selected.network" class="meta-item">{{ selected.network }}</span>
                    <span v-if="selected.year" class="meta-item">{{ selected.year }}</span>
                    <span v-if="selected.runtime" class="meta-item">{{ selected.runtime }} {{ t('min') }}</span>
                    <span v-if="selected.time" class="meta-item"><icon name="schedule"></icon> {{ selected.time }}</span>
                    <span v-if="selected.rating" class="meta-item meta-rating"><icon name="star"></icon> {{ selected.rating.toFixed(1) }}</span>
                    <span v-if="selected.certification" class="meta-item meta-cert">{{ selected.certification }}</span>
                </div>
                <div class="modal-genres" v-if="selected.genres && selected.genres.length">
                    <span class="genre" v-for="g in selected.genres.slice(0, 4)" :key="g">{{ g }}</span>
                </div>
                <p class="modal-overview" v-if="selected.overview">{{ selected.overview }}</p>
                <div class="modal-actions">
                    <button v-if="selected.hasFile" class="btn btn-play" @click="play(selected)">
                        <icon name="play"></icon> {{ t('play_mpc') }}
                    </button>
                    <button class="btn btn-watch" :class="{ on: isWatched(selected) }" @click="toggle(selected)">
                        <icon name="check"></icon> {{ isWatched(selected) ? t('watched') : t('mark_watched') }}
                    </button>
                    <a v-if="sonarrUrl && selected.seriesSlug" class="btn btn-sonarr"
                       :href="sonarrUrl + '/series/' + selected.seriesSlug" target="_blank" rel="noopener"
                       :title="t('open_in_sonarr')">
                        <icon name="external"></icon> Sonarr
                    </a>
                </div>

                <div class="torrent-search">
                    <button class="btn btn-search" @click="searchTorrents" :disabled="searching">
                        <icon name="search"></icon> {{ searching ? t('searching_torrents') : t('search_torrents') }}
                    </button>
                    <div class="torrent-results" v-if="releases !== null">
                        <p v-if="releases.length === 0" class="torrent-empty">{{ t('torrents_search_empty') }}</p>
                        <table v-else class="torrent-table">
                            <thead><tr>
                                <th>{{ t('th_title') }}</th><th>{{ t('th_indexer') }}</th><th>{{ t('th_quality') }}</th><th>{{ t('th_size') }}</th><th>{{ t('th_seed') }}</th><th>{{ t('th_age') }}</th><th></th>
                            </tr></thead>
                            <tbody>
                                <tr v-for="r in releases" :key="r.guid" :class="{ rejected: r.rejected }">
                                    <td class="t-title" :title="(r.rejections && r.rejections.length) ? r.rejections.join(' · ') : r.title"><a v-if="r.infoUrl" :href="r.infoUrl" target="_blank" rel="noopener" class="t-link">{{ r.title }}</a><template v-else>{{ r.title }}</template></td>
                                    <td class="t-idx">{{ r.indexer }}</td>
                                    <td><span class="q-badge" :class="qBadge(r.quality)">{{ r.quality }}</span></td>
                                    <td class="t-nowrap">{{ fmtSize(r.size) }}</td>
                                    <td><span class="seed" :class="seedClass(r.seeders)">{{ r.seeders }}</span></td>
                                    <td class="t-nowrap">{{ r.age }}{{ t('u_d') }}</td>
                                    <td>
                                        <button class="grab-btn" :class="grabbed[r.guid]" @click="grab(r)"
                                                :disabled="grabbed[r.guid]==='wait' || grabbed[r.guid]==='ok'"
                                                :title="grabbed[r.guid]==='ok' ? t('grab_sent_sonarr') : (grabbed[r.guid]==='err' ? t('failed') : t('download_verb'))">
                                            <icon :name="grabbed[r.guid]==='ok' ? 'check' : (grabbed[r.guid]==='err' ? 'close' : 'download')"></icon>
                                        </button>
                                    </td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <div class="modal-backdrop" v-if="addTarget" @click.self="addTarget=null">
        <div class="modal add-modal">
            <div class="modal-banner add-banner">
                <img v-if="addTarget.poster" :src="addTarget.poster" alt="">
                <div class="modal-grad"></div>
                <button class="modal-close" @click="addTarget=null"><icon name="close"></icon></button>
                <div class="modal-head">
                    <h2>{{ addTarget.title }}<span v-if="addTarget.year" class="add-year"> ({{ addTarget.year }})</span></h2>
                </div>
            </div>
            <div class="modal-body" v-if="(addType==='series' && seriesOptions) || (addType==='movie' && movieOptions)">
                <template v-if="addType === 'series'">
                    <div class="form-grid">
                        <label class="field"><span>{{ t('label_monitor') }}</span>
                            <select v-model="addForm.monitor">
                                <option v-for="m in seriesOptions.monitorOptions" :key="m" :value="m">{{ m }}</option>
                            </select>
                        </label>
                        <label class="field"><span>{{ t('label_quality_profile') }}</span>
                            <select v-model="addForm.qualityProfileId">
                                <option v-for="p in seriesOptions.qualityProfiles" :key="p.id" :value="p.id">{{ p.name }}</option>
                            </select>
                        </label>
                        <label class="field"><span>{{ t('label_series_type') }}</span>
                            <select v-model="addForm.seriesType">
                                <option v-for="ty in seriesOptions.seriesTypes" :key="ty" :value="ty">{{ ty }}</option>
                            </select>
                        </label>
                        <label class="field" v-if="seriesOptions.rootFolders.length > 1"><span>{{ t('label_root_folder') }}</span>
                            <select v-model="addForm.rootFolderPath">
                                <option v-for="rf in seriesOptions.rootFolders" :key="rf.id" :value="rf.path">{{ rf.path }}</option>
                            </select>
                        </label>
                    </div>
                    <div class="field"><span>{{ t('label_tags') }}</span>
                        <div class="tag-list" v-if="seriesOptions.tags.length">
                            <button v-for="tg in seriesOptions.tags" :key="tg.id" class="tag-chip" :class="{ on: addForm.tags.includes(tg.id) }" @click="toggleAddTag(tg.id)">{{ tg.label }}</button>
                        </div>
                        <p v-else class="muted">{{ t('no_tags_sonarr') }}</p>
                    </div>
                    <label class="check-row"><input type="checkbox" v-model="addForm.searchNow"><span>{{ t('search_episodes_now') }}</span></label>
                </template>

                <template v-else>
                    <div class="form-grid">
                        <label class="field"><span>{{ t('label_quality_profile') }}</span>
                            <select v-model="addForm.qualityProfileId">
                                <option v-for="p in movieOptions.qualityProfiles" :key="p.id" :value="p.id">{{ p.name }}</option>
                            </select>
                        </label>
                        <label class="field"><span>{{ t('label_min_availability') }}</span>
                            <select v-model="addForm.minimumAvailability">
                                <option v-for="a in movieOptions.availabilityOptions" :key="a" :value="a">{{ a }}</option>
                            </select>
                        </label>
                        <label class="field" v-if="movieOptions.rootFolders.length > 1"><span>{{ t('label_root_folder') }}</span>
                            <select v-model="addForm.rootFolderPath">
                                <option v-for="rf in movieOptions.rootFolders" :key="rf.id" :value="rf.path">{{ rf.path }}</option>
                            </select>
                        </label>
                    </div>
                    <div class="field"><span>{{ t('label_tags') }}</span>
                        <div class="tag-list" v-if="movieOptions.tags.length">
                            <button v-for="tg in movieOptions.tags" :key="tg.id" class="tag-chip" :class="{ on: addForm.tags.includes(tg.id) }" @click="toggleAddTag(tg.id)">{{ tg.label }}</button>
                        </div>
                        <p v-else class="muted">{{ t('no_tags_radarr') }}</p>
                    </div>
                    <label class="check-row"><input type="checkbox" v-model="addForm.monitored"><span>{{ t('monitor_movie') }}</span></label>
                    <label class="check-row"><input type="checkbox" v-model="addForm.searchNow"><span>{{ t('search_now') }}</span></label>
                </template>

                <div class="modal-actions">
                    <button class="btn btn-play" @click="addItem" :disabled="adding">
                        <icon name="check"></icon> {{ adding ? t('adding') : (addType==='movie' ? t('add_movie') : t('add_series')) }}
                    </button>
                    <button class="btn btn-watch" @click="addTarget=null">{{ t('cancel') }}</button>
                </div>
            </div>
        </div>
    </div>

    <div class="modal-backdrop" v-if="selectedFilm" @click.self="closeFilm">
        <div class="modal">
            <div class="modal-banner">
                <img v-if="selectedFilm.banner || selectedFilm.poster" :src="selectedFilm.banner || selectedFilm.poster" alt="">
                <div class="modal-grad"></div>
                <button class="modal-close" @click="closeFilm"><icon name="close"></icon></button>
                <div class="modal-head">
                    <h2>{{ selectedFilm.title }}<span v-if="selectedFilm.year" class="add-year"> ({{ selectedFilm.year }})</span></h2>
                    <div class="modal-badges">
                        <span class="badge" :class="'status-' + (selectedFilm.status==='available' ? 'available' : (selectedFilm.status==='missing' ? 'downloading' : 'upcoming'))">{{ filmStatusLabel(selectedFilm) }}</span>
                    </div>
                </div>
            </div>
            <div class="modal-body">
                <p v-if="selectedFilm.overview" class="film-overview">{{ selectedFilm.overview }}</p>
                <div class="modal-actions">
                    <button v-if="selectedFilm.hasFile" class="btn btn-play" @click="playFilm(selectedFilm)">
                        <icon name="play"></icon> {{ t('play_mpc') }}
                    </button>
                    <a v-if="radarrUrl && selectedFilm.slug" class="btn btn-sonarr" :href="radarrUrl + '/movie/' + selectedFilm.slug" target="_blank" rel="noopener" :title="t('open_in_radarr')">
                        <icon name="external"></icon> Radarr
                    </a>
                </div>
                <div class="torrent-search">
                    <button class="btn btn-search" @click="filmSearch" :disabled="filmSearching">
                        <icon name="search"></icon> {{ filmSearching ? t('searching_torrents') : t('search_torrents') }}
                    </button>
                    <div class="torrent-results" v-if="filmReleases !== null">
                        <p v-if="filmReleases.length === 0" class="torrent-empty">{{ t('torrents_search_empty') }}</p>
                        <table v-else class="torrent-table">
                            <thead><tr>
                                <th>{{ t('th_title') }}</th><th>{{ t('th_indexer') }}</th><th>{{ t('th_quality') }}</th><th>{{ t('th_size') }}</th><th>{{ t('th_seed') }}</th><th>{{ t('th_age') }}</th><th></th>
                            </tr></thead>
                            <tbody>
                                <tr v-for="r in filmReleases" :key="r.guid" :class="{ rejected: r.rejected }">
                                    <td class="t-title" :title="(r.rejections && r.rejections.length) ? r.rejections.join(' · ') : r.title"><a v-if="r.infoUrl" :href="r.infoUrl" target="_blank" rel="noopener" class="t-link">{{ r.title }}</a><template v-else>{{ r.title }}</template></td>
                                    <td class="t-idx">{{ r.indexer }}</td>
                                    <td><span class="q-badge" :class="qBadge(r.quality)">{{ r.quality }}</span></td>
                                    <td class="t-nowrap">{{ fmtSize(r.size) }}</td>
                                    <td><span class="seed" :class="seedClass(r.seeders)">{{ r.seeders }}</span></td>
                                    <td class="t-nowrap">{{ r.age }}{{ t('u_d') }}</td>
                                    <td>
                                        <button class="grab-btn" :class="filmGrabbed[r.guid]" @click="filmGrab(r)"
                                                :disabled="filmGrabbed[r.guid]==='wait' || filmGrabbed[r.guid]==='ok'"
                                                :title="filmGrabbed[r.guid]==='ok' ? t('grab_sent_radarr') : (filmGrabbed[r.guid]==='err' ? t('failed') : t('download_verb'))">
                                            <icon :name="filmGrabbed[r.guid]==='ok' ? 'check' : (filmGrabbed[r.guid]==='err' ? 'close' : 'download')"></icon>
                                        </button>
                                    </td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <div class="modal-backdrop" v-if="schemaOpen" @click.self="schemaOpen=false">
        <div class="modal pick-modal">
            <div class="pick-head">
                <h2>{{ t('prowlarr_add_indexer') }}</h2>
                <button class="modal-close" @click="schemaOpen=false"><icon name="close"></icon></button>
            </div>
            <label class="pick-search">
                <icon name="search"></icon>
                <input v-model="schemaQuery" type="text" :placeholder="t('filter_catalog')">
            </label>
            <div class="pick-list">
                <div v-if="schemaList === null" class="loading">{{ t('loading_catalog') }}</div>
                <template v-else>
                    <p v-if="filteredSchema.length === 0" class="muted" style="padding:14px;text-align:center">{{ t('prowlarr_empty') }}</p>
                    <button v-for="ix in filteredSchema" :key="ix.name" class="pick-row" :disabled="addingIndexer === ix.name" @click="addIndexer(ix.name)">
                        <span class="pick-name">{{ ix.name }}</span>
                        <span class="pick-tags">
                            <span class="type-badge" :class="ix.protocol === 'usenet' ? 'series' : 'movie'">{{ ix.protocol }}</span>
                            <span class="pick-priv" :class="ix.privacy">{{ ix.privacy }}</span>
                        </span>
                    </button>
                </template>
            </div>
        </div>
    </div>

    <div class="modal-backdrop" v-if="deleteTarget" @click.self="deleteTarget=null">
        <div class="modal confirm-modal">
            <div class="confirm-body">
                <h2>{{ t('delete_confirm_title', { label: deleteTarget.label }) }}</h2>
                <label class="check-row">
                    <input type="checkbox" v-model="deleteFiles">
                    <span>{{ t('delete_with_files') }}</span>
                </label>
                <p class="muted" v-if="deleteFiles">{{ t('delete_files_warning') }}</p>
                <div class="modal-actions">
                    <button class="btn btn-danger" @click="confirmDelete"><icon name="trash"></icon> {{ t('delete') }}</button>
                    <button class="btn btn-watch" @click="deleteTarget=null">{{ t('cancel') }}</button>
                </div>
            </div>
        </div>
    </div>
    `,
});

app.component('icon', {
    props: { name: { type: String, required: true } },
    computed: { d() { return ICONS[this.name] || ''; } },
    template: `<svg class="icon" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path :d="d"></path></svg>`,
});

app.mount('#app');
