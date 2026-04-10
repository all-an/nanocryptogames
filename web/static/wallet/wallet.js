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

  // ── Translations ───────────────────────────────────────────────────────────

  const i18n = {
    en: {
      // Choice modal
      choiceTitle:    'Nano Wallet',
      choiceSubtitle: 'Create a new wallet or import an existing one.',
      choiceWarning:  '<strong>Before you create a wallet:</strong> a 64-character seed will be generated and shown to you <em>once</em>. You must save it in a secure place — it is the only way to recover your funds. We never store it.',
      choiceCreate:   'Create Wallet',
      choiceImport:   'Import Wallet',
      choiceCancel:   'Cancel',
      choiceAriaLabel: 'Wallet options',
      // Import modal
      importTitle:    'Import Wallet',
      importSubtitle: 'Enter your 64-character hex seed to restore your wallet.',
      importWarning:  '<strong>Never share your seed.</strong> Only enter it on a device you trust. Your seed stays in your browser and is never sent to our servers.',
      importLabel:    'Seed (64 hex characters)',
      importSubmit:   'Import',
      importCancel:   'Cancel',
      importingBtn:   'Importing\u2026',
      importErrFmt:   'Seed must be exactly 64 hex characters.',
      importErrFail:  'Request failed. Please try again.',
      importAriaLabel: 'Import wallet',
      // Seed modal
      seedTitle:      'Your New Nano Wallet',
      seedSubtitle:   'Generated locally \u2014 never stored on our servers.',
      seedWarning:    '<strong>Save your seed now.</strong> This is the only time it will be shown. Anyone with this seed controls your wallet. Store it somewhere safe and private.',
      seedLabel:      'Seed (64 hex characters)',
      seedAddrLabel:  'Address (index 0)',
      seedClose:      'I have saved my seed \u2014 close',
      seedAriaLabel:  'New wallet created',
      // Errors
      createFail:     'Could not generate wallet. Please try again.',
    },
    pt: {
      // Choice modal
      choiceTitle:    'Nano Carteira',
      choiceSubtitle: 'Crie uma nova carteira ou importe uma existente.',
      choiceWarning:  '<strong>Antes de criar uma carteira:</strong> uma seed de 64 caracteres será gerada e exibida <em>uma única vez</em>. Você deve salvá-la em um lugar seguro — é a única forma de recuperar seus fundos. Nós nunca a armazenamos.',
      choiceCreate:   'Criar Carteira',
      choiceImport:   'Importar Carteira',
      choiceCancel:   'Cancelar',
      choiceAriaLabel: 'Opções de carteira',
      // Import modal
      importTitle:    'Importar Carteira',
      importSubtitle: 'Digite sua seed hexadecimal de 64 caracteres para restaurar sua carteira.',
      importWarning:  '<strong>Nunca compartilhe sua seed.</strong> Digite-a apenas em um dispositivo de confiança. Sua seed fica no seu navegador e nunca é enviada aos nossos servidores.',
      importLabel:    'Seed (64 caracteres hexadecimais)',
      importSubmit:   'Importar',
      importCancel:   'Cancelar',
      importingBtn:   'Importando\u2026',
      importErrFmt:   'A seed deve ter exatamente 64 caracteres hexadecimais.',
      importErrFail:  'Falha na requisição. Tente novamente.',
      importAriaLabel: 'Importar carteira',
      // Seed modal
      seedTitle:      'Sua Nova Nano Carteira',
      seedSubtitle:   'Gerada localmente \u2014 nunca armazenada em nossos servidores.',
      seedWarning:    '<strong>Salve sua seed agora.</strong> Esta é a única vez que ela será exibida. Quem tiver esta seed controla sua carteira. Guarde-a em um lugar seguro e privado.',
      seedLabel:      'Seed (64 caracteres hexadecimais)',
      seedAddrLabel:  'Endereço (índice 0)',
      seedClose:      'Já salvei minha seed \u2014 fechar',
      seedAriaLabel:  'Nova carteira criada',
      // Errors
      createFail:     'Não foi possível gerar a carteira. Tente novamente.',
    },
  };

  function t(key) {
    const lang = localStorage.getItem('ncgLang') === 'pt' ? 'pt' : 'en';
    return i18n[lang][key] || i18n.en[key];
  }

  // ── Choice modal (shown first when player clicks "Create Wallet") ──────────

  function buildChoiceModal() {
    const overlay = document.createElement('div');
    overlay.className = 'wallet-overlay';
    overlay.setAttribute('role', 'dialog');
    overlay.setAttribute('aria-modal', 'true');
    overlay.setAttribute('aria-label', t('choiceAriaLabel'));

    overlay.innerHTML = `
      <div class="wallet-modal">
        <div class="wallet-modal-icon">💳</div>
        <h2 class="wallet-modal-title">${t('choiceTitle')}</h2>
        <p class="wallet-modal-subtitle">${t('choiceSubtitle')}</p>

        <div class="wallet-modal-warning">
          ⚠ ${t('choiceWarning')}
        </div>

        <div class="wallet-choice-actions">
          <button class="wallet-choice-btn wallet-choice-btn--create" id="wallet-choice-create" type="button">
            ${t('choiceCreate')}
          </button>
          <button class="wallet-choice-btn wallet-choice-btn--import" id="wallet-choice-import" type="button">
            ${t('choiceImport')}
          </button>
        </div>

        <button class="wallet-modal-cancel" id="wallet-choice-cancel" type="button">${t('choiceCancel')}</button>
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
    overlay.setAttribute('aria-label', t('importAriaLabel'));

    overlay.innerHTML = `
      <div class="wallet-modal">
        <div class="wallet-modal-icon">🔑</div>
        <h2 class="wallet-modal-title">${t('importTitle')}</h2>
        <p class="wallet-modal-subtitle">${t('importSubtitle')}</p>

        <div class="wallet-modal-warning">
          ⚠ ${t('importWarning')}
        </div>

        <label class="wallet-modal-label" for="wallet-import-seed">${t('importLabel')}</label>
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
            ${t('importSubmit')}
          </button>
          <button class="wallet-choice-btn wallet-choice-btn--import" id="wallet-import-cancel" type="button">
            ${t('importCancel')}
          </button>
        </div>
      </div>
    `;

    return overlay;
  }

  function showImportModal() {
    const overlay = buildImportModal();
    document.body.appendChild(overlay);

    const input     = overlay.querySelector('#wallet-import-seed');
    const errEl     = overlay.querySelector('#wallet-import-err');
    const submitBtn = overlay.querySelector('#wallet-import-submit');

    overlay.querySelector('#wallet-import-cancel').addEventListener('click', function () {
      overlay.remove();
    });

    submitBtn.addEventListener('click', function () {
      const seed = input.value.trim();
      errEl.textContent = '';

      if (!/^[0-9a-fA-F]{64}$/.test(seed)) {
        errEl.textContent = t('importErrFmt');
        return;
      }

      submitBtn.disabled    = true;
      submitBtn.textContent = t('importingBtn');

      fetch('/wallet/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ seed: seed }),
      })
        .then(function (r) { return r.json(); })
        .then(function (d) {
          if (d.error) {
            errEl.textContent     = d.error;
            submitBtn.disabled    = false;
            submitBtn.textContent = t('importSubmit');
            return;
          }
          sessionStorage.setItem('wallet_seed', seed);
          sessionStorage.setItem('wallet_address', d.address);
          overlay.remove();
          window.location.href = '/wallet';
        })
        .catch(function () {
          errEl.textContent     = t('importErrFail');
          submitBtn.disabled    = false;
          submitBtn.textContent = t('importSubmit');
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
    overlay.setAttribute('aria-label', t('seedAriaLabel'));

    overlay.innerHTML = `
      <div class="wallet-modal">
        <div class="wallet-modal-icon">💳</div>
        <h2 class="wallet-modal-title">${t('seedTitle')}</h2>
        <p class="wallet-modal-subtitle">${t('seedSubtitle')}</p>

        <div class="wallet-modal-warning">
          ⚠ ${t('seedWarning')}
        </div>

        <p class="wallet-modal-label">${t('seedLabel')}</p>
        <div class="wallet-seed-row">
          <div class="wallet-seed-box" id="wallet-seed-text">${seed}</div>
          <button class="wallet-copy-btn" id="wallet-copy-btn" title="Copy seed" type="button">
            ${COPY_ICON}
          </button>
        </div>

        <p class="wallet-modal-label">${t('seedAddrLabel')}</p>
        <div class="wallet-address-box">${address}</div>

        <button class="wallet-modal-close" id="wallet-modal-close" type="button">
          ${t('seedClose')}
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
        alert(t('createFail'));
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
