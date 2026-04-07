// shooter-code-flow.js — tab switching + step detail expand/collapse

document.addEventListener('DOMContentLoaded', () => {

  // ── Tab switching ────────────────────────────────────────────────────────
  const tabBtns  = document.querySelectorAll('.tab-btn');
  const panels   = document.querySelectorAll('.flow-panel');

  tabBtns.forEach((btn) => {
    btn.addEventListener('click', () => {
      const target = btn.dataset.tab;
      tabBtns.forEach((b) => b.classList.toggle('active', b === btn));
      panels.forEach((p) => p.classList.toggle('active', p.id === target));
    });
  });

  // ── Step row expand/collapse ─────────────────────────────────────────────
  document.querySelectorAll('.step-row').forEach((row) => {
    const detail = row.nextElementSibling;
    if (!detail || !detail.classList.contains('detail-panel')) return;

    row.addEventListener('click', () => {
      const isOpen = detail.classList.contains('open');
      // Close all first
      document.querySelectorAll('.detail-panel.open').forEach((d) => d.classList.remove('open'));
      document.querySelectorAll('.step-row.expanded').forEach((r) => r.classList.remove('expanded'));
      // Toggle clicked one
      if (!isOpen) {
        detail.classList.add('open');
        row.classList.add('expanded');
        // Smooth scroll into view
        setTimeout(() => detail.scrollIntoView({ behavior: 'smooth', block: 'nearest' }), 50);
      }
    });
  });

  // ── Step counter badges ──────────────────────────────────────────────────
  document.querySelectorAll('.flow-panel').forEach((panel) => {
    panel.querySelectorAll('.step-row').forEach((row, i) => {
      const num = document.createElement('span');
      num.textContent = i + 1;
      num.style.cssText =
        'position:absolute;top:6px;left:6px;font-size:0.65rem;color:#4a3a6a;font-weight:700;z-index:3;';
      row.style.position = 'relative';
      row.appendChild(num);
    });
  });

});
