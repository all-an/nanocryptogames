// wallet.js — handles the "Create Wallet" button and seed modal on the landing page.
// Fetches POST /wallet/create, then renders a modal with the seed and copy button.
(function () {
  'use strict';

  // SVG icon: two overlapping pages (clipboard copy)
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

  // buildModal constructs the overlay DOM node from the generated wallet data.
  function buildModal(seed, address) {
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

  // showModal renders the modal and wires up copy + close behaviour.
  function showModal(seed, address) {
    const overlay = buildModal(seed, address);
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
        // Fallback: select the text so the user can Ctrl+C manually
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
    });

    // Dismiss on backdrop click (outside the card).
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) {
        overlay.remove();
      }
    });

    // Dismiss on Escape.
    function onKeyDown(e) {
      if (e.key === 'Escape') {
        overlay.remove();
        document.removeEventListener('keydown', onKeyDown);
      }
    }
    document.addEventListener('keydown', onKeyDown);
  }

  // createWallet calls the server, handles loading state, and shows the modal.
  function createWallet(btn) {
    const originalText = btn.textContent;
    btn.disabled = true;
    btn.textContent = 'Creating…';

    fetch('/wallet/create', { method: 'POST' })
      .then(function (res) {
        if (!res.ok) throw new Error('server error ' + res.status);
        return res.json();
      })
      .then(function (data) {
        showModal(data.seed, data.address);
      })
      .catch(function (err) {
        console.error('wallet create:', err);
        alert('Could not generate wallet. Please try again.');
      })
      .finally(function () {
        btn.disabled = false;
        btn.textContent = originalText;
      });
  }

  // Wire up all elements with data-action="create-wallet" once the DOM is ready.
  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('[data-action="create-wallet"]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        createWallet(btn);
      });
    });
  });
}());
