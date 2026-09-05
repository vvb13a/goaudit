package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/vvb13a/goaudit/domain"
)

// HtmlService renders domain audits as self-contained, interactive HTML reports.
type HtmlService struct {
	dir string
}

func NewHtmlService(dir string) *HtmlService {
	return &HtmlService{dir: dir}
}

// AuditPath returns the storage path of the HTML report for the given audit ID.
func (s *HtmlService) AuditPath(auditID string) string {
	return filepath.Join(s.dir, auditID+".html")
}

// Export renders the audit into an HTML document and returns the bytes.
func (s *HtmlService) Export(a *domain.Audit) ([]byte, error) {
	return s.build(a)
}

// ExportTo renders the audit into an HTML file at the given path.
func (s *HtmlService) ExportTo(a *domain.Audit, path string) error {
	data, err := s.build(a)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create report directory: %w", err)
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// ExportAudit renders the audit into the service directory and returns the path.
func (s *HtmlService) ExportAudit(a *domain.Audit) (string, error) {
	if a == nil {
		return "", domain.ErrInvalidAudit
	}
	if a.ID == "" {
		return "", fmt.Errorf("export audit: audit id is empty")
	}

	path := s.AuditPath(a.ID)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check existing report: %w", err)
	}

	if err := s.ExportTo(a, path); err != nil {
		return "", err
	}
	return path, nil
}

// RemoveAuditFile deletes the HTML report stored for the given audit ID.
func (s *HtmlService) RemoveAuditFile(auditID string) error {
	if auditID == "" {
		return nil
	}
	if err := os.Remove(s.AuditPath(auditID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove report: %w", err)
	}
	return nil
}

func (s *HtmlService) build(a *domain.Audit) ([]byte, error) {
	if a == nil {
		return nil, domain.ErrInvalidAudit
	}

	raw, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("encode audit data: %w", err)
	}

	var buf bytes.Buffer
	if err := auditHTMLTmpl.Execute(&buf, struct {
		AuditJSON template.JS
	}{AuditJSON: jsEscapeJSON(raw)}); err != nil {
		return nil, fmt.Errorf("render audit report: %w", err)
	}
	return buf.Bytes(), nil
}

func jsEscapeJSON(raw []byte) template.JS {
	esc := strings.NewReplacer(
		"&", `\u0026`,
		"<", `\u003c`,
		">", `\u003e`,
		"\u2028", `\u2028`,
		"\u2029", `\u2029`,
	)
	return template.JS(esc.Replace(string(raw)))
}

var auditHTMLTmpl = template.Must(template.New("audit-report").Parse(htmlReportSource))

