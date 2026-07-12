// État global de l'app
const state = {
  achievements: [],     // tous les succès chargés
  selected: [],         // noms sélectionnés, dans l'ordre choisi (drag)
  axis: "percent",
  chartType: "line",     // "bar" | "line"
  swapped: true,        // axes X/Y inversés (succès en X, % en Y)
  chart: null,
  chartFs: null,        // graphique plein écran
  panOffset: 0,         // index du premier succès affiché (fenêtre glissante)
  panWindow: 0,         // nombre de succès affichés à la fois (0 = tout)
};

// ---------- Onglets ----------
document.querySelectorAll(".tab").forEach((btn) => {
  btn.addEventListener("click", () => {
    document.querySelectorAll(".tab").forEach((b) => b.classList.remove("active"));
    document.querySelectorAll(".tab-content").forEach((c) => c.classList.remove("active"));
    btn.classList.add("active");
    document.getElementById("tab-content-" + btn.dataset.tab).classList.add("active");
    if (btn.dataset.tab === "settings") loadKeyStatus();
  });
});

// ---------- Status helper ----------
const statusEl = document.getElementById("status");
function setStatus(msg, type = "info") {
  statusEl.textContent = msg;
  statusEl.className = "status show " + type;
}
function clearStatus() { statusEl.className = "status"; statusEl.textContent = ""; }

// ---------- Chargement des succès ----------
document.getElementById("btn-load").addEventListener("click", loadAchievements);

const searchInput = document.getElementById("search");
const suggestEl = document.getElementById("search-suggest");
let searchState = { results: [], activeIdx: -1, debounce: null };

searchInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter") {
    if (searchState.activeIdx >= 0 && searchState.results[searchState.activeIdx]) {
      pickSuggestion(searchState.results[searchState.activeIdx]);
    } else {
      loadAchievements();
    }
    return;
  }
  if (!suggestEl.hidden) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      searchState.activeIdx = Math.min(searchState.activeIdx + 1, searchState.results.length - 1);
      renderSuggest();
      return;
    }
    if (e.key === "ArrowUp") {
      e.preventDefault();
      searchState.activeIdx = Math.max(searchState.activeIdx - 1, -1);
      renderSuggest();
      return;
    }
    if (e.key === "Escape") {
      hideSuggest();
      return;
    }
  }
});

searchInput.addEventListener("input", () => {
  clearTimeout(searchState.debounce);
  const term = searchInput.value.trim();
  if (!term) { hideSuggest(); return; }
  searchState.debounce = setTimeout(() => runSearch(term), 250);
});

searchInput.addEventListener("blur", () => {
  // Laisse le temps au clic sur une suggestion de se déclencher.
  setTimeout(hideSuggest, 150);
});

async function runSearch(term) {
  const lang = document.getElementById("lang").value;
  try {
    const res = await fetch(`/stats/api/search?term=${encodeURIComponent(term)}&lang=${encodeURIComponent(lang)}`);
    const text = await res.text();
    let data;
    try {
      data = JSON.parse(text);
    } catch (e) {
      throw new Error(`Réponse non-JSON (HTTP ${res.status}). Avez-vous relancé le serveur ?`);
    }
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
    searchState.results = data.results || [];
    searchState.activeIdx = -1;
    renderSuggest();
  } catch (err) {
    console.error(err);
    setStatus(err.message, "error");
    hideSuggest();
  }
}

function renderSuggest() {
  if (searchState.results.length === 0) {
    suggestEl.innerHTML = `<li class="suggest-empty">Aucun résultat.</li>`;
    suggestEl.hidden = false;
    return;
  }
  suggestEl.innerHTML = "";
  searchState.results.forEach((r, i) => {
    const li = document.createElement("li");
    li.className = "suggest-item" + (i === searchState.activeIdx ? " active" : "");
    li.innerHTML = `
      <img src="${r.img || ""}" alt="" onerror="this.style.visibility='hidden'" />
      <span class="suggest-name">${escapeHtml(r.name)}</span>
      <span class="suggest-appid">#${r.appid}</span>
    `;
    li.addEventListener("mousedown", (e) => {
      e.preventDefault();
      pickSuggestion(r);
    });
    suggestEl.appendChild(li);
  });
  suggestEl.hidden = false;
}

