// gin-swagger-ui docs page: renders the OpenAPI document served at
// data-spec and lets you send requests to the documented routes.
(() => {
  "use strict";

  const METHOD_ORDER = ["get", "post", "put", "patch", "delete", "head", "options", "trace"];
  const BODY_METHODS = new Set(["post", "put", "patch"]);
  const AUTH_KEY = "gindocs.auth";

  const specURL = document.body.dataset.spec;
  const nav = document.getElementById("nav");
  const main = document.getElementById("main");
  const search = document.getElementById("search");

  const state = {
    spec: null,
    ops: [],
    view: null, // { op } or { schema: name }
    server: "",
    auth: loadAuth(), // scheme name -> credentials
    forms: new Map(), // op hash -> { values, body } typed into Try it
    results: new Map(), // op hash -> last response element
  };

  // ---------- DOM helpers ----------

  // h builds a DOM element; null, false and "" children are skipped.
  function h(tag, attrs, ...children) {
    const el = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs || {})) {
      if (v == null || v === false) continue;
      if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
      else if (k === "class") el.className = v;
      else el.setAttribute(k, v === true ? "" : v);
    }
    append(el, children);
    return el;
  }

  function append(el, children) {
    for (const c of children.flat(Infinity)) {
      if (c != null && c !== false && c !== "") el.append(c.nodeType ? c : String(c));
    }
  }

  function set(el, ...children) {
    el.replaceChildren();
    append(el, children);
  }

  const ICONS = {
    lock: "M7 11V7a5 5 0 0 1 10 0v4M5 11h14v10H5z",
    unlock: "M7 11V7a5 5 0 0 1 9.6-2M5 11h14v10H5z",
  };

  function icon(name) {
    const ns = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("class", "icon");
    svg.setAttribute("aria-hidden", "true");
    const path = document.createElementNS(ns, "path");
    path.setAttribute("d", ICONS[name]);
    svg.append(path);
    return svg;
  }

  // inline renders `code`, **bold** and [links](url) in descriptions.
  function inline(text) {
    text = String(text);
    const out = [];
    let last = 0;
    for (const m of text.matchAll(/`([^`]+)`|\*\*([^*]+)\*\*|\[([^\]]+)\]\(([^)\s]+)\)/g)) {
      out.push(text.slice(last, m.index));
      if (m[1] != null) out.push(h("code", null, m[1]));
      else if (m[2] != null) out.push(h("strong", null, m[2]));
      else if (/^(https?:|mailto:|\/|#)/.test(m[4])) out.push(h("a", { href: m[4], target: m[4][0] === "#" ? null : "_blank", rel: "noopener" }, m[3]));
      else out.push(m[3]);
      last = m.index + m[0].length;
    }
    out.push(text.slice(last));
    return out;
  }

  // md splits text into paragraphs on blank lines.
  const md = (text) => (text ? String(text).trim().split(/\n\s*\n/).map((p) => h("p", null, inline(p))) : []);

  const methodTag = (m) => h("span", { class: "method m-" + m }, m.toUpperCase());

  // pathNodes highlights {param} segments of an OpenAPI path.
  const pathNodes = (path) =>
    path.split(/(\{[^}]+\})/).filter(Boolean).map((part) =>
      part.startsWith("{") ? h("span", { class: "param" }, part) : part);

  const absolute = (url) => new URL(url, location.href).href;

  function formatBytes(n) {
    if (n < 1024) return n + " B";
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " KB";
    return (n / 1024 / 1024).toFixed(1) + " MB";
  }

  // ---------- schemas ----------

  const schemas = () => (state.spec.components && state.spec.components.schemas) || {};
  const refName = (ref) => ref.split("/").pop();
  const resolve = (s) => (s && s.$ref ? schemas()[refName(s.$ref)] : s);

  function typeLabel(s) {
    if (!s) return "any";
    if (s.$ref) return refName(s.$ref);
    if (s.type === "array") return "array<" + typeLabel(s.items) + ">";
    if (s.type === "object" && s.additionalProperties && !s.properties) {
      return "map<string, " + typeLabel(s.additionalProperties) + ">";
    }
    if (!s.type) return "any";
    return s.format ? s.type + " (" + s.format + ")" : s.type;
  }

  // typeNode is typeLabel with schema names linked to their page.
  function typeNode(s) {
    if (s && s.$ref) {
      const name = refName(s.$ref);
      return h("a", { class: "type-link", href: "#schema/" + name }, name);
    }
    if (s && s.type === "array") return h("span", null, "array<", typeNode(s.items), ">");
    return h("span", null, typeLabel(s));
  }

  function constraints(s) {
    if (!s || s.$ref) return null;
    const out = [];
    if (s.enum) out.push("one of: " + s.enum.map((v) => JSON.stringify(v)).join(", "));
    if (s.minimum != null) out.push("≥ " + s.minimum);
    if (s.maximum != null) out.push("≤ " + s.maximum);
    if (s.minLength != null) out.push("min length " + s.minLength);
    if (s.maxLength != null) out.push("max length " + s.maxLength);
    if (s.minItems != null) out.push("min items " + s.minItems);
    if (s.maxItems != null) out.push("max items " + s.maxItems);
    if (s.default !== undefined) out.push("default: " + JSON.stringify(s.default));
    if (s.example !== undefined) out.push("example: " + JSON.stringify(s.example));
    return out.length ? h("div", { class: "constraints" }, out.map((c) => h("span", { class: "constraint" }, c))) : null;
  }

  function isComplex(s) {
    const r = resolve(s);
    if (!r) return false;
    if (r.type === "array") return isComplex(r.items);
    return Object.keys(r.properties || {}).length > 0 || isComplex(r.additionalProperties);
  }

  // schemaTree renders an object's properties, expanding nested objects.
  // seen holds the schema names on the current branch to stop recursion.
  function schemaTree(s, seen = new Set(), depth = 0) {
    if (!s) return h("p", { class: "muted" }, "Any JSON value.");
    if (s.$ref) {
      const name = refName(s.$ref);
      if (seen.has(name)) return h("p", { class: "muted small" }, "Recursive: see ", typeNode(s), ".");
      const r = resolve(s);
      if (!r) return h("p", { class: "muted" }, "Unknown schema " + name + ".");
      return schemaTree(r, new Set(seen).add(name), depth);
    }
    if (s.type === "array") return schemaTree(s.items, seen, depth);
    const props = Object.entries(s.properties || {});
    if (!props.length) {
      if (s.additionalProperties) {
        return h("div", { class: "tree" }, propRow("[key: string]", s.additionalProperties, false, seen, depth));
      }
      return h("div", { class: "tree-leaf" }, h("code", null, typeLabel(s)), constraints(s));
    }
    const required = new Set(s.required || []);
    return h("div", { class: "tree" }, props.map(([name, ps]) => propRow(name, ps, required.has(name), seen, depth)));
  }

  function propRow(name, s, required, seen, depth) {
    const head = [
      h("code", { class: "prop-name" }, name),
      h("span", { class: "prop-type" }, typeNode(s)),
      required && h("span", { class: "req" }, "required"),
    ];
    const own = s && !s.$ref ? s : null; // OpenAPI ignores siblings of $ref
    const info = [own && own.description && h("div", { class: "prop-desc" }, inline(own.description)), constraints(own)];
    if (!isComplex(s)) return h("div", { class: "prop" }, h("div", { class: "prop-head" }, head), info);
    return h("details", { class: "prop", open: depth < 1 },
      h("summary", { class: "prop-head" }, head), info, schemaTree(s, seen, depth + 1));
  }

  const SAMPLE_STRINGS = {
    "date-time": "2025-01-01T12:00:00Z",
    date: "2025-01-01",
    email: "user@example.com",
    uuid: "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    uri: "https://example.com",
    byte: "aGVsbG8=",
  };

  // example builds a sample value for a schema, like Swagger's "Example Value".
  function example(s, seen = new Set()) {
    if (!s) return {};
    if (s.example !== undefined) return s.example;
    if (s.$ref) {
      const name = refName(s.$ref);
      if (seen.has(name)) return {};
      return example(resolve(s), new Set(seen).add(name));
    }
    if (s.enum) return s.enum[0];
    if (s.default !== undefined) return s.default;
    switch (s.type) {
      case "string": return SAMPLE_STRINGS[s.format] || "string";
      case "integer":
      case "number": return s.minimum != null ? s.minimum : 0;
      case "boolean": return true;
      case "array": return [example(s.items, seen)];
    }
    if (s.properties) {
      const obj = {};
      for (const [k, v] of Object.entries(s.properties)) obj[k] = example(v, seen);
      return obj;
    }
    if (s.additionalProperties) return { additionalProp1: example(s.additionalProperties, seen) };
    return {};
  }

  const exampleText = (s) => JSON.stringify(example(s), null, 2);
  const codeBlock = (text) => h("pre", { class: "code" }, text);

  function tabs(items) {
    const body = h("div", { class: "tab-body" });
    const buttons = items.map((it, i) =>
      h("button", { type: "button", class: "tab", role: "tab", onclick: () => select(i) }, it.label));
    function select(i) {
      buttons.forEach((b, j) => b.setAttribute("aria-selected", String(i === j)));
      set(body, items[i].render());
    }
    select(0);
    return h("div", { class: "tabs" }, h("div", { class: "tab-list", role: "tablist" }, buttons), body);
  }

  // mediaBlock shows each content type with Example and Schema tabs.
  function mediaBlock(content) {
    return Object.entries(content || {}).map(([type, media]) =>
      h("div", { class: "media" },
        h("div", { class: "media-type" }, type),
        media.schema && tabs([
          { label: "Example", render: () => codeBlock(exampleText(media.schema)) },
          {
            label: "Schema",
            render: () => h("div", { class: "schema-box" },
              h("div", { class: "schema-type" }, typeNode(media.schema)),
              schemaTree(media.schema)),
          },
        ])));
  }

  // ---------- security ----------

  const schemes = () => (state.spec.components && state.spec.components.securitySchemes) || {};

  // requirements lists the alternative security requirements of an operation.
  const requirements = (o) => o.op.security || state.spec.security || [];

  // activeRequirement returns the first requirement whose schemes all have
  // credentials, or null.
  const activeRequirement = (o) =>
    requirements(o).find((r) => Object.keys(r).every((name) => state.auth[name])) || null;

  function loadAuth() {
    try {
      return JSON.parse(sessionStorage.getItem(AUTH_KEY)) || {};
    } catch {
      return {};
    }
  }

  function saveAuth() {
    try {
      sessionStorage.setItem(AUTH_KEY, JSON.stringify(state.auth));
    } catch { /* storage unavailable: keep credentials in memory only */ }
  }

  const base64 = (s) => btoa(String.fromCharCode(...new TextEncoder().encode(s)));

  function schemeLabel(sc) {
    if (sc.type === "http" && sc.scheme === "bearer") return "HTTP Bearer token";
    if (sc.type === "http" && sc.scheme === "basic") return "HTTP Basic";
    if (sc.type === "apiKey") return "API key in " + sc.in + " " + JSON.stringify(sc.name);
    return sc.type;
  }

  function openAuth() {
    const dlg = h("dialog", { class: "auth-dialog", "aria-labelledby": "auth-title" });
    const render = () => set(dlg,
      h("div", { class: "dialog-head" },
        h("h2", { id: "auth-title" }, "Authorize"),
        h("button", { type: "button", class: "btn-ghost", onclick: () => dlg.close() }, "Done")),
      h("p", { class: "muted small" }, "Credentials are sent with Try it requests and kept until this tab closes."),
      Object.entries(schemes()).map(([name, sc]) => authSection(name, sc, render)));
    render();
    dlg.addEventListener("close", () => {
      dlg.remove();
      renderHeader();
      renderView();
    });
    document.body.append(dlg);
    dlg.showModal();
  }

  function authSection(name, sc, rerender) {
    const authorized = !!state.auth[name];
    const head = h("div", { class: "auth-head" },
      h("code", null, name),
      h("span", { class: "muted" }, schemeLabel(sc)),
      authorized && h("span", { class: "badge ok" }, icon("lock"), "Authorized"));
    if (authorized) {
      return h("section", { class: "auth-section" }, head,
        h("button", {
          type: "button",
          class: "btn-ghost",
          onclick: () => { delete state.auth[name]; saveAuth(); rerender(); },
        }, "Log out"));
    }

    let inputs;
    if (sc.type === "http" && sc.scheme === "basic") {
      inputs = [
        h("input", { name: "username", placeholder: "Username", autocomplete: "username", required: true }),
        h("input", { name: "password", type: "password", placeholder: "Password", autocomplete: "current-password", required: true }),
      ];
    } else if ((sc.type === "http" && sc.scheme === "bearer") || sc.type === "apiKey") {
      inputs = [h("input", {
        name: "value", type: "password", autocomplete: "off", required: true,
        placeholder: sc.type === "apiKey" ? "API key" : "Token",
      })];
    } else {
      return h("section", { class: "auth-section" }, head,
        h("p", { class: "muted small" }, "The docs UI can't send this kind of credential."));
    }
    const form = h("form", {
      class: "auth-form",
      onsubmit: (e) => {
        e.preventDefault();
        state.auth[name] = Object.fromEntries(new FormData(form));
        saveAuth();
        rerender();
      },
    }, inputs, h("button", { type: "submit", class: "btn" }, "Authorize"));
    return h("section", { class: "auth-section" }, head, form);
  }

  function applyAuth(o, headers, query, cookies) {
    const req = activeRequirement(o);
    for (const name of Object.keys(req || {})) {
      const sc = schemes()[name];
      const c = state.auth[name];
      if (!sc || !c) continue;
      if (sc.type === "http" && sc.scheme === "bearer") headers.Authorization = "Bearer " + c.value;
      else if (sc.type === "http" && sc.scheme === "basic") headers.Authorization = "Basic " + base64(c.username + ":" + c.password);
      else if (sc.type === "apiKey" && sc.in === "header") headers[sc.name] = c.value;
      else if (sc.type === "apiKey" && sc.in === "query") query.append(sc.name, c.value);
      else if (sc.type === "apiKey" && sc.in === "cookie") cookies.push(sc.name + "=" + c.value);
    }
  }

  // ---------- header & navigation ----------

  const extLink = (href, text) => h("a", { class: "chip link", href, target: "_blank", rel: "noopener" }, text);

  function renderHeader() {
    const spec = state.spec;
    const info = spec.info || {};
    if (info.title) {
      document.title = info.title;
      document.getElementById("title").textContent = info.title;
    }

    const links = [];
    if (info.termsOfService) links.push(extLink(info.termsOfService, "Terms"));
    if (info.contact) {
      const c = info.contact;
      const href = c.url || (c.email && "mailto:" + c.email);
      links.push(href ? extLink(href, c.name || "Contact") : h("span", { class: "chip" }, c.name));
    }
    if (info.license) {
      links.push(info.license.url ? extLink(info.license.url, info.license.name) : h("span", { class: "chip" }, info.license.name));
    }
    set(document.getElementById("meta"),
      info.version && h("span", { class: "chip" }, "v" + info.version),
      spec.openapi && h("span", { class: "chip" }, "OpenAPI " + spec.openapi),
      h("span", { class: "chip" }, state.ops.length + (state.ops.length === 1 ? " endpoint" : " endpoints")),
      h("a", { class: "chip link", href: specURL, target: "_blank" }, specURL),
      links);

    const servers = spec.servers || [];
    let serverSelect = null;
    if (servers.length > 1) {
      serverSelect = h("select", {
        "aria-label": "Server",
        onchange: () => { state.server = serverSelect.value.replace(/\/$/, ""); renderView(); },
      }, servers.map((s) => h("option", { value: s.url }, s.description ? s.description + " — " + s.url : s.url)));
      serverSelect.value = (servers.find((s) => s.url.replace(/\/$/, "") === state.server) || servers[0]).url;
    }
    const anyAuth = Object.keys(state.auth).some((k) => schemes()[k]);
    set(document.getElementById("actions"),
      serverSelect && h("label", { class: "server" }, h("span", { class: "muted small" }, "Server"), serverSelect),
      Object.keys(schemes()).length > 0 && h("button", {
        type: "button",
        class: "btn authorize" + (anyAuth ? " on" : ""),
        onclick: openAuth,
      }, icon(anyAuth ? "lock" : "unlock"), anyAuth ? "Authorized" : "Authorize"));

    set(document.getElementById("info"), md(info.description));
  }

  function renderNav() {
    const q = search.value.trim().toLowerCase();
    const match = (...xs) => !q || xs.some((x) => x && x.toLowerCase().includes(q));

    // Declared tags first (in their order), then tags as routes use them.
    const groups = new Map((state.spec.tags || []).map((t) => [t.name, []]));
    for (const o of state.ops) {
      if (!match(o.method + " " + o.path, o.op.summary, o.tag, o.op.operationId)) continue;
      if (!groups.has(o.tag)) groups.set(o.tag, []);
      groups.get(o.tag).push(o);
    }
    const sections = [...groups].filter(([, list]) => list.length).map(([name, list]) =>
      h("section", { class: "group" },
        h("h2", null, name, h("span", { class: "count" }, list.length)),
        list.map(navItem)));

    const names = Object.keys(schemas()).sort().filter((n) => match(n));
    if (names.length) {
      sections.push(h("section", { class: "group" },
        h("h2", null, "Schemas", h("span", { class: "count" }, names.length)),
        names.map((n) => h("a", {
          href: "#schema/" + n,
          class: "route" + (state.view && state.view.schema === n ? " active" : ""),
        }, h("span", { class: "method m-schema" }, "{ }"), h("span", { class: "route-path" }, n)))));
    }

    if (sections.length) set(nav, sections);
    else set(nav, h("p", { class: "nav-empty" }, state.ops.length ? "No matches." : "No routes registered."));
  }

  function navItem(o) {
    const active = state.view && state.view.op === o;
    return h("a", { href: o.hash, class: "route" + (active ? " active" : "") + (o.op.deprecated ? " deprecated" : "") },
      methodTag(o.method),
      h("span", { class: "route-text" },
        h("span", { class: "route-path" }, pathNodes(o.path)),
        o.op.summary && h("span", { class: "route-summary" }, o.op.summary)));
  }

  function route() {
    const hash = decodeURIComponent(location.hash);
    const name = hash.startsWith("#schema/") ? hash.slice("#schema/".length) : null;
    if (name && schemas()[name]) {
      state.view = { schema: name };
    } else {
      state.view = { op: state.ops.find((o) => o.hash === hash) || state.ops[0] || null };
    }
    renderNav();
    renderView();
    window.scrollTo(0, 0);
  }

  function renderView() {
    if (state.view.schema) renderSchema(state.view.schema);
    else renderOp(state.view.op);
  }

  // ---------- pages ----------

  function renderSchema(name) {
    const s = schemas()[name];
    const ref = "#/components/schemas/" + name;
    const usedBy = state.ops.filter((o) => JSON.stringify(o.op).includes(JSON.stringify(ref)));
    set(main,
      h("div", { class: "op-head" },
        h("span", { class: "method m-schema" }, "SCHEMA"),
        h("h2", { class: "op-path" }, name)),
      s.description && h("div", { class: "md" }, md(s.description)),
      h("section", { class: "block" },
        tabs([
          { label: "Schema", render: () => schemaTree(s, new Set([name])) },
          { label: "Example", render: () => codeBlock(exampleText({ $ref: ref })) },
        ])),
      usedBy.length > 0 && h("section", { class: "block" },
        h("h3", null, "Used by"),
        h("div", { class: "used-by" }, usedBy.map(navItem))));
  }

  function renderOp(o) {
    if (!o) {
      set(main, h("div", { class: "empty" },
        h("h2", null, "No routes yet"),
        h("p", null, "Register routes on your Gin engine and they will appear here.")));
      return;
    }
    const op = o.op;
    const desc = md(op.description);
    const responses = Object.entries(op.responses || {}).sort(([a], [b]) => a.localeCompare(b));
    set(main,
      h("div", { class: "op-head" },
        methodTag(o.method),
        h("h2", { class: "op-path" + (op.deprecated ? " deprecated" : "") }, pathNodes(o.path)),
        op.deprecated && h("span", { class: "badge warn" }, "Deprecated"),
        requirements(o).length > 0 && authBadge(o)),
      op.summary && h("p", { class: "summary" }, op.summary),
      desc.length > 0 && h("div", { class: "md" }, desc),
      h("div", { class: "op-meta" },
        (op.tags || []).map((t) => h("span", { class: "chip" }, t)),
        op.operationId && h("span", { class: "chip", title: "operationId" }, op.operationId)),

      paramsBlock(o),

      op.requestBody && h("section", { class: "block" },
        h("h3", null, "Request body", op.requestBody.required && h("span", { class: "req" }, "required")),
        md(op.requestBody.description),
        mediaBlock(op.requestBody.content)),

      h("section", { class: "block" },
        h("h3", null, "Responses"),
        h("div", { class: "responses" }, responses.map(([code, r]) =>
          h("div", { class: "response-item" },
            h("div", { class: "response-head" },
              h("code", { class: "res-code s" + code[0] }, code),
              h("span", null, inline(r.description || ""))),
            mediaBlock(r.content))))),

      tryPanel(o));
  }

  function authBadge(o) {
    const ok = !!activeRequirement(o);
    const names = [...new Set(requirements(o).flatMap(Object.keys))].join(" or ");
    return h("button", {
      type: "button",
      class: "badge lock" + (ok ? " ok" : ""),
      title: "Requires " + names,
      onclick: openAuth,
    }, icon(ok ? "lock" : "unlock"), ok ? "Authorized" : "Auth required");
  }

  function paramsBlock(o) {
    return h("section", { class: "block" },
      h("h3", null, "Parameters"),
      o.params.length
        ? h("div", { class: "table-wrap" }, h("table", { class: "params" },
          h("thead", null, h("tr", null,
            h("th", null, "Name"), h("th", null, "In"), h("th", null, "Type"), h("th", null, "Description"))),
          h("tbody", null, o.params.map((p) => h("tr", null,
            h("td", null, h("code", null, p.name), p.required && h("span", { class: "req" }, "required")),
            h("td", null, p.in),
            h("td", null, h("code", null, typeNode(p.schema)), constraints(p.schema)),
            h("td", { class: "muted" }, p.description ? inline(p.description) : "—"))))))
        : h("p", { class: "muted" }, "No parameters."));
  }

  // ---------- try it ----------

  function paramControl(p, form) {
    const s = p.schema || {};
    const choices = s.enum || (s.type === "boolean" ? [true, false] : null);
    let el;
    if (choices) {
      el = h("select", { name: p.name, required: p.required },
        h("option", { value: "" }, p.required ? "Select…" : "—"),
        choices.map((v) => h("option", { value: String(v) }, String(v))));
    } else {
      el = h("input", {
        type: "text", name: p.name, required: p.required, autocomplete: "off", spellcheck: "false",
        inputmode: s.type === "integer" ? "numeric" : s.type === "number" ? "decimal" : null,
        placeholder: s.example !== undefined ? String(s.example) : s.type === "array" ? "comma, separated, values" : p.name,
      });
    }
    const saved = form.values[p.name];
    el.value = saved !== undefined ? saved : s.default !== undefined ? String(s.default) : "";
    el.addEventListener("input", () => { form.values[p.name] = el.value; });
    return el;
  }

  function buildRequest(o, fields, body, ctype) {
    let path = o.path;
    const query = new URLSearchParams();
    const headers = {};
    const cookies = [];
    for (const { p, el } of fields) {
      const v = el.value;
      if (v === "") continue; // empty path params keep their {placeholder}
      const values = p.schema && p.schema.type === "array"
        ? v.split(",").map((x) => x.trim()).filter(Boolean)
        : [v];
      if (p.in === "path") {
        // Encode per segment so catch-all params can contain "/".
        path = path.replace("{" + p.name + "}", v.split("/").map(encodeURIComponent).join("/"));
      } else if (p.in === "query") {
        values.forEach((x) => query.append(p.name, x));
      } else if (p.in === "header") {
        headers[p.name] = values.join(",");
      } else if (p.in === "cookie") {
        cookies.push(p.name + "=" + v);
      }
    }
    applyAuth(o, headers, query, cookies);
    const data = body ? body.trim() : "";
    if (data) headers["Content-Type"] = ctype;
    const qs = query.toString();
    return { url: state.server + path + (qs ? "?" + qs : ""), headers, data, cookies };
  }

  function toCurl(o, r) {
    const sh = (s) => "'" + s.replace(/'/g, "'\\''") + "'";
    const parts = ["curl"];
    if (o.method === "head") parts.push("-I");
    else if (o.method !== "get") parts.push("-X " + o.method.toUpperCase());
    parts.push(sh(absolute(r.url)));
    for (const [k, v] of Object.entries(r.headers)) parts.push("-H " + sh(k + ": " + v));
    if (r.cookies.length) parts.push("-b " + sh(r.cookies.join("; ")));
    if (r.data) parts.push("--data " + sh(r.data));
    return parts.join(parts.length > 3 ? " \\\n  " : " ");
  }

  function tryPanel(o) {
    const form = state.forms.get(o.hash) || { values: {}, body: null };
    state.forms.set(o.hash, form);
    let out = state.results.get(o.hash);
    if (!out) {
      out = h("div", { class: "result", "aria-live": "polite" });
      state.results.set(o.hash, out);
    }

    const fields = o.params.map((p) => ({ p, el: paramControl(p, form) }));

    const content = (o.op.requestBody && o.op.requestBody.content) || null;
    const ctype = content ? Object.keys(content)[0] : "application/json";
    const schema = content && content[ctype] && content[ctype].schema;
    let bodyEl = null;
    let bodyErr = null;
    let resetBtn = null;
    if (content || BODY_METHODS.has(o.method)) {
      const initial = schema ? exampleText(schema) : "";
      if (form.body == null) form.body = initial;
      bodyEl = h("textarea", {
        rows: Math.min(16, Math.max(5, form.body.split("\n").length + 1)),
        spellcheck: "false",
        placeholder: '{\n  "name": "Ada"\n}',
        oninput: () => { form.body = bodyEl.value; },
      });
      bodyEl.value = form.body;
      bodyErr = h("div", { class: "field-error", role: "alert" });
      if (schema) {
        resetBtn = h("button", {
          type: "button",
          class: "link-btn",
          onclick: () => { bodyEl.value = form.body = initial; refresh(); },
        }, "Reset to example");
      }
    }

    const curl = h("pre", { class: "curl" });
    const send = h("button", { type: "submit", class: "btn" }, "Send request");
    const build = () => buildRequest(o, fields, bodyEl && bodyEl.value, ctype);
    const refresh = () => { curl.textContent = toCurl(o, build()); };

    const submit = async (e) => {
      e.preventDefault();
      if (bodyEl && bodyEl.value.trim() && ctype.includes("json")) {
        try {
          JSON.parse(bodyEl.value);
          bodyErr.textContent = "";
        } catch (err) {
          bodyErr.textContent = "Body is not valid JSON: " + err.message;
          bodyEl.focus();
          return;
        }
      }
      const r = build();
      send.disabled = true;
      send.textContent = "Sending…";
      const t0 = performance.now();
      try {
        const init = { method: o.method.toUpperCase(), headers: r.headers };
        if (r.data) init.body = r.data;
        const res = await fetch(r.url, init);
        const text = await res.text();
        showResult(out, r, res, text, performance.now() - t0);
      } catch (err) {
        set(out, h("div", { class: "error" }, "Request failed: " + err.message));
      } finally {
        send.disabled = false;
        send.textContent = "Send request";
      }
    };

    const formEl = h("form", {
      class: "try-form",
      onsubmit: submit,
      oninput: refresh,
      onchange: refresh,
      onkeydown: (e) => {
        if ((e.metaKey || e.ctrlKey) && e.key === "Enter") formEl.requestSubmit();
      },
    },
    fields.length > 0 && h("div", { class: "fields" }, fields.map(({ p, el }) =>
      h("label", { class: "field" },
        h("span", { class: "field-name" }, p.name,
          p.required && h("span", { class: "req" }, "*"),
          h("small", null, p.in + " · " + typeLabel(p.schema))),
        el))),
    bodyEl && h("div", { class: "field" },
      h("div", { class: "field-row" },
        h("span", { class: "field-name" }, "Body", h("small", null, ctype + (content ? "" : " · optional"))),
        resetBtn),
      bodyEl,
      bodyErr),
    h("div", { class: "actions" },
      send,
      h("button", { type: "button", class: "btn-ghost", onclick: () => set(out) }, "Clear"),
      h("span", { class: "hint" }, "⌘/Ctrl + Enter")));

    const copy = navigator.clipboard && h("button", {
      type: "button",
      class: "copy",
      onclick: async () => {
        try {
          await navigator.clipboard.writeText(curl.textContent);
          copy.textContent = "Copied";
        } catch {
          copy.textContent = "Copy failed";
        }
        setTimeout(() => { copy.textContent = "Copy"; }, 1200);
      },
    }, "Copy");

    let authNote = null;
    if (requirements(o).length) {
      const active = activeRequirement(o);
      authNote = active
        ? h("span", { class: "auth-note ok" }, icon("lock"), "Sending credentials for " + Object.keys(active).join(" + "))
        : h("button", { type: "button", class: "auth-note", onclick: openAuth }, icon("unlock"), "Not authorized: add credentials");
    }

    refresh();
    return h("section", { class: "block try" },
      h("div", { class: "try-head" }, h("h3", null, "Try it"), authNote),
      formEl,
      h("div", { class: "curl-wrap" },
        h("div", { class: "curl-head" }, h("span", null, "cURL"), copy),
        curl),
      out);
  }

  function showResult(out, r, res, text, ms) {
    const ctype = res.headers.get("content-type") || "";
    let pretty = text;
    if (ctype.includes("json")) {
      try { pretty = JSON.stringify(JSON.parse(text), null, 2); } catch { /* show raw */ }
    }
    const headers = [...res.headers];
    const download = () => {
      const a = h("a", {
        href: URL.createObjectURL(new Blob([text], { type: ctype || "text/plain" })),
        download: "response" + (ctype.includes("json") ? ".json" : ".txt"),
      });
      a.click();
      setTimeout(() => URL.revokeObjectURL(a.href), 1000);
    };
    set(out,
      h("div", { class: "result-head" },
        h("span", { class: "status s" + String(res.status)[0] }, (res.status + " " + res.statusText).trim()),
        h("span", { class: "muted" }, Math.round(ms) + " ms · " + formatBytes(new Blob([text]).size)),
        h("span", { class: "spacer" }),
        text && h("button", { type: "button", class: "link-btn", onclick: download }, "Download")),
      h("div", { class: "request-url" },
        h("span", { class: "muted small" }, "Request URL"),
        h("code", null, absolute(r.url))),
      h("pre", { class: "code res-body" }, pretty || h("span", { class: "muted" }, "(empty body)")),
      h("details", { class: "res-headers" },
        h("summary", null, "Response headers (" + headers.length + ")"),
        h("table", { class: "headers" }, headers.map(([k, v]) =>
          h("tr", null, h("td", null, h("code", null, k)), h("td", null, v))))));
  }

  // ---------- startup ----------

  function collect(spec) {
    const list = [];
    for (const path of Object.keys(spec.paths || {}).sort()) {
      const item = spec.paths[path];
      for (const m of METHOD_ORDER) {
        const op = item[m];
        if (!op) continue;
        list.push({
          method: m,
          path,
          op,
          params: [...(item.parameters || []), ...(op.parameters || [])],
          tag: (op.tags && op.tags[0]) || "default",
          hash: "#" + m + path,
        });
      }
    }
    return list;
  }

  async function init() {
    try {
      const res = await fetch(specURL);
      if (!res.ok) throw new Error(res.status + " " + res.statusText);
      state.spec = await res.json();
    } catch (err) {
      set(main, h("div", { class: "empty" },
        h("h2", null, "Could not load the API spec"),
        h("p", null, h("code", null, specURL), ": " + err.message)));
      return;
    }
    state.ops = collect(state.spec);
    const servers = state.spec.servers || [];
    if (servers.length) state.server = servers[0].url.replace(/\/$/, "");

    renderHeader();
    search.addEventListener("input", renderNav);
    document.addEventListener("keydown", (e) => {
      const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName);
      if (e.key === "/" && !typing) {
        e.preventDefault();
        search.focus();
      }
    });
    window.addEventListener("hashchange", route);
    route();
  }

  init();
})();
