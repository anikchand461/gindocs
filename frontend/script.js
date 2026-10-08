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
