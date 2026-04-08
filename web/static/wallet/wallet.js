// wallet.js — wallet entry modal and seed display for the landing page.
(function () {
  'use strict';

  const COPY_ICON = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none"
      stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
      aria-hidden="true">
    <rect x="9" y="9" width="13" height="13" rx="2"/>
    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
  </svg>`;

  const CHECK_ICON = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none"
      stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"
      aria-hidden="true">
    <polyline points="20 6 9 17 4 12"/>
  </svg>`;

  // ── Choice modal (shown first when player clicks "Create Wallet") ──────────

  function buildChoiceModal() {
    const overlay = document.createElement('div');
    overlay.className = 'wallet-overlay';
    overlay.setAttribute('role', 'dialog');
    overlay.setAttribute('aria-modal', 'true');
    overlay.setAttribute('aria-label', 'Wallet options');

    overlay.innerHTML = `
      <div class="wallet-modal">
        <div class="wallet-modal-icon">💳</div>
        <h2 class="wallet-modal-title">Nano Wallet</h2>
        <p class="wallet-modal-subtitle">Create a new wallet or import an existing one.</p>

        <div class="wallet-modal-warning">
          ⚠ <strong>Before you create a wallet:</strong> a 64-character seed will be
          generated and shown to you <em>once</em>. You must save it in a secure place —
          it is the only way to recover your funds. We never store it.
        </div>

        <div class="wallet-choice-actions">
          <button class="wallet-choice-btn wallet-choice-btn--create" id="wallet-choice-create" type="button">
            Create Wallet
          </button>
          <button class="wallet-choice-btn wallet-choice-btn--import" id="wallet-choice-import" type="button">
            Import Wallet
          </button>
        </div>

        <button class="wallet-modal-cancel" id="wallet-choice-cancel" type="button">Cancel</button>
      </div>
    `;

    return overlay;
  }

  function showChoiceModal() {
    const overlay = buildChoiceModal();
    document.body.appendChild(overlay);

    overlay.querySelector('#wallet-choice-create').addEventListener('click', function () {
      overlay.remove();
      callCreateWallet();
    });

    overlay.querySelector('#wallet-choice-import').addEventListener('click', function () {
      overlay.remove();
      showImportModal();
    });

    overlay.querySelector('#wallet-choice-cancel').addEventListener('click', function () {
      overlay.remove();
    });

    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) overlay.remove();
    });

    function onKeyDown(e) {
      if (e.key === 'Escape') {
        overlay.remove();
        document.removeEventListener('keydown', onKeyDown);
      }
    }
    document.addEventListener('keydown', onKeyDown);
  }

  // ── Import wallet modal ────────────────────────────────────────────────────

  function buildImportModal() {
    const overlay = document.createElement('div');
    overlay.className = 'wallet-overlay';
    overlay.setAttribute('role', 'dialog');
    overlay.setAttribute('aria-modal', 'true');
    overlay.setAttribute('aria-label', 'Import wallet');

    overlay.innerHTML = `
      <div class="wallet-modal">
        <div class="wallet-modal-icon">🔑</div>
        <h2 class="wallet-modal-title">Import Wallet</h2>
        <p class="wallet-modal-subtitle">Enter your 64-character hex seed to restore your wallet.</p>

        <div class="wallet-modal-warning">
          ⚠ <strong>Never share your seed.</strong> Only enter it on a device you trust.
          Your seed stays in your browser and is never sent to our servers.
        </div>

        <label class="wallet-modal-label" for="wallet-import-seed">Seed (64 hex characters)</label>
        <div class="wallet-import-row">
          <input
            class="wallet-import-input"
            id="wallet-import-seed"
            type="text"
            maxlength="64"
            placeholder="0000…"
            spellcheck="false"
            autocomplete="off"
          >
        </div>
        <p class="wallet-import-err" id="wallet-import-err"></p>

        <div class="wallet-choice-actions" style="margin-top:16px;">
          <button class="wallet-choice-btn wallet-choice-btn--create" id="wallet-import-submit" type="button">
            Import
          </button>
          <button class="wallet-choice-btn wallet-choice-btn--import" id="wallet-import-cancel" type="button">
            Cancel
          </button>
        </div>
      </div>
    `;

    return overlay;
  }

  function showImportModal() {
    const overlay = buildImportModal();
    document.body.appendChild(overlay);

    const input   = overlay.querySelector('#wallet-import-seed');
    const errEl   = overlay.querySelector('#wallet-import-err');
    const submitBtn = overlay.querySelector('#wallet-import-submit');

    overlay.querySelector('#wallet-import-cancel').addEventListener('click', function () {
      overlay.remove();
    });

    submitBtn.addEventListener('click', function () {
      const seed = input.value.trim();
      errEl.textContent = '';

      if (!/^[0-9a-fA-F]{64}$/.test(seed)) {
        errEl.textContent = 'Seed must be exactly 64 hex characters.';
        return;
      }

      submitBtn.disabled    = true;
      submitBtn.textContent = 'Importing…';

      fetch('/wallet/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ seed: seed }),
      })
        .then(function (r) { return r.json(); })
        .then(function (d) {
          if (d.error) {
            errEl.textContent = d.error;
            submitBtn.disabled    = false;
            submitBtn.textContent = 'Import';
            return;
          }
          sessionStorage.setItem('wallet_seed', seed);
          sessionStorage.setItem('wallet_address', d.address);
          overlay.remove();
          window.location.href = '/wallet';
        })
        .catch(function () {
          errEl.textContent     = 'Request failed. Please try again.';
          submitBtn.disabled    = false;
          submitBtn.textContent = 'Import';
        });
    });

    // Submit on Enter
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') submitBtn.click();
    });

    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) overlay.remove();
    });

    function onKeyDown(e) {
      if (e.key === 'Escape') {
        overlay.remove();
        document.removeEventListener('keydown', onKeyDown);
      }
    }
    document.addEventListener('keydown', onKeyDown);
    setTimeout(function () { input.focus(); }, 50);
  }

  // ── Seed modal (shown after wallet is created) ─────────────────────────────

  function buildSeedModal(seed, address) {
    const overlay = document.createElement('div');
    overlay.className = 'wallet-overlay';
    overlay.setAttribute('role', 'dialog');
    overlay.setAttribute('aria-modal', 'true');
    overlay.setAttribute('aria-label', 'New wallet created');

    overlay.innerHTML = `
      <div class="wallet-modal">
        <div class="wallet-modal-icon">💳</div>
        <h2 class="wallet-modal-title">Your New Nano Wallet</h2>
        <p class="wallet-modal-subtitle">Generated locally — never stored on our servers.</p>

        <div class="wallet-modal-warning">
          ⚠ <strong>Save your seed now.</strong> This is the only time it will be shown.
          Anyone with this seed controls your wallet. Store it somewhere safe and private.
        </div>

        <p class="wallet-modal-label">Seed (64 hex characters)</p>
        <div class="wallet-seed-row">
          <div class="wallet-seed-box" id="wallet-seed-text">${seed}</div>
          <button class="wallet-copy-btn" id="wallet-copy-btn" title="Copy seed" type="button">
            ${COPY_ICON}
          </button>
        </div>

        <p class="wallet-modal-label">Address (index 0)</p>
        <div class="wallet-address-box">${address}</div>

        <button class="wallet-modal-close" id="wallet-modal-close" type="button">
          I have saved my seed — close
        </button>
      </div>
    `;

    return overlay;
  }

  function showSeedModal(seed, address) {
    const overlay = buildSeedModal(seed, address);
    document.body.appendChild(overlay);

    const copyBtn = overlay.querySelector('#wallet-copy-btn');
    copyBtn.addEventListener('click', function () {
      navigator.clipboard.writeText(seed).then(function () {
        copyBtn.innerHTML = CHECK_ICON;
        copyBtn.classList.add('copied');
        setTimeout(function () {
          copyBtn.innerHTML = COPY_ICON;
          copyBtn.classList.remove('copied');
        }, 2000);
      }).catch(function () {
        const box = overlay.querySelector('#wallet-seed-text');
        const range = document.createRange();
        range.selectNodeContents(box);
        const sel = window.getSelection();
        sel.removeAllRanges();
        sel.addRange(range);
      });
    });

    overlay.querySelector('#wallet-modal-close').addEventListener('click', function () {
      overlay.remove();
      window.location.href = '/wallet';
    });

    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) overlay.remove();
    });

    function onKeyDown(e) {
      if (e.key === 'Escape') {
        overlay.remove();
        document.removeEventListener('keydown', onKeyDown);
      }
    }
    document.addEventListener('keydown', onKeyDown);
  }

  // ── API call ───────────────────────────────────────────────────────────────

  function callCreateWallet() {
    fetch('/wallet/create', { method: 'POST' })
      .then(function (res) {
        if (!res.ok) throw new Error('server error ' + res.status);
        return res.json();
      })
      .then(function (data) {
        sessionStorage.setItem('wallet_seed', data.seed);
        sessionStorage.setItem('wallet_address', data.address);
        showSeedModal(data.seed, data.address);
      })
      .catch(function (err) {
        console.error('wallet create:', err);
        alert('Could not generate wallet. Please try again.');
      });
  }

  // ── Wire up landing page button ────────────────────────────────────────────

  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('[data-action="create-wallet"]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        showChoiceModal();
      });
    });
  });
}());