function hideSuggest() {
  suggestEl.hidden = true;
  suggestEl.innerHTML = "";
  searchState.activeIdx = -1;
}

function pickSuggestion(r) {
  searchInput.value = `${r.name} (#${r.appid})`;
  searchInput.dataset.appid = String(r.appid);
  hideSuggest();
  loadAchievements();
}

async function loadAchievements() {
  let appid = searchInput.dataset.appid || "";
  const term = searchInput.value.trim();
  // Si l'utilisateur a tapé directement un AppID numérique.
  if (!appid && /^\d+$/.test(term)) {
    appid = term;
  }
  const lang = document.getElementById("lang").value;
  if (!appid) { setStatus("Recherchez ou saisissez un AppID.", "error"); return; }

  setStatus("Chargement des succès…", "info");
  const btn = document.getElementById("btn-load");
  btn.disabled = true;
  // Réinitialise l'appid mémorisé : la prochaine recherche repartira de zéro.
  delete searchInput.dataset.appid;

  try {
    const res = await fetch(`/stats/api/achievements?appid=${encodeURIComponent(appid)}&lang=${encodeURIComponent(lang)}`);
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);

    state.achievements = data.achievements || [];
    state.selected = [];
    renderList();
    renderOrderList();
    renderChart();
    const gameName = data.gameName ? ` — ${data.gameName}` : "";
    setStatus(`${state.achievements.length} succès chargés${gameName}.`, "ok");
  } catch (err) {
    setStatus(err.message, "error");
    state.achievements = [];
    state.selected = [];
    renderList();
    renderOrderList();
    renderChart();
  } finally {
    btn.disabled = false;
  }
}

// ---------- Liste des succès ----------
const listEl = document.getElementById("ach-list");
const filterEl = document.getElementById("filter");

filterEl.addEventListener("input", renderList);

document.getElementById("btn-select-all").addEventListener("click", () => {
  const q = filterEl.value.toLowerCase().trim();
  const filtered = state.achievements.filter((a) => {
    if (!q) return true;
    return (
      a.displayName.toLowerCase().includes(q) ||
      a.name.toLowerCase().includes(q) ||
      (a.description || "").toLowerCase().includes(q)
    );
  });
  for (const a of filtered) {
    if (!state.selected.includes(a.name)) state.selected.push(a.name);
  }
  renderList();
  renderOrderList();
  renderChart();
});

document.getElementById("btn-select-none").addEventListener("click", () => {
  state.selected = [];
  renderList();
  renderOrderList();
  renderChart();
});

function renderList() {
  const q = filterEl.value.toLowerCase().trim();
  listEl.innerHTML = "";

  const filtered = state.achievements.filter((a) => {
    if (!q) return true;
    return (
      a.displayName.toLowerCase().includes(q) ||
      a.name.toLowerCase().includes(q) ||
      (a.description || "").toLowerCase().includes(q)
    );
  });

  if (filtered.length === 0) {
    listEl.innerHTML = `<li class="hint">Aucun succès à afficher.</li>`;
    return;
  }

  // Tri : du plus abandonné au moins abandonné (par défaut)
  filtered.sort((a, b) => (b.abandon || 0) - (a.abandon || 0));

  for (const a of filtered) {
    const li = document.createElement("li");
    li.className = "ach-item";
    li.draggable = true;
    li.dataset.name = a.name;
    if (state.selected.includes(a.name)) li.classList.add("selected");

    const pct = a.percent ?? 0;
    const pctClass = pct < 25 ? "low" : pct < 60 ? "mid" : "high";

    li.innerHTML = `
      <img src="${a.iconGray || a.icon || ""}" alt="" onerror="this.style.visibility='hidden'" />
      <div class="ach-info">
        <div class="ach-name">${escapeHtml(a.displayName || a.name)}</div>
        <div class="ach-desc">${escapeHtml(a.description || "")}</div>
      </div>
      <div class="ach-pct ${pctClass}" title="Taux de complétion global">
        ${pct.toFixed(1)}%
      </div>
    `;

    // Drag
    li.addEventListener("dragstart", (e) => {
      li.classList.add("dragging");
      e.dataTransfer.setData("text/plain", a.name);
      e.dataTransfer.effectAllowed = "copy";
    });
    li.addEventListener("dragend", () => li.classList.remove("dragging"));

    // Double-clic = ajouter/retirer rapidement
    li.addEventListener("dblclick", () => toggleSelected(a.name));

    // Simple clic = sélectionner aussi (ergonomie)
    li.addEventListener("click", () => toggleSelected(a.name));

    listEl.appendChild(li);
  }
}