const htmlReportSource = `<!doctype html>
<html lang="en" class="h-full bg-slate-950 text-slate-100">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GoAudit Report</title>
<script src="https://cdn.tailwindcss.com"></script>
<script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600&display=swap" rel="stylesheet">

<script>
tailwind.config = {
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Plus Jakarta Sans"', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'monospace'],
      }
    }
  }
}
</script>

<style>
  [x-cloak] { display: none !important; }
  @media print {
    .no-print { display: none !important; }
    body { background: white !important; color: black !important; }
    .print-break { page-break-after: always; }
  }
</style>

<script id="audit-data" type="application/json">{{.AuditJSON}}</script>

<script>
var SEVS = [
  {key: 'fatal',   label: 'Fatal',   w: 50, dot: 'bg-rose-500',   badge: 'bg-rose-500/10 text-rose-400 border-rose-500/20' },
  {key: 'error',   label: 'Error',   w: 40, dot: 'bg-red-500',    badge: 'bg-red-500/10 text-red-400 border-red-500/20' },
  {key: 'warning', label: 'Warning', w: 30, dot: 'bg-amber-400',  badge: 'bg-amber-400/10 text-amber-300 border-amber-400/20' },
  {key: 'notice',  label: 'Notice',  w: 20, dot: 'bg-sky-400',    badge: 'bg-sky-400/10 text-sky-300 border-sky-400/20' },
  {key: 'info',    label: 'Info',    w: 10, dot: 'bg-indigo-400', badge: 'bg-indigo-400/10 text-indigo-300 border-indigo-400/20' },
  {key: 'success', label: 'Passed',  w: 0,  dot: 'bg-emerald-400',badge: 'bg-emerald-400/10 text-emerald-300 border-emerald-400/20' }
];

var SEVMAP = (function () {
  var m = {};
  SEVS.forEach(function (s) { m[s.key] = s; });
  return m;
})();

// sevPass reports whether an issue of the given severity is a pass:
// failures are severity warning and worse.
function sevPass(key) {
  var sev = SEVMAP[key] || SEVS[4];
  return sev.w < 30;
}

function clip(s, n) {
  s = s || '';
  return s.length > n ? s.slice(0, n - 1) + '…' : s;
}

function fmtDuration(ns) {
  var ms = Math.round(ns / 1e6);
  if (ms < 1000) { return ms + ' ms'; }
  var s = ms / 1000;
  if (s < 60) { return s.toFixed(2) + ' s'; }
  var m = Math.floor(s / 60);
  s = Math.floor(s % 60);
  return m + 'm ' + s + 's';
}

function fmtTS(iso) {
  if (!iso) { return '—'; }
  var d = new Date(iso);
  return isNaN(d.getTime()) ? iso : d.toLocaleString();
}

function page() {
  var data = {name: 'Unknown', urls: []};
  var el = document.getElementById('audit-data');
  if (el) {
    try { data = JSON.parse(el.textContent); } catch (e) { console.error('Failed to parse audit data', e); }
  }

  return {
    audit: data,
    activeTab: 'overview', // 'overview' | 'issues' | 'endpoints'
    search: '',
    selectedCategory: 'all',
    selectedReportIdx: -1,
    groupByCheck: true,
    sevOn: {fatal: true, error: true, warning: true, notice: true, info: false, success: false},
    sort: 'severity',
    expanded: {},
    copiedKey: null,

    urls: function () { return this.audit.urls || []; },

    allIssues: function () {
      var out = [];
      this.urls().forEach(function (r, ri) {
        (r.issues || []).forEach(function (iss, ii) {
          var copy = Object.assign({}, iss);
          copy.key = ri + ':' + ii;
          copy.reportIdx = ri;
          copy.url = r.url;
          copy.finalUrl = r.final_url || r.url;
          copy.httpStatus = r.status_code;
          copy.passed = sevPass(iss.severity);
          out.push(copy);
        });
      });
      return out;
    },

    // Health Score calculation (0-100%)
    score: function () {
      var total = 0, passed = 0;
      this.allIssues().forEach(function (i) {
        if (i.severity === 'info') return; // info doesn't affect score
        total++;
        if (i.passed) passed++;
      });
      return total === 0 ? 100 : Math.round((passed / total) * 100);
    },

    categories: function () {
      var cats = {};
      this.allIssues().forEach(function (i) {
        var c = i.category || 'general';
        if (!cats[c]) cats[c] = { total: 0, passed: 0, failed: 0 };
        cats[c].total++;
        if (i.passed) cats[c].passed++;
        else cats[c].failed++;
      });
      return cats;
    },

    categoryScore: function (cat) {
      var c = this.categories()[cat];
      if (!c || c.total === 0) return 100;
      return Math.round((c.passed / c.total) * 100);
    },

    avgDurationMs: function () {
      var reps = this.urls();
      if (!reps.length) return 0;
      var sum = reps.reduce(function (acc, r) { return acc + (r.duration || 0); }, 0);
      return Math.round((sum / reps.length) / 1e6);
    },

    failedEndpointsCount: function () {
      return this.urls().filter(function (r) {
        return (r.issues || []).some(function (i) { return !sevPass(i.severity); });
      }).length;
    },

    // Filtered Issues list
    filteredIssues: function () {
      var self = this;
      var q = (this.search || '').trim().toLowerCase();

      return this.allIssues().filter(function (i) {
        if (self.selectedReportIdx >= 0 && i.reportIdx !== self.selectedReportIdx) return false;
        if (self.selectedCategory !== 'all' && i.category !== self.selectedCategory) return false;
        if (!self.sevOn[i.severity]) return false;

        if (q) {
          var hay = (i.message + ' ' + i.check_name + ' ' + (i.category || '') + ' ' + i.url).toLowerCase();
          if (hay.indexOf(q) === -1) return false;
        }
        return true;
      });
    },

    // Grouped issues by check_name for cleaner presentation
    groupedIssues: function () {
      var groups = {};
      this.filteredIssues().forEach(function (i) {
        if (!groups[i.check_name]) {
          groups[i.check_name] = {
            check_name: i.check_name,
            category: i.category,
            severity: i.severity,
            passed: i.passed,
            message: i.message,
            items: []
          };
        }
        groups[i.check_name].items.push(i);
      });
      var arr = Object.values(groups);
      arr.sort(function (a, b) { return SEVMAP[b.severity].w - SEVMAP[a.severity].w; });
      return arr;
    },

    // Top failing checks for the Overview dashboard
    topFailingChecks: function () {
      var counts = {};
      this.allIssues().forEach(function (i) {
        if (!i.passed) {
          if (!counts[i.check_name]) {
            counts[i.check_name] = { check_name: i.check_name, severity: i.severity, count: 0, category: i.category };
          }
          counts[i.check_name].count++;
        }
      });
      return Object.values(counts).sort(function (a, b) { return b.count - a.count; }).slice(0, 5);
    },

    severityCounts: function () {
      var counts = {fatal: 0, error: 0, warning: 0, notice: 0, info: 0, success: 0};
      this.allIssues().forEach(function (i) {
        if (counts[i.severity] !== undefined) counts[i.severity]++;
      });
      return counts;
    },

    toggleExpand: function (k) { this.expanded[k] = !this.expanded[k]; },
    expandAll: function () {
      var self = this;
      this.filteredIssues().forEach(function (i) { if (i.details) self.expanded[i.key] = true; });
    },
    collapseAll: function () { this.expanded = {}; },

    copyEvidence: function (key, data) {
      var self = this;
      navigator.clipboard.writeText(JSON.stringify(data, null, 2)).then(function () {
        self.copiedKey = key;
        setTimeout(function () { self.copiedKey = null; }, 2000);
      });
    },

    sevMeta: function (k) { return SEVMAP[k] || SEVS[4]; }
  };
}
</script>
</head>
<body class="bg-slate-950 font-sans antialiased selection:bg-indigo-500 selection:text-white" x-data="page()" x-cloak>

<div class="min-h-full flex flex-col">
  <!-- Top Navigation Bar -->
  <nav class="border-b border-slate-800 bg-slate-900/60 backdrop-blur-md sticky top-0 z-40">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex items-center justify-between h-16">
        <div class="flex items-center gap-3">
          <div class="h-8 w-8 rounded-lg bg-gradient-to-tr from-indigo-500 to-sky-400 flex items-center justify-center font-bold text-white shadow-lg shadow-indigo-500/20">
            GA
          </div>
          <div>
            <div class="flex items-center gap-2">
              <span class="font-bold text-white tracking-tight" x-text="audit.name || 'GoAudit Report'"></span>
              <span class="text-[10px] font-mono uppercase bg-indigo-500/20 text-indigo-300 border border-indigo-500/30 px-1.5 py-0.5 rounded font-semibold" x-text="(audit.check_names || []).length + ' checks'"></span>
            </div>
            <span class="text-xs text-slate-400 font-mono" x-text="'ID: ' + audit.id"></span>
          </div>
        </div>

        <!-- Navigation Tabs -->
        <div class="flex items-center gap-1 bg-slate-950/60 p-1 rounded-xl border border-slate-800 text-xs font-semibold">
          <button @click="activeTab = 'overview'" :class="activeTab === 'overview' ? 'bg-indigo-600 text-white shadow' : 'text-slate-400 hover:text-white'" class="px-3.5 py-1.5 rounded-lg transition">Overview</button>
          <button @click="activeTab = 'issues'" :class="activeTab === 'issues' ? 'bg-indigo-600 text-white shadow' : 'text-slate-400 hover:text-white'" class="px-3.5 py-1.5 rounded-lg transition flex items-center gap-1.5">
            <span>Issues</span>
            <span class="bg-rose-500/20 text-rose-300 rounded-full px-1.5 py-0.2 text-[10px]" x-text="allIssues().filter(i => !i.passed).length"></span>
          </button>
          <button @click="activeTab = 'endpoints'" :class="activeTab === 'endpoints' ? 'bg-indigo-600 text-white shadow' : 'text-slate-400 hover:text-white'" class="px-3.5 py-1.5 rounded-lg transition flex items-center gap-1.5">
            <span>URLs</span>
            <span class="bg-slate-800 text-slate-300 rounded-full px-1.5 py-0.2 text-[10px]" x-text="urls().length"></span>
          </button>
        </div>

        <div class="flex items-center gap-3 no-print">
          <button onclick="window.print()" class="text-xs font-semibold px-3 py-1.5 rounded-lg border border-slate-700 bg-slate-800 hover:bg-slate-700 text-slate-200 transition">
            Export / Print
          </button>
        </div>
      </div>
    </div>
  </nav>

  <main class="max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 flex-1 space-y-8">
    
    <!-- ================================================================= -->
    <!-- TAB 1: OVERVIEW DASHBOARD -->
    <!-- ================================================================= -->
    <div x-show="activeTab === 'overview'" class="space-y-6">
      <!-- High-Level Metric Tiles -->
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <!-- Health Score Dial -->
        <div class="bg-slate-900/80 border border-slate-800/80 rounded-2xl p-5 flex items-center justify-between">
          <div>
            <div class="text-xs uppercase font-bold tracking-wider text-slate-400">Health Score</div>
            <div class="text-3xl font-extrabold mt-1 text-white" x-text="score() + '%'"></div>
            <div class="text-xs text-slate-400 mt-1 font-medium" x-text="score() >= 90 ? 'Excellent condition' : 'Needs attention'"></div>
          </div>
          <div class="relative h-14 w-14 flex items-center justify-center">
            <svg class="h-full w-full -rotate-90" viewBox="0 0 36 36">
              <path class="text-slate-800" stroke-width="3.5" stroke="currentColor" fill="none" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
              <path :class="score() >= 80 ? 'text-emerald-500' : (score() >= 60 ? 'text-amber-500' : 'text-rose-500')" 
                    :stroke-dasharray="score() + ', 100'" stroke-width="3.5" stroke-linecap="round" stroke="currentColor" fill="none" 
                    d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
            </svg>
          </div>
        </div>

        <!-- Endpoints Scanned -->
        <div class="bg-slate-900/80 border border-slate-800/80 rounded-2xl p-5">
          <div class="text-xs uppercase font-bold tracking-wider text-slate-400">Endpoints Audited</div>
          <div class="text-3xl font-extrabold mt-1 text-white" x-text="urls().length"></div>
          <div class="text-xs text-rose-400 mt-1 font-semibold" x-text="failedEndpointsCount() + ' URLs have issues'"></div>
        </div>

        <!-- Avg Latency -->
        <div class="bg-slate-900/80 border border-slate-800/80 rounded-2xl p-5">
          <div class="text-xs uppercase font-bold tracking-wider text-slate-400">Avg Server Latency</div>
          <div class="text-3xl font-extrabold mt-1 text-white" x-text="avgDurationMs() + ' ms'"></div>
          <div class="text-xs text-slate-400 mt-1" x-text="'Total run: ' + fmtDuration(audit.duration)"></div>
        </div>

        <!-- Total Issues -->
        <div class="bg-slate-900/80 border border-slate-800/80 rounded-2xl p-5">
          <div class="text-xs uppercase font-bold tracking-wider text-slate-400">Total Failures</div>
          <div class="text-3xl font-extrabold mt-1 text-rose-400" x-text="allIssues().filter(i => !i.passed).length"></div>
          <div class="text-xs text-slate-400 mt-1" x-text="severityCounts().warning + ' warnings, ' + severityCounts().error + ' errors'"></div>
        </div>
      </div>

      <!-- Category Breakdown Grid -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div class="bg-slate-900/80 border border-slate-800/80 rounded-2xl p-6 space-y-4">
          <h2 class="text-base font-bold text-white flex items-center justify-between">
            <span>Category Health</span>
            <span class="text-xs text-slate-400 font-normal">Pass Rate (%)</span>
          </h2>
          <div class="space-y-4">
            <template x-for="(stats, cat) in categories()" :key="cat">
              <div class="space-y-1.5">
                <div class="flex justify-between text-xs font-semibold">
                  <span class="capitalize text-slate-300" x-text="cat"></span>
                  <span :class="categoryScore(cat) >= 90 ? 'text-emerald-400' : 'text-amber-400'" x-text="categoryScore(cat) + '%'"></span>
                </div>
                <div class="h-2 w-full bg-slate-800 rounded-full overflow-hidden">
                  <div class="h-full rounded-full transition-all duration-500" 
                       :class="categoryScore(cat) >= 90 ? 'bg-emerald-500' : (categoryScore(cat) >= 60 ? 'bg-amber-500' : 'bg-rose-500')" 
                       :style="'width: ' + categoryScore(cat) + '%'"></div>
                </div>
              </div>
            </template>
          </div>
        </div>

        <!-- Most Common Failing Checks -->
        <div class="bg-slate-900/80 border border-slate-800/80 rounded-2xl p-6 space-y-4">
          <h2 class="text-base font-bold text-white">Top Action Items</h2>
          <div class="space-y-3">
            <template x-for="item in topFailingChecks()" :key="item.check_name">
              <div class="flex items-center justify-between p-3 rounded-xl bg-slate-950/60 border border-slate-800/60 text-xs">
                <div class="flex items-center gap-2.5">
                  <span class="h-2 w-2 rounded-full" :class="sevMeta(item.severity).dot"></span>
                  <span class="font-mono font-medium text-slate-200" x-text="item.check_name"></span>
                  <span class="text-[10px] uppercase font-semibold text-slate-500" x-text="item.category"></span>
                </div>
                <div class="font-semibold text-rose-400 bg-rose-500/10 border border-rose-500/20 px-2 py-0.5 rounded-full" x-text="item.count + ' URLs'"></div>
              </div>
            </template>
            <template x-if="topFailingChecks().length === 0">
              <div class="text-center py-6 text-sm text-slate-500">🎉 No failing checks detected!</div>
            </template>
          </div>
        </div>
      </div>
    </div>

    <!-- ================================================================= -->
    <!-- TAB 2: ISSUES EXPLORER -->
    <!-- ================================================================= -->
    <div x-show="activeTab === 'issues'" class="space-y-6">
      <!-- Search & Filters -->
      <div class="bg-slate-900/80 border border-slate-800/80 rounded-2xl p-4 space-y-4">
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative flex-1 min-w-[240px]">
            <input type="search" placeholder="Search checks, error messages, URLs..."
                   class="w-full bg-slate-950 border border-slate-800 rounded-xl px-4 py-2 text-sm text-slate-200 placeholder-slate-500 focus:outline-none focus:border-indigo-500 transition"
                   x-model.debounce.150ms="search">
          </div>

          <!-- Category filter -->
          <select class="bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-300 focus:outline-none focus:border-indigo-500"
                  x-model="selectedCategory">
            <option value="all">All Categories</option>
            <template x-for="(stats, cat) in categories()" :key="cat">
              <option :value="cat" x-text="cat.toUpperCase()"></option>
            </template>
          </select>

          <!-- Grouping Toggle -->
          <button @click="groupByCheck = !groupByCheck" 
                  class="text-xs font-semibold px-3 py-2 rounded-xl border transition flex items-center gap-1.5"
                  :class="groupByCheck ? 'bg-indigo-600 text-white border-indigo-500' : 'bg-slate-950 border-slate-800 text-slate-400'">
            <span x-text="groupByCheck ? 'Grouped by Check' : 'Flat Stream'"></span>
          </button>
        </div>

        <!-- Severity Chips & Action Bar -->
        <div class="flex flex-wrap items-center justify-between gap-3 pt-2 border-t border-slate-800/60">
          <div class="flex flex-wrap items-center gap-2">
            <template x-for="s in SEVS" :key="s.key">
              <button @click="sevOn[s.key] = !sevOn[s.key]"
                      class="rounded-lg px-2.5 py-1 text-xs font-semibold inline-flex items-center gap-1.5 border transition"
                      :class="sevOn[s.key] ? s.badge : 'bg-slate-950 border-slate-800 text-slate-600'">
                <span class="h-1.5 w-1.5 rounded-full" :class="s.dot"></span>
                <span x-text="s.label + ' (' + severityCounts()[s.key] + ')'"></span>
              </button>
            </template>
          </div>

          <div class="flex items-center gap-2">
            <button @click="expandAll()" class="text-xs font-medium text-slate-400 hover:text-white px-2 py-1 rounded">Expand All</button>
            <span class="text-slate-700">|</span>
            <button @click="collapseAll()" class="text-xs font-medium text-slate-400 hover:text-white px-2 py-1 rounded">Collapse All</button>
          </div>
        </div>
      </div>

      <!-- GROUPED VIEW -->
      <template x-if="groupByCheck">
        <div class="space-y-4">
          <template x-for="grp in groupedIssues()" :key="grp.check_name">
            <div class="bg-slate-900/80 border border-slate-800/80 rounded-2xl overflow-hidden">
              <div class="p-4 flex items-center justify-between border-b border-slate-800/40 bg-slate-900/40 cursor-pointer"
                   @click="toggleExpand(grp.check_name)">
                <div class="flex items-center gap-3">
                  <span class="h-2.5 w-2.5 rounded-full" :class="sevMeta(grp.severity).dot"></span>
                  <div>
                    <div class="flex items-center gap-2">
                      <span class="font-mono text-sm font-semibold text-white" x-text="grp.check_name"></span>
                      <span class="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded border" :class="sevMeta(grp.severity).badge" x-text="grp.severity"></span>
                      <span class="text-[10px] font-medium uppercase text-slate-400" x-text="grp.category"></span>
                    </div>
                    <p class="text-xs text-slate-400 mt-0.5" x-text="grp.message"></p>
                  </div>
                </div>
                <div class="flex items-center gap-3">
                  <span class="text-xs font-semibold text-slate-400 font-mono" x-text="grp.items.length + ' URLs'"></span>
                  <span class="text-slate-500 text-xs" x-text="expanded[grp.check_name] ? '▲' : '▼'"></span>
                </div>
              </div>

              <!-- List of URLs for this Check -->
              <div x-show="expanded[grp.check_name]" class="divide-y divide-slate-800/40">
                <template x-for="item in grp.items" :key="item.key">
                  <div class="p-3 pl-8 bg-slate-950/40 hover:bg-slate-950/80 transition text-xs space-y-2">
                    <div class="flex items-center justify-between">
                      <a :href="item.url" target="_blank" class="font-mono text-indigo-400 hover:underline" x-text="item.url"></a>
                      <template x-if="item.details">
                        <button @click="copyEvidence(item.key, item.details)" class="text-[10px] text-slate-500 hover:text-slate-300 font-mono">
                          <span x-text="copiedKey === item.key ? 'Copied!' : 'Copy Evidence'"></span>
                        </button>
                      </template>
                    </div>
                    <template x-if="item.details">
                      <pre class="bg-slate-950 border border-slate-800/60 p-2.5 rounded-xl font-mono text-[11px] text-emerald-300 overflow-x-auto" x-text="JSON.stringify(item.details, null, 2)"></pre>
                    </template>
                  </div>
                </template>
              </div>
            </div>
          </template>
        </div>
      </template>

      <!-- FLAT VIEW -->
      <template x-if="!groupByCheck">
        <div class="bg-slate-900/80 border border-slate-800/80 rounded-2xl divide-y divide-slate-800/60 overflow-hidden">
          <template x-for="i in filteredIssues()" :key="i.key">
            <article class="p-4 hover:bg-slate-800/30 transition">
              <div class="flex items-start gap-3">
                <span class="mt-1 h-2.5 w-2.5 rounded-full shrink-0" :class="sevMeta(i.severity).dot"></span>
                <div class="min-w-0 flex-1 space-y-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="text-xs font-mono font-bold text-white" x-text="i.check_name"></span>
                    <span class="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded border" :class="sevMeta(i.severity).badge" x-text="i.severity"></span>
                    <span class="text-[10px] uppercase font-semibold text-slate-400" x-text="i.category"></span>
                  </div>
                  <p class="text-sm text-slate-300" x-text="i.message"></p>
                  <div class="flex items-center gap-4 text-xs font-mono text-slate-400 pt-1">
                    <a :href="i.url" target="_blank" class="text-indigo-400 hover:underline" x-text="clip(i.url, 75)"></a>
                    <template x-if="i.details">
                      <button @click="toggleExpand(i.key)" class="text-slate-400 hover:text-white font-semibold">
                        <span x-text="expanded[i.key] ? 'Hide Evidence' : 'Show Evidence'"></span>
                      </button>
                    </template>
                  </div>
                </div>
              </div>

              <template x-if="expanded[i.key] && i.details">
                <div class="mt-3 ml-5 rounded-xl border border-slate-800 bg-slate-950 p-3 relative">
                  <div class="flex justify-between items-center mb-1 text-[10px] uppercase font-mono tracking-wider text-slate-500">
                    <span>Evidence Payload</span>
                    <button @click="copyEvidence(i.key, i.details)" class="text-indigo-400 hover:underline" x-text="copiedKey === i.key ? 'Copied!' : 'Copy JSON'"></button>
                  </div>
                  <pre class="text-xs font-mono text-emerald-300 overflow-x-auto leading-relaxed" x-text="JSON.stringify(i.details, null, 2)"></pre>
                </div>
              </template>
            </article>
          </template>
        </div>
      </template>
    </div>

    <!-- ================================================================= -->
    <!-- TAB 3: ENDPOINTS / URL CRAWL TABLE -->
    <!-- ================================================================= -->
    <div x-show="activeTab === 'endpoints'" class="space-y-6">
      <div class="bg-slate-900/80 border border-slate-800/80 rounded-2xl overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-xs text-slate-300">
            <thead class="bg-slate-950/80 text-slate-400 uppercase font-mono tracking-wider border-b border-slate-800">
              <tr>
                <th class="p-3.5">Status</th>
                <th class="p-3.5">URL</th>
                <th class="p-3.5">Server Latency</th>
                <th class="p-3.5">Issues</th>
                <th class="p-3.5 text-right">Inspect</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800/60 font-mono">
              <template x-for="(r, idx) in urls()" :key="idx">
                <tr class="hover:bg-slate-800/30 transition">
                  <td class="p-3.5">
                    <span class="px-2 py-0.5 rounded-full font-bold text-[10px]"
                          :class="r.status_code >= 200 && r.status_code < 300 ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border border-rose-500/20'"
                          x-text="r.status_code"></span>
                  </td>
                  <td class="p-3.5 font-sans font-medium text-slate-200">
                    <div x-text="r.url"></div>
                    <div x-show="r.final_url && r.final_url !== r.url" class="text-[11px] text-slate-500 font-mono" x-text="'→ ' + r.final_url"></div>
                  </td>
                  <td class="p-3.5 text-slate-400" x-text="fmtDuration(r.duration)"></td>
                  <td class="p-3.5">
                    <span class="px-2 py-0.5 rounded font-semibold text-[10px]"
                          :class="(r.summary.failed_count || 0) > 0 ? 'bg-rose-500/20 text-rose-300' : 'bg-emerald-500/20 text-emerald-300'"
                          x-text="(r.summary.failed_count || 0) + ' failed'"></span>
                  </td>
                  <td class="p-3.5 text-right font-sans">
                    <button @click="selectedReportIdx = idx; activeTab = 'issues'" 
                            class="text-xs text-indigo-400 hover:text-indigo-300 font-semibold underline">
                      View Issues
                    </button>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </div>
    </div>

  </main>

  <!-- Footer -->
  <footer class="border-t border-slate-800/60 py-6 text-center text-xs text-slate-500">
    Audited by <span class="font-semibold text-slate-400">GoAudit Engine</span> &bull; Blazing Fast Static Reports
  </footer>
</div>
</body>
</html>
`
