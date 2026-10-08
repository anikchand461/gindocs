// gindocs website: copy buttons and example tabs.
(() => {
  "use strict";

  // Copy buttons: data-copy holds the text, or data-copy-from names an
  // element whose text is copied.
  document.querySelectorAll(".copy").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const from = btn.dataset.copyFrom && document.querySelector(btn.dataset.copyFrom);
      const text = from ? from.textContent : btn.dataset.copy;
      const label = btn.textContent;
      try {
        await navigator.clipboard.writeText(text);
        btn.textContent = "Copied!";
        btn.classList.add("done");
      } catch {
        btn.textContent = "Press ⌘C";
      }
      setTimeout(() => {
        btn.textContent = label;
        btn.classList.remove("done");
      }, 1400);
    });
  });

  // Mobile menu: the ☰ button opens the section links below the header.
  const menuBtn = document.querySelector(".menu-btn");
  const menu = document.getElementById("site-menu");
  const wide = matchMedia("(min-width: 901px)");
  const setMenu = (open, returnFocus) => {
    document.body.classList.toggle("menu-open", open);
    menuBtn.setAttribute("aria-expanded", String(open));
    menuBtn.setAttribute("aria-label", open ? "Close menu" : "Open menu");
    if (open) menu.querySelector("a").focus();
    else if (returnFocus) menuBtn.focus();
  };
  const isOpen = () => document.body.classList.contains("menu-open");
  menuBtn.addEventListener("click", () => setMenu(!isOpen(), true));
  menu.addEventListener("click", (e) => {
    if (e.target.closest("a") && isOpen()) setMenu(false);
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && isOpen()) setMenu(false, true);
  });
  document.addEventListener("click", (e) => {
    if (isOpen() && !e.target.closest(".nav")) setMenu(false);
  });
  wide.addEventListener("change", () => { if (wide.matches) setMenu(false); });

  // Tabs with arrow-key navigation (WAI-ARIA tabs pattern).
  document.querySelectorAll("[data-tabs]").forEach((root) => {
    const tabs = [...root.querySelectorAll('[role="tab"]')];
    const select = (tab, focus) => {
      tabs.forEach((t) => {
        const on = t === tab;
        t.setAttribute("aria-selected", String(on));
        t.tabIndex = on ? 0 : -1;
        document.getElementById(t.getAttribute("aria-controls")).hidden = !on;
      });
      if (focus) tab.focus();
    };
    tabs.forEach((tab, i) => {
      tab.addEventListener("click", () => select(tab, false));
      tab.addEventListener("keydown", (e) => {
        const step = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
        if (step) {
          e.preventDefault();
          select(tabs[(i + step + tabs.length) % tabs.length], true);
        }
      });
    });
  });
})();
