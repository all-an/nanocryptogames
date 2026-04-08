// wallet-page.js — logic for the /wallet page.
// Reads seed + address from sessionStorage set by wallet.js on the landing page.
(function () {
  'use strict';

  var seed    = sessionStorage.getItem('wallet_seed');
  var address = sessionStorage.getItem('wallet_address');

  var elLoading  = document.getElementById('wp-loading');
  var elNoWallet = document.getElementById('wp-no-wallet');
  var elMain     = document.getElementById('wp-main');

  if (!seed || !address) {
    elLoading.style.display  = 'none';
    elNoWallet.style.display = '';
    return;
  }

  // Show main UI
  elLoading.style.display = 'none';
  elMain.style.display    = '';
  document.getElementById('wp-address').textContent = address;

  // ── QR code modal ─────────────────────────────────────────────────────────

  function showQRModal() {
    var overlay = document.createElement('div');
    overlay.className = 'wp-qr-overlay';

    overlay.innerHTML =
      '<div class="wp-qr-modal">' +
        '<p class="wp-qr-title">Receive XNO</p>' +
        '<img class="wp-qr-img" src="https://api.qrserver.com/v1/create-qr-code/?size=220x220&margin=12&data=nano:' + encodeURIComponent(address) + '" alt="QR code" width="220" height="220">' +
        '<p class="wp-qr-address">' + address + '</p>' +
        '<button class="wp-btn wp-btn--secondary wp-qr-close" type="button">Close</button>' +
      '</div>';

    document.body.appendChild(overlay);

    overlay.querySelector('.wp-qr-close').addEventListener('click', function () {
      overlay.remove();
    });

    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) overlay.remove();
    });

    function onKey(e) {
      if (e.key === 'Escape') {
        overlay.remove();
        document.removeEventListener('keydown', onKey);
      }
    }
    document.addEventListener('keydown', onKey);
  }

  document.getElementById('wp-qr-btn').addEventListener('click', showQRModal);

  // ── Copy address ──────────────────────────────────────────────────────────
  document.getElementById('wp-copy-addr').addEventListener('click', function () {
    var btn = this;
    navigator.clipboard.writeText(address).then(function () {
      btn.style.color = '#52c07a';
      setTimeout(function () { btn.style.color = ''; }, 1500);
    });
  });

  // ── Balance ───────────────────────────────────────────────────────────────
  function fetchBalance() {
    var btn = document.getElementById('wp-refresh-btn');
    btn.disabled = true;
    btn.style.opacity = '0.5';

    fetch('/wallet/balance', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ address: address }),
    })
      .then(function (r) { return r.json(); })
      .then(function (d) {
        if (d.error) {
          document.getElementById('wp-balance').textContent = 'error';
          return;
        }
        document.getElementById('wp-balance').textContent = d.balance_xno;
        var pendingEl = document.getElementById('wp-pending');
        if (d.pending_xno && d.pending_xno !== '0') {
          pendingEl.textContent = '+ ' + d.pending_xno + ' XNO pending';
          pendingEl.style.display = '';
        } else {
          pendingEl.style.display = 'none';
        }
      })
      .catch(function () {
        document.getElementById('wp-balance').textContent = 'error';
      })
      .finally(function () {
        btn.disabled = false;
        btn.style.opacity = '';
      });
  }

  fetchBalance();
  document.getElementById('wp-refresh-btn').addEventListener('click', fetchBalance);

  // ── Receive ───────────────────────────────────────────────────────────────
  document.getElementById('wp-receive-btn').addEventListener('click', function () {
    var btn  = this;
    var msg  = document.getElementById('wp-receive-msg');
    btn.disabled   = true;
    btn.textContent = 'Checking…';
    msg.textContent = '';
    msg.className   = 'wp-msg';

    fetch('/wallet/receive', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ seed: seed }),
    })
      .then(function (r) { return r.json(); })
      .then(function (d) {
        if (d.error) {
          msg.textContent = d.error;
          msg.classList.add('wp-msg--error');
        } else if (d.received === 0) {
          msg.textContent = 'No pending transactions.';
        } else {
          msg.textContent = 'Received ' + d.received + ' transaction' + (d.received > 1 ? 's' : '') + '.';
          msg.classList.add('wp-msg--ok');
          fetchBalance();
        }
      })
      .catch(function () {
        msg.textContent = 'Request failed. Please try again.';
        msg.classList.add('wp-msg--error');
      })
      .finally(function () {
        btn.disabled    = false;
        btn.textContent = 'Check for incoming';
      });
  });

  // ── Send ──────────────────────────────────────────────────────────────────
  document.getElementById('wp-send-form').addEventListener('submit', function (e) {
    e.preventDefault();
    var btn    = document.getElementById('wp-send-btn');
    var msg    = document.getElementById('wp-send-msg');
    var to     = document.getElementById('wp-send-to').value.trim();
    var amount = document.getElementById('wp-send-amount').value.trim();

    msg.textContent = '';
    msg.className   = 'wp-msg';
    btn.disabled    = true;
    btn.textContent = 'Sending…';

    fetch('/wallet/send', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ seed: seed, to: to, amount_xno: amount }),
    })
      .then(function (r) { return r.json(); })
      .then(function (d) {
        if (d.error) {
          msg.textContent = d.error;
          msg.classList.add('wp-msg--error');
        } else {
          msg.textContent = 'Sent! Block: ' + d.hash.slice(0, 16) + '…';
          msg.classList.add('wp-msg--ok');
          document.getElementById('wp-send-to').value     = '';
          document.getElementById('wp-send-amount').value = '';
          fetchBalance();
        }
      })
      .catch(function () {
        msg.textContent = 'Request failed. Please try again.';
        msg.classList.add('wp-msg--error');
      })
      .finally(function () {
        btn.disabled    = false;
        btn.textContent = 'Send';
      });
  });
}());