function toggleSelected(name) {
  const idx = state.selected.indexOf(name);
  if (idx >= 0) {
    state.selected.splice(idx, 1);
  } else {
    state.selected.push(name);
  }
  renderList();
  renderOrderList();
  renderChart();
}

// ---------- Dropzone + Graph ----------
const dropzone = document.getElementById("dropzone");
const canvas = document.getElementById("chart");
const chartWrap = document.getElementById("chart-wrap");
const axisSel = document.getElementById("axis");
const countEl = document.getElementById("selected-count");

axisSel.addEventListener("change", () => {
  state.axis = axisSel.value;
  renderChart();
  if (fullscreenEl) renderChartFullscreen();
});

const chartTypeSel = document.getElementById("chart-type");
chartTypeSel.addEventListener("change", () => {
  state.chartType = chartTypeSel.value;
  renderChart();
  if (fullscreenEl) renderChartFullscreen();
});

const swapBtn = document.getElementById("btn-swap-axes");
swapBtn.addEventListener("click", () => {
  state.swapped = !state.swapped;
  swapBtn.classList.toggle("active", state.swapped);
  state.panOffset = 0; // reset du pan quand on change d'orientation
  renderChart();
  if (fullscreenEl) renderChartFullscreen();
});

// Slider de translation horizontale
document.getElementById("pan-slider").addEventListener("input", (e) => {
  state.panOffset = parseInt(e.target.value, 10);
  renderChart();
});
document.getElementById("pan-first").addEventListener("click", () => {
  state.panOffset = 0;
  renderChart();
});
document.getElementById("pan-last").addEventListener("click", () => {
  const byName = new Map(state.achievements.map((a) => [a.name, a]));
  const total = state.selected.length;
  state.panOffset = Math.max(0, total - state.panWindow);
  renderChart();
});

const expandBtn = document.getElementById("btn-expand");
let fullscreenEl = null;

expandBtn.addEventListener("click", () => {
  if (fullscreenEl) {
    closeFullscreen();
  } else {
    openFullscreen();
  }
});

function openFullscreen() {
  if (state.selected.length === 0) {
    setStatus("Sélectionnez au moins un succès à grapher.", "error");
    return;
  }
  expandBtn.classList.add("active");

  fullscreenEl = document.createElement("div");
  fullscreenEl.className = "chart-fullscreen";
  fullscreenEl.innerHTML = `
    <div class="chart-fullscreen-header">
      <h2>Graphique — ${state.selected.length} succès</h2>
      <button id="btn-close-fullscreen" class="primary">✕ Fermer (Échap)</button>
    </div>
    <div class="chart-fullscreen-canvas">
      <canvas id="chart-fs"></canvas>
    </div>
  `;
  document.body.appendChild(fullscreenEl);

  fullscreenEl.querySelector("#btn-close-fullscreen").addEventListener("click", closeFullscreen);

  // Rend un second graphique dédié en plein écran.
  renderChartFullscreen();

  document.addEventListener("keydown", onEscFullscreen);
}

function closeFullscreen() {
  if (!fullscreenEl) return;
  fullscreenEl.remove();
  fullscreenEl = null;
  expandBtn.classList.remove("active");
  document.removeEventListener("keydown", onEscFullscreen);
  // Le graphique normal est toujours en arrière-plan, pas besoin de recréer.
}

function onEscFullscreen(e) {
  if (e.key === "Escape") closeFullscreen();
}

