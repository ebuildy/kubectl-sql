"use strict";

(function () {
  const editor = document.getElementById("editor");
  const highlight = document.querySelector("#highlight code");
  const suggestions = document.getElementById("suggestions");
  const results = document.getElementById("results");
  const runBtn = document.getElementById("run");

  // --- Syntax highlighting (textarea-over-<pre> overlay) -------------------

  const KEYWORDS = [
    "SELECT", "FROM", "WHERE", "ORDER", "BY", "GROUP", "HAVING", "LIMIT",
    "OFFSET", "AS", "AND", "OR", "NOT", "IN", "IS", "NULL", "LIKE", "BETWEEN",
    "DISTINCT", "COUNT", "ASC", "DESC", "ON", "USING", "JOIN", "INNER", "LEFT",
    "RIGHT", "FULL", "OUTER", "CROSS", "UNION", "ALL", "WITH", "SHOW", "TABLES",
    "DESCRIBE", "TABLE", "NAMESPACE",
  ];
  const keywordRe = new RegExp("\\b(" + KEYWORDS.join("|") + ")\\b", "gi");

  function escapeHtml(s) {
    return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }

  // Tokenize strings first (so keywords inside literals are not colored), then
  // arrows, then keywords. Each match is wrapped in a span class.
  function renderHighlight(text) {
    let out = "";
    const stringRe = /'(?:[^'\\]|\\.)*'?/g;
    let last = 0;
    let m;
    while ((m = stringRe.exec(text)) !== null) {
      out += highlightNonString(text.slice(last, m.index));
      out += '<span class="tok-string">' + escapeHtml(m[0]) + "</span>";
      last = m.index + m[0].length;
    }
    out += highlightNonString(text.slice(last));
    highlight.innerHTML = out;
  }

  function highlightNonString(text) {
    let s = escapeHtml(text);
    s = s.replace(/-&gt;/g, '<span class="tok-arrow">-&gt;</span>');
    s = s.replace(keywordRe, '<span class="tok-keyword">$1</span>');
    return s;
  }

  function syncScroll() {
    highlight.parentElement.scrollTop = editor.scrollTop;
    highlight.parentElement.scrollLeft = editor.scrollLeft;
  }

  // --- Autocomplete --------------------------------------------------------

  let activeIndex = -1;
  let debounceTimer = null;

  function hideSuggestions() {
    suggestions.hidden = true;
    suggestions.innerHTML = "";
    activeIndex = -1;
  }

  async function fetchCompletions() {
    const line = editor.value;
    const pos = editor.selectionStart;
    try {
      const url = "/api/complete?line=" + encodeURIComponent(line) + "&pos=" + pos;
      const resp = await fetch(url);
      const data = await resp.json();
      renderSuggestions(data.candidates || []);
    } catch (e) {
      hideSuggestions();
    }
  }

  function scheduleCompletions() {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(fetchCompletions, 150);
  }

  function renderSuggestions(candidates) {
    if (!candidates.length) {
      hideSuggestions();
      return;
    }
    suggestions.innerHTML = "";
    candidates.forEach((c, i) => {
      const li = document.createElement("li");
      li.textContent = c;
      li.addEventListener("mousedown", (ev) => {
        ev.preventDefault();
        insertCandidate(c);
      });
      suggestions.appendChild(li);
    });
    activeIndex = 0;
    setActive(0);
    positionSuggestions();
    suggestions.hidden = false;
  }

  // positionSuggestions places the popup just below the text caret, overlaying
  // the textarea, by measuring the caret's pixel coordinates.
  function positionSuggestions() {
    const caret = caretCoordinates(editor, editor.selectionStart);
    suggestions.style.left = Math.max(0, caret.left - editor.scrollLeft) + "px";
    suggestions.style.top = caret.top + caret.height - editor.scrollTop + "px";
  }

  // caretCoordinates returns the caret's {top,left,height} within the textarea
  // by rendering an off-screen mirror <div> that copies the textarea's box and
  // text styles, then measuring a marker span at the caret offset. This is the
  // standard technique for locating a caret in a <textarea>.
  function caretCoordinates(el, position) {
    const mirror = document.createElement("div");
    const computed = window.getComputedStyle(el);
    const props = [
      "boxSizing", "width", "paddingTop", "paddingRight", "paddingBottom",
      "paddingLeft", "borderTopWidth", "borderRightWidth", "borderBottomWidth",
      "borderLeftWidth", "fontStyle", "fontVariant", "fontWeight", "fontStretch",
      "fontSize", "lineHeight", "fontFamily", "textAlign", "textIndent",
      "letterSpacing", "wordSpacing", "tabSize", "whiteSpace", "wordBreak",
      "overflowWrap",
    ];
    const s = mirror.style;
    s.position = "absolute";
    s.visibility = "hidden";
    s.whiteSpace = "pre-wrap";
    s.wordWrap = "break-word";
    s.top = "0";
    s.left = "0";
    props.forEach((p) => { s[p] = computed[p]; });

    mirror.textContent = el.value.slice(0, position);
    const span = document.createElement("span");
    // Non-empty content so the span has a measurable box at the caret.
    span.textContent = el.value.slice(position) || ".";
    mirror.appendChild(span);

    document.body.appendChild(mirror);
    const coords = {
      top: span.offsetTop,
      left: span.offsetLeft,
      height: parseInt(computed.lineHeight, 10) || parseInt(computed.fontSize, 10),
    };
    document.body.removeChild(mirror);
    return coords;
  }

  function setActive(i) {
    const items = suggestions.querySelectorAll("li");
    items.forEach((el, idx) => el.classList.toggle("active", idx === i));
  }

  // Replace the partial word ending at the cursor with the full candidate token.
  function insertCandidate(candidate) {
    const value = editor.value;
    const pos = editor.selectionStart;
    const before = value.slice(0, pos);
    const wordMatch = before.match(/[A-Za-z_][A-Za-z0-9_]*$/);
    const start = wordMatch ? pos - wordMatch[0].length : pos;
    const next = value.slice(0, start) + candidate + value.slice(pos);
    editor.value = next;
    const caret = start + candidate.length;
    editor.setSelectionRange(caret, caret);
    hideSuggestions();
    renderHighlight(editor.value);
  }

  // --- Query submission ----------------------------------------------------

  async function runQuery() {
    const sql = editor.value.trim();
    if (!sql) return;

    // Reflect the submitted query in the URL so it is bookmarkable and the
    // browser's Back/Forward buttons move between queries. Only push a new
    // history entry when the query actually changed (comparing decoded values,
    // so it is robust to encoding differences) — this also avoids duplicating
    // the entry on initial load or when restoring from popstate.
    if (new URLSearchParams(location.search).get("sql") !== sql) {
      const params = new URLSearchParams();
      params.set("sql", sql);
      history.pushState({ sql }, "", location.pathname + "?" + params.toString());
    }

    runBtn.disabled = true;
    results.innerHTML = '<p class="empty">Running…</p>';
    try {
      const resp = await fetch("/api/query", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql }),
      });
      const data = await resp.json();
      if (!resp.ok) {
        renderError(data);
      } else {
        renderTable(data);
      }
    } catch (e) {
      renderError({ error: String(e) });
    } finally {
      runBtn.disabled = false;
    }
  }

  function renderError(data) {
    results.innerHTML = "";
    const p = document.createElement("p");
    p.className = "error";
    p.textContent = data.error || "query failed";
    results.appendChild(p);
    if (data.correctedSql) {
      const s = document.createElement("div");
      s.className = "suggestion";
      s.textContent = data.suggestion || "Did you mean:";
      const btn = document.createElement("button");
      btn.textContent = data.correctedSql;
      btn.addEventListener("click", () => {
        editor.value = data.correctedSql;
        renderHighlight(editor.value);
        runQuery();
      });
      s.appendChild(btn);
      results.appendChild(s);
    }
  }

  function renderTable(data) {
    const columns = data.columns || [];
    const rows = data.rows || [];
    if (!rows.length) {
      results.innerHTML = '<p class="empty">No rows matched.</p>';
      return;
    }
    const table = document.createElement("table");
    const thead = document.createElement("thead");
    const htr = document.createElement("tr");
    columns.forEach((c) => {
      const th = document.createElement("th");
      const label = document.createElement("span");
      label.className = "th-label";
      label.textContent = c;
      th.appendChild(label);
      const grip = document.createElement("span");
      grip.className = "col-resizer";
      attachResize(grip, th);
      th.appendChild(grip);
      htr.appendChild(th);
    });
    thead.appendChild(htr);
    table.appendChild(thead);

    const tbody = document.createElement("tbody");
    rows.forEach((row) => {
      const tr = document.createElement("tr");
      columns.forEach((c) => {
        tr.appendChild(buildCell(row[c]));
      });
      tbody.appendChild(tr);
    });
    table.appendChild(tbody);

    results.innerHTML = "";
    results.appendChild(table);
  }

  // attachResize wires a header grip so dragging it changes the column width.
  // The dragged-from width and pointer X are captured on mousedown; document
  // listeners track the drag so it continues even when the pointer leaves the
  // grip, and are torn down on release.
  function attachResize(grip, th) {
    let startX = 0;
    let startWidth = 0;

    const onMove = (e) => {
      const width = Math.max(40, startWidth + (e.clientX - startX));
      th.style.width = width + "px";
    };
    const onUp = () => {
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
      document.body.classList.remove("col-resizing");
    };

    grip.addEventListener("mousedown", (e) => {
      e.preventDefault();
      e.stopPropagation();
      startX = e.clientX;
      startWidth = th.offsetWidth;
      document.addEventListener("mousemove", onMove);
      document.addEventListener("mouseup", onUp);
      document.body.classList.add("col-resizing");
    });
  }

  // buildCell renders a value into a <td>. Composite values (objects/arrays)
  // are shown as colored YAML — the same shape the CLI's table renderer uses —
  // rather than JSON. Scalars are plain text.
  function buildCell(v) {
    const td = document.createElement("td");
    if (v !== null && typeof v === "object") {
      const pre = document.createElement("pre");
      pre.className = "yaml-cell";
      // innerHTML is built only from escaped content plus known <span> tags.
      pre.innerHTML = yamlLines(v, 0).join("\n");
      td.appendChild(pre);
    } else if (v === null || v === undefined) {
      td.textContent = "";
    } else {
      td.textContent = String(v);
    }
    return td;
  }

  // yamlLines serializes value to an array of YAML line strings (HTML), with
  // keys and scalars wrapped in colorizing spans. Nested objects/arrays are
  // emitted as indented blocks; the dash for composite list items sits on its
  // own line so deep structures stay readable.
  function yamlLines(value, indent) {
    const pad = "  ".repeat(indent);
    const lines = [];

    if (Array.isArray(value)) {
      if (!value.length) {
        lines.push(pad + "[]");
        return lines;
      }
      value.forEach((item) => {
        if (isComposite(item)) {
          lines.push(pad + "-");
          yamlLines(item, indent + 1).forEach((l) => lines.push(l));
        } else {
          lines.push(pad + "- " + yamlScalar(item));
        }
      });
      return lines;
    }

    if (value !== null && typeof value === "object") {
      const keys = Object.keys(value);
      if (!keys.length) {
        lines.push(pad + "{}");
        return lines;
      }
      keys.forEach((k) => {
        const key = pad + '<span class="yaml-key">' + escapeHtml(k) + "</span>:";
        const val = value[k];
        if (isComposite(val)) {
          lines.push(key);
          yamlLines(val, indent + 1).forEach((l) => lines.push(l));
        } else {
          lines.push(key + " " + yamlScalar(val));
        }
      });
      return lines;
    }

    lines.push(pad + yamlScalar(value));
    return lines;
  }

  // isComposite reports whether a value should be expanded onto its own block
  // (a non-empty object or array). Empty containers render inline as {} / [].
  function isComposite(v) {
    if (v === null || typeof v !== "object") return false;
    if (Array.isArray(v)) return v.length > 0;
    return Object.keys(v).length > 0;
  }

  function yamlScalar(v) {
    if (v === null || v === undefined) {
      return '<span class="yaml-null">null</span>';
    }
    if (typeof v === "number") {
      return '<span class="yaml-num">' + escapeHtml(String(v)) + "</span>";
    }
    if (typeof v === "boolean") {
      return '<span class="yaml-bool">' + escapeHtml(String(v)) + "</span>";
    }
    return '<span class="yaml-string">' + escapeHtml(String(v)) + "</span>";
  }

  // --- Event wiring --------------------------------------------------------

  editor.addEventListener("input", () => {
    renderHighlight(editor.value);
    scheduleCompletions();
  });
  editor.addEventListener("scroll", syncScroll);

  editor.addEventListener("keydown", (ev) => {
    const popupOpen = !suggestions.hidden;
    if (popupOpen) {
      const items = suggestions.querySelectorAll("li");
      if (ev.key === "ArrowDown") {
        ev.preventDefault();
        activeIndex = (activeIndex + 1) % items.length;
        setActive(activeIndex);
        return;
      }
      if (ev.key === "ArrowUp") {
        ev.preventDefault();
        activeIndex = (activeIndex - 1 + items.length) % items.length;
        setActive(activeIndex);
        return;
      }
      if (ev.key === "Enter" || ev.key === "Tab") {
        ev.preventDefault();
        insertCandidate(items[activeIndex].textContent);
        return;
      }
      if (ev.key === "Escape") {
        ev.preventDefault();
        hideSuggestions();
        return;
      }
    } else if (ev.key === "Tab") {
      ev.preventDefault();
      fetchCompletions();
      return;
    }

    if ((ev.ctrlKey || ev.metaKey) && ev.key === "Enter") {
      ev.preventDefault();
      hideSuggestions();
      runQuery();
    }
  });

  runBtn.addEventListener("click", runQuery);

  // Escape hides the suggestion panel from anywhere on the page, not just when
  // focus is in the editor.
  document.addEventListener("keydown", (ev) => {
    if (ev.key === "Escape" && !suggestions.hidden) {
      hideSuggestions();
    }
  });

  // Back/Forward: restore the editor from the URL's ?sql= and re-run it (or
  // reset to the empty state when navigating back to a query-less URL). The URL
  // already reflects the target entry here, so runQuery won't push it again.
  window.addEventListener("popstate", () => {
    const sql = new URLSearchParams(location.search).get("sql") || "";
    editor.value = sql;
    hideSuggestions();
    renderHighlight(editor.value);
    if (sql) {
      runQuery();
    } else {
      results.innerHTML = '<p class="empty">Run a query to see results.</p>';
    }
  });

  // Pre-fill the editor from the ?sql= query string (set when the CLI is given a
  // positional query) and run it immediately so results show on load.
  const initialSql = new URLSearchParams(window.location.search).get("sql");
  if (initialSql) {
    editor.value = initialSql;
    renderHighlight(editor.value);
    runQuery();
  } else {
    renderHighlight(editor.value);
  }
})();