function renderChartFullscreen() {
  if (!fullscreenEl) return;
  const fsCanvas = fullscreenEl.querySelector("#chart-fs");
  if (!fsCanvas) return;

  const byName = new Map(state.achievements.map((a) => [a.name, a]));
  const selected = state.selected.map((n) => byName.get(n)).filter(Boolean);
  if (selected.length === 0) return;

  const labels = selected.map((a) => a.displayName || a.name);
  const values = selected.map((a) => a[state.axis] ?? 0);
  const isAbandon = state.axis === "abandon";
  const color = isAbandon ? "#ff6b6b" : "#4ade80";
  const colorBg = isAbandon ? "rgba(255,107,107,0.18)" : "rgba(74,222,128,0.18)";
  const isLine = state.chartType === "line";

  const dataset = {
    label: isAbandon ? "Taux d'abandon (%)" : "Taux de complétion (%)",
    data: values,
    backgroundColor: colorBg,
    borderColor: color,
    borderWidth: 2,
    borderRadius: 6,
    pointRadius: 3,
  };
  if (isLine) {
    dataset.fill = true;
    dataset.tension = 0.35;
    dataset.pointBackgroundColor = color;
  }

  const indexAxis = state.swapped ? "x" : "y";

  // Détruit l'ancien graphique plein écran si présent.
  if (state.chartFs) state.chartFs.destroy();
  state.chartFs = new Chart(fsCanvas, {
    type: state.chartType,
    data: { labels, datasets: [dataset] },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      indexAxis,
      scales: {
        x: { min: 0, max: 100, ticks: { color: "#9aa3c0" }, grid: { color: "#2a3150" } },
        y: { ticks: { color: "#9aa3c0" }, grid: { color: "#2a3150" } },
      },
      plugins: {
        legend: { labels: { color: "#e6e9f5" } },
        tooltip: {
          callbacks: {
            afterLabel: (ctx) => {
              const a = selected[ctx.dataIndex];
              return [
                `Complétion : ${a.percent.toFixed(1)}%`,
                `Abandon : ${a.abandon.toFixed(1)}%`,
                a.description ? `— ${a.description}` : "",
              ].filter(Boolean);
            },
          },
        },
      },
    },
  });
}

dropzone.addEventListener("dragover", (e) => {
  e.preventDefault();
  e.dataTransfer.dropEffect = "copy";
  dropzone.classList.add("drag-over");
});
dropzone.addEventListener("dragleave", () => dropzone.classList.remove("drag-over"));
dropzone.addEventListener("drop", (e) => {
  e.preventDefault();
  dropzone.classList.remove("drag-over");
  const name = e.dataTransfer.getData("text/plain");
  if (name && !state.selected.includes(name)) {
    state.selected.push(name);
    renderList();
    renderOrderList();
    renderChart();
  }
});

document.getElementById("btn-clear").addEventListener("click", () => {
  state.selected = [];
  renderList();
  renderOrderList();
  renderChart();
});

function renderChart() {
  // Construit la liste dans l'ordre choisi par l'utilisateur (state.selected).
  const byName = new Map(state.achievements.map((a) => [a.name, a]));
  const selected = state.selected.map((n) => byName.get(n)).filter(Boolean);
  countEl.textContent = `${selected.length} succès sélectionné(s)`;

  if (selected.length === 0) {
    chartWrap.hidden = true;
    document.querySelector(".dropzone-empty").style.display = "block";
    document.getElementById("dropzone").classList.remove("has-chart");
    document.getElementById("pan-bar").hidden = true;
    if (state.chart) { state.chart.destroy(); state.chart = null; }
    return;
  }

  chartWrap.hidden = false;
  document.querySelector(".dropzone-empty").style.display = "none";

  const dropzone = document.getElementById("dropzone");
  dropzone.classList.add("has-chart");

  // Fenêtre glissante : en mode vertical (succès en X) et si on a beaucoup
  // de succès, on n'en affiche qu'une partie contrôlée par le slider.
  const vertical = state.swapped;
  const maxVisible = vertical ? 60 : 0; // 0 = pas de limite (mode horizontal)
  const panBar = document.getElementById("pan-bar");
  const panSlider = document.getElementById("pan-slider");
  const panLabel = document.getElementById("pan-label");

  if (vertical && maxVisible > 0 && selected.length > maxVisible) {
    panBar.hidden = false;
    state.panWindow = maxVisible;
    // Ajuste l'offset si on a retiré des succès.
    const maxOffset = selected.length - state.panWindow;
    if (state.panOffset > maxOffset) state.panOffset = maxOffset;
    if (state.panOffset < 0) state.panOffset = 0;
    panSlider.min = 0;
    panSlider.max = maxOffset;
    panSlider.value = state.panOffset;
    panLabel.textContent = `${state.panOffset + 1}–${state.panOffset + state.panWindow} / ${selected.length}`;
  } else {
    panBar.hidden = true;
    state.panWindow = 0;
    state.panOffset = 0;
  }

  const visible = state.panWindow > 0
    ? selected.slice(state.panOffset, state.panOffset + state.panWindow)
    : selected;

  // On respecte l'ordre défini par l'utilisateur (drag & drop dans la liste d'ordre).

  const labels = visible.map((a) => a.displayName || a.name);
  const values = visible.map((a) => a[state.axis] ?? 0);
  const isAbandon = state.axis === "abandon";
  const color = isAbandon ? "#ff6b6b" : "#4ade80";
  const colorBg = isAbandon ? "rgba(255,107,107,0.18)" : "rgba(74,222,128,0.18)";

  const isLine = state.chartType === "line";
  const many = visible.length > 60;
  const dataset = {
    label: isAbandon ? "Taux d'abandon (%)" : "Taux de complétion (%)",
    data: values,
    backgroundColor: colorBg,
    borderColor: color,
    borderWidth: 2,
    borderRadius: many ? 2 : 6,
    pointRadius: many ? 0 : 3,
  };
  if (isLine) {
    dataset.fill = true;
    dataset.tension = 0.35;
    dataset.pointBackgroundColor = color;
  }

  const data = { labels, datasets: [dataset] };

  // indexAxis : 'y' = barres horizontales (succès en Y, % en X).
  // Quand inversé, on bascule en 'x' (succès en X, % en Y).
  const indexAxis = state.swapped ? "x" : "y";

  const opts = {
    responsive: true,
    maintainAspectRatio: false,
    indexAxis,
    scales: {
      x: { min: 0, max: 100, ticks: { color: "#9aa3c0", autoSkip: true, maxTicksLimit: 11 }, grid: { color: "#2a3150" } },
      y: { ticks: { color: "#9aa3c0", autoSkip: many, maxTicksLimit: vertical ? 11 : 80 }, grid: { color: "#2a3150" } },
    },
    plugins: {
      legend: { labels: { color: "#e6e9f5" } },
      tooltip: {
        callbacks: {
          afterLabel: (ctx) => {
            const a = visible[ctx.dataIndex];
            return [
              `Complétion : ${a.percent.toFixed(1)}%`,
              `Abandon : ${a.abandon.toFixed(1)}%`,
              a.description ? `— ${a.description}` : "",
            ].filter(Boolean);
          },
        },
      },
    },
    animation: many ? false : undefined,
  };

  // Chart.js ne permet pas de changer le type d'un graphique existant :
  // on détruit et recrée.
  if (state.chart) {
    state.chart.destroy();
  }
  state.chart = new Chart(canvas, { type: state.chartType, data, options: opts });
}

// ---------- Liste d'ordre (réordonnable) ----------
const orderListEl = document.getElementById("order-list");

document.getElementById("btn-sort-desc").addEventListener("click", () => {
  sortSelected((a, b) => (b[state.axis] || 0) - (a[state.axis] || 0));
});
document.getElementById("btn-sort-asc").addEventListener("click", () => {
  sortSelected((a, b) => (a[state.axis] || 0) - (b[state.axis] || 0));
});

function sortSelected(compare) {
  const byName = new Map(state.achievements.map((a) => [a.name, a]));
  state.selected.sort((n1, n2) => {
    const a = byName.get(n1), b = byName.get(n2);
    return compare(a, b);
  });
  renderOrderList();
  renderChart();
}

function renderOrderList() {
  orderListEl.innerHTML = "";
  if (state.selected.length === 0) {
    orderListEl.innerHTML = `<li class="order-empty">Aucun succès sélectionné.</li>`;
    return;
  }
  const byName = new Map(state.achievements.map((a) => [a.name, a]));
  state.selected.forEach((name, i) => {
    const a = byName.get(name);
    if (!a) return;
    const li = document.createElement("li");
    li.className = "order-item";
    li.draggable = true;
    li.dataset.name = name;

    li.innerHTML = `
      <span class="order-handle">⠿</span>
      <span class="order-index">${i + 1}</span>
      <span class="order-name">${escapeHtml(a.displayName || a.name)}</span>
      <span class="order-value">${(a[state.axis] ?? 0).toFixed(1)}%</span>
      <button class="order-remove" title="Retirer">✕</button>
    `;

    li.querySelector(".order-remove").addEventListener("click", (e) => {
      e.stopPropagation();
      toggleSelected(name);
    });

    li.addEventListener("dragstart", (e) => {
      li.classList.add("dragging");
      e.dataTransfer.setData("text/plain", name);
      e.dataTransfer.effectAllowed = "move";
    });
    li.addEventListener("dragend", () => {
      li.classList.remove("dragging");
      orderListEl.querySelectorAll(".order-item").forEach((el) => {
        el.classList.remove("drag-over-top", "drag-over-bottom");
      });
    });

    li.addEventListener("dragover", (e) => {
      e.preventDefault();
      e.dataTransfer.dropEffect = "move";
      const rect = li.getBoundingClientRect();
      const mid = rect.top + rect.height / 2;
      li.classList.remove("drag-over-top", "drag-over-bottom");
      if (e.clientY < mid) li.classList.add("drag-over-top");
      else li.classList.add("drag-over-bottom");
    });
    li.addEventListener("dragleave", () => {
      li.classList.remove("drag-over-top", "drag-over-bottom");
    });
    li.addEventListener("drop", (e) => {
      e.preventDefault();
      const draggedName = e.dataTransfer.getData("text/plain");
      if (!draggedName || draggedName === name) return;
      const fromIdx = state.selected.indexOf(draggedName);
      let toIdx = state.selected.indexOf(name);
      const rect = li.getBoundingClientRect();
      const mid = rect.top + rect.height / 2;
      const insertAfter = e.clientY >= mid;
      if (fromIdx < 0) return;
      state.selected.splice(fromIdx, 1);
      toIdx = state.selected.indexOf(name);
      if (insertAfter) toIdx += 1;
      state.selected.splice(toIdx, 0, draggedName);
      renderOrderList();
      renderChart();
    });

    orderListEl.appendChild(li);
  });
}

// ---------- Redimensionnement des panneaux ----------
const resizer = document.getElementById("resizer");
const layoutEl = document.getElementById("layout");
const panelLeft = document.getElementById("panel-left");
const panelRight = document.getElementById("panel-right");

let resizing = false;
resizer.addEventListener("mousedown", (e) => {
  e.preventDefault();
  resizing = true;
  resizer.classList.add("dragging");
  document.body.style.cursor = "col-resize";
  document.body.style.userSelect = "none";
});

document.addEventListener("mousemove", (e) => {
  if (!resizing) return;
  const rect = layoutEl.getBoundingClientRect();
  let leftWidth = e.clientX - rect.left;
  const totalWidth = rect.width;
  const min = 240;
  const max = totalWidth - 400;
  if (leftWidth < min) leftWidth = min;
  if (leftWidth > max) leftWidth = max;
  const rightWidth = totalWidth - leftWidth - 6;
  panelLeft.style.width = leftWidth + "px";
  panelRight.style.width = rightWidth + "px";
  if (state.chart) state.chart.resize();
});

document.addEventListener("mouseup", () => {
  if (resizing) {
    resizing = false;
    resizer.classList.remove("dragging");
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  }
});

// ---------- Exports ----------
function getSelectedData() {
  const byName = new Map(state.achievements.map((a) => [a.name, a]));
  return state.selected.map((n) => byName.get(n)).filter(Boolean);
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function exportPNG() {
  if (!state.chart) { setStatus("Aucun graphique à exporter.", "error"); return; }
  // Chart.js rend sur un canvas ; on récupère directement l'image.
  const url = state.chart.toBase64Image("image/png", 1);
  fetch(url).then((r) => r.blob()).then((blob) => downloadBlob(blob, "statsgraphsteam.png"));
}

function exportCSV() {
  const data = getSelectedData();
  if (data.length === 0) { setStatus("Aucune donnée à exporter.", "error"); return; }
  const header = ["#", "Nom interne", "Nom affiché", "Description", "Complétion (%)", "Abandon (%)", "Caché"];
  const rows = data.map((a, i) => [
    i + 1,
    a.name,
    a.displayName,
    a.description || "",
    a.percent,
    a.abandon,
    a.hidden ? "Oui" : "Non",
  ]);
  const all = [header, ...rows];
  // Échappement CSV : on entoure de guillemets et on double les guillemets internes.
  const csv = all.map((row) =>
    row.map((cell) => {
      const s = String(cell);
      if (s.includes(",") || s.includes('"') || s.includes("\n")) {
        return '"' + s.replace(/"/g, '""') + '"';
      }
      return s;
    }).join(",")
  ).join("\r\n");
  // BOM UTF-8 pour qu'Excel ouvre correctement les accents.
  downloadBlob(new Blob(["\uFEFF" + csv], { type: "text/csv;charset=utf-8" }), "statsgraphsteam.csv");
}

function exportPDF() {
  if (!state.chart) { setStatus("Aucun graphique à exporter.", "error"); return; }
  const { jsPDF } = window.jspdf;
  const imgData = state.chart.toBase64Image("image/png", 1);
  const canvas = state.chart.canvas;
  const ratio = canvas.height / canvas.width;
  const pdfW = 297; // A4 paysage en mm
  const pdfH = 210;
  const margin = 10;
  const imgW = pdfW - margin * 2;
  const imgH = imgW * ratio;
  const imgY = (pdfH - imgH) / 2;
  const pdf = new jsPDF({ orientation: "landscape", unit: "mm", format: "a4" });
  pdf.setFontSize(14);
  pdf.text("StatsGraphSteam", margin, margin + 2);
  pdf.addImage(imgData, "PNG", margin, imgY, imgW, Math.min(imgH, pdfH - margin * 2 - 10));
  pdf.save("statsgraphsteam.pdf");
}

document.getElementById("btn-export-png").addEventListener("click", exportPNG);
document.getElementById("btn-export-excel").addEventListener("click", exportCSV);
document.getElementById("btn-export-pdf").addEventListener("click", exportPDF);

// ---------- Paramètres API ----------
async function loadKeyStatus() {
  try {
    const res = await fetch("/stats/api/settings");
    const data = await res.json();
    const el = document.getElementById("key-status");
    if (data.has_key) {
      el.className = "key-status show ok";
      el.textContent = `Clé configurée (${data.masked}).`;
    } else {
      el.className = "key-status show warn";
      el.textContent = "Aucune clé configurée. Renseignez-la ci-dessous.";
    }
  } catch (e) {
    console.error(e);
  }
}

document.getElementById("btn-save-key").addEventListener("click", async () => {
  const key = document.getElementById("api-key").value.trim();
  if (!key) { loadKeyStatus(); return; }
  const btn = document.getElementById("btn-save-key");
  btn.disabled = true;
  try {
    const res = await fetch("/stats/api/settings", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ steam_api_key: key }),
    });
    if (!res.ok) throw new Error("Échec de l'enregistrement");
    document.getElementById("api-key").value = "";
    await loadKeyStatus();
  } catch (err) {
    alert(err.message);
  } finally {
    btn.disabled = false;
  }
});

// ---------- Utils ----------
function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

// Init
loadKeyStatus();
swapBtn.classList.add("active");
