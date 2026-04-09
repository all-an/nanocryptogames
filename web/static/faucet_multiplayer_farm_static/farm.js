// farm.js — Nano Faucet Multiplayer Farm client
'use strict';

const GRID_W   = 20;
const GRID_H   = 15;
const CELL_W   = 40;
const CELL_H   = 40;

const canvas   = document.getElementById('farm-canvas');
const ctx      = canvas.getContext('2d');
let   username = canvas.dataset.username; // mutable — updated when player renames
const room     = canvas.dataset.room;
// Parse room as "col,row" 2-D grid coordinates (e.g. "0,0", "1,-2").
function parseGrid(r) {
  const [c, w] = r.split(',').map(Number);
  return { col: isNaN(c) ? 0 : c, row: isNaN(w) ? 0 : w };
}
const { col: gridCol, row: gridRow } = parseGrid(room);
const entryX   = parseInt(canvas.dataset.entryX, 10);
const entryY   = parseInt(canvas.dataset.entryY, 10);

// State
let myID      = null;
let myColor   = '#4A90D9';
let players   = {};   // id → {id, username, x, y, color}
let blocks    = [];   // [[x,y], …] impassable cells for this grid
let doors     = [];   // [[x,y], …] passable door cells for this grid
let connected = false;

// ── WebSocket ──────────────────────────────────────────────────────────────

function wsURL() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  let url = `${proto}://${location.host}/farm/ws?room=${encodeURIComponent(room)}`;
  if (entryX >= 0) url += `&ex=${entryX}`;
  if (entryY >= 0) url += `&ey=${entryY}`;
  return url;
}

let ws;
function connect() {
  ws = new WebSocket(wsURL());

  ws.onopen = () => {
    connected = true;
    appendSystem('Connected to ' + room);
  };

  ws.onclose = () => {
    connected = false;
    appendSystem('Disconnected — reconnecting in 3s…');
    setTimeout(connect, 3000);
  };

  ws.onerror = () => ws.close();

  ws.onmessage = (e) => {
    try {
      handleMessage(JSON.parse(e.data));
    } catch (_) {}
  };
}

function send(obj) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(obj));
  }
}

// ── Message handling ───────────────────────────────────────────────────────

function handleMessage(msg) {
  switch (msg.type) {
    case 'init':
      myID    = msg.id;
      myColor = msg.color;
      blocks  = msg.blocks || [];
      doors   = msg.doors  || [];
      appendSystem(`Welcome, ${msg.username}! You are in ${msg.room}.`);
      break;

    case 'state':
      players = {};
      for (const p of (msg.players || [])) {
        players[p.id] = p;
      }
      render();
      updateOnlineList();
      break;

    case 'chat':
      appendChat(msg.from, msg.text);
      break;

    case 'system':
      appendSystem(msg.text);
      break;

    case 'dm':
      appendDM(msg.from, msg.to, msg.text);
      break;
  }
}

// ── Rendering ──────────────────────────────────────────────────────────────

function render() {
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  drawGrid();
  drawEldar();         // NPC beneath players
  drawPlayers();
  drawEldarBubble();   // speech bubble always on top
}

function isBlockCell(x, y) {
  return blocks.some(b => b[0] === x && b[1] === y);
}

function isDoorCell(x, y) {
  return doors.some(d => d[0] === x && d[1] === y);
}

function drawGrid() {
  for (let y = 0; y < GRID_H; y++) {
    for (let x = 0; x < GRID_W; x++) {
      const px = x * CELL_W;
      const py = y * CELL_H;
      const w  = CELL_W - 1;
      const h  = CELL_H - 1;

      if (isBlockCell(x, y)) {
        drawBrick(px, py, w, h);
      } else if (isDoorCell(x, y)) {
        drawDoor(px, py, w, h);
      } else {
        const cellNum = (gridRow * 1000 + gridCol) * GRID_W * GRID_H + y * GRID_W + x + 1;
        ctx.fillStyle = '#0f0f22';
        ctx.fillRect(px, py, w, h);
        ctx.fillStyle = '#4a4a80';
        ctx.font = '9px monospace';
        ctx.textAlign = 'left';
        ctx.fillText(cellNum, px + 2, py + 10);
      }
    }
  }
}

function drawBrick(px, py, w, h) {
  // Base fill
  ctx.fillStyle = '#1a0a00';
  ctx.fillRect(px, py, w, h);

  // Brick rows
  const bH = 8, bW = 17;
  ctx.fillStyle = '#7a3a10';
  for (let row = 0; row * (bH + 1) < h; row++) {
    const by = py + row * (bH + 1);
    const offset = (row % 2) * Math.floor(bW / 2);
    for (let bx = px - offset; bx < px + w; bx += bW + 1) {
      const rx = Math.max(bx, px);
      const rw = Math.min(bx + bW, px + w) - rx;
      if (rw > 0) ctx.fillRect(rx + 1, by + 1, rw - 1, bH - 1);
    }
  }

  // Glowing orange border
  ctx.save();
  ctx.shadowColor   = '#ff6600';
  ctx.shadowBlur    = 10;
  ctx.strokeStyle   = '#ff8833';
  ctx.lineWidth     = 2;
  ctx.strokeRect(px + 1, py + 1, w - 2, h - 2);
  ctx.restore();
}

function drawDoor(px, py, w, h) {
  // Same brick background as solid blocks
  drawBrick(px, py, w, h);

  // Door dimensions
  const dw = 14;                          // door width
  const dh = 24;                          // total door height (arch + rect body)
  const dr = dw / 2;                      // arch radius
  const dx = px + Math.floor((w - dw) / 2);  // door left edge
  const dy = py + h - dh - 1;            // door top (sits at cell bottom)
  const ax = dx + dr;                     // arch centre x
  const ay = dy + dr;                     // arch centre y

  // Dark interior (arch top + rectangular body)
  ctx.beginPath();
  ctx.arc(ax, ay, dr, Math.PI, 0);       // semicircular arch
  ctx.lineTo(dx + dw, dy + dh);
  ctx.lineTo(dx, dy + dh);
  ctx.closePath();
  ctx.fillStyle = '#06060f';
  ctx.fill();

  // Golden arch (glowing)
  ctx.save();
  ctx.shadowColor = '#f0c040';
  ctx.shadowBlur  = 8;
  ctx.strokeStyle = '#d4a843';
  ctx.lineWidth   = 1.5;
  ctx.beginPath();
  ctx.arc(ax, ay, dr, Math.PI, 0);
  ctx.stroke();
  ctx.restore();

  // Door frame sides and sill
  ctx.save();
  ctx.strokeStyle = '#a07820';
  ctx.lineWidth   = 1.5;
  ctx.beginPath();
  ctx.moveTo(dx,      ay);
  ctx.lineTo(dx,      dy + dh);
  ctx.lineTo(dx + dw, dy + dh);
  ctx.lineTo(dx + dw, ay);
  ctx.stroke();
  ctx.restore();
}

function drawPlayers() {
  for (const p of Object.values(players)) {
    const cx = p.x * CELL_W + CELL_W / 2;
    const cy = p.y * CELL_H + CELL_H / 2;
    const r  = 13;

    ctx.save();

    // Outer glow — colored halo for depth
    ctx.shadowColor = p.color;
    ctx.shadowBlur  = 10;

    // Drop shadow offset
    ctx.beginPath();
    ctx.arc(cx, cy + 3, r, 0, Math.PI * 2);
    ctx.fillStyle = 'rgba(0,0,0,0.5)';
    ctx.fill();

    // Player circle with glow still active
    ctx.beginPath();
    ctx.arc(cx, cy, r, 0, Math.PI * 2);
    ctx.fillStyle = p.color;
    ctx.fill();

    // White border — always visible on every player
    ctx.shadowBlur  = 0; // don't blur the stroke
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth   = p.id === myID ? 2.5 : 1.5;
    ctx.stroke();

    ctx.restore();

    // Nano ticker inside circle
    ctx.fillStyle = 'rgba(255,255,255,0.9)';
    ctx.font = 'bold 11px sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText('Ӿ', cx, cy);
    ctx.textBaseline = 'alphabetic';

    // Username label with subtle text shadow for readability
    ctx.save();
    ctx.shadowColor = 'rgba(0,0,0,0.8)';
    ctx.shadowBlur  = 4;
    ctx.fillStyle   = '#ffffff';
    ctx.font        = 'bold 9px sans-serif';
    ctx.textAlign   = 'center';
    ctx.fillText(p.username.slice(0, 10), cx, cy + r + 11);
    ctx.restore();
  }
}

// ── NPC: King Eldar Raiblock ───────────────────────────────────────────────
// Cell 172 in room "0,0" = col 12, row 9 (1-indexed) = x:11, y:8 (0-indexed).
// The speech bubble appears once the player reaches column 9 (me.x >= 8).

const ELDAR_ROOM = '0,0';
const ELDAR_X    = 11;
const ELDAR_Y    = 8;

/** Draws King Eldar at his throne cell. Renders beneath players. */
function drawEldar() {
  if (room !== ELDAR_ROOM) return;
  const cx = ELDAR_X * CELL_W + CELL_W / 2;
  const cy = ELDAR_Y * CELL_H + CELL_H / 2;
  const r  = 14;

  ctx.save();
  ctx.shadowColor = '#D4AF37';
  ctx.shadowBlur  = 14;

  // Drop shadow
  ctx.beginPath();
  ctx.arc(cx, cy + 3, r, 0, Math.PI * 2);
  ctx.fillStyle = 'rgba(0,0,0,0.5)';
  ctx.fill();

  // Royal gold body
  ctx.beginPath();
  ctx.arc(cx, cy, r, 0, Math.PI * 2);
  ctx.fillStyle = '#D4AF37';
  ctx.fill();

  // White border
  ctx.shadowBlur  = 0;
  ctx.strokeStyle = '#ffffff';
  ctx.lineWidth   = 2;
  ctx.stroke();
  ctx.restore();

  // Crown glyph
  ctx.fillStyle    = '#3a2a00';
  ctx.font         = 'bold 13px sans-serif';
  ctx.textAlign    = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText('♛', cx, cy);
  ctx.textBaseline = 'alphabetic';

  // Name label
  ctx.save();
  ctx.shadowColor = 'rgba(0,0,0,0.9)';
  ctx.shadowBlur  = 4;
  ctx.fillStyle   = '#D4AF37';
  ctx.font        = 'bold 9px sans-serif';
  ctx.textAlign   = 'center';
  ctx.fillText('Eldar', cx, cy + r + 11);
  ctx.restore();
}

/**
 * Draws Eldar's speech bubble when the local player has reached column 9+
 * (me.x >= 8 in 0-indexed terms). Rendered on top of everything so it is
 * always readable.
 */
function drawEldarBubble() {
  if (room !== ELDAR_ROOM) return;
  const me = players[myID];
  if (!me || me.x < 8) return;

  const ex = ELDAR_X * CELL_W + CELL_W / 2;
  const ey = ELDAR_Y * CELL_H + CELL_H / 2;
  const r  = 14;

  const lines = [
    'Welcome to the LeMahieu Kingdom!',
    'I am King Eldar Raiblock,',
    'lord of these lands.',
  ];

  ctx.save();
  ctx.font = 'bold 11px sans-serif';

  const pad   = 11;
  const lineH = 16;
  const bw    = Math.max(...lines.map(l => ctx.measureText(l).width)) + pad * 2;
  const bh    = lines.length * lineH + pad * 2;
  const ptr   = 9;  // pointer triangle height

  // Bubble anchored above Eldar's circle
  const bx = ex - bw / 2;
  const by = ey - r - ptr - bh - 2;

  // Background
  ctx.fillStyle   = '#fffff0';
  ctx.strokeStyle = '#D4AF37';
  ctx.lineWidth   = 1.5;
  ctx.beginPath();
  ctx.roundRect(bx, by, bw, bh, 7);
  ctx.fill();
  ctx.stroke();

  // Pointer triangle — fill first, then stroke only the two exposed sides
  const ptx = ex;
  const pty = ey - r - 2;
  ctx.fillStyle = '#fffff0';
  ctx.beginPath();
  ctx.moveTo(ptx - 8, by + bh);
  ctx.lineTo(ptx + 8, by + bh);
  ctx.lineTo(ptx, pty);
  ctx.closePath();
  ctx.fill();
  ctx.strokeStyle = '#D4AF37';
  ctx.lineWidth   = 1.5;
  ctx.beginPath();
  ctx.moveTo(ptx - 8, by + bh);
  ctx.lineTo(ptx, pty);
  ctx.lineTo(ptx + 8, by + bh);
  ctx.stroke();

  // Text
  ctx.fillStyle    = '#2a1a00';
  ctx.textAlign    = 'center';
  ctx.textBaseline = 'top';
  lines.forEach((line, i) => {
    ctx.fillText(line, ex, by + pad + i * lineH);
  });
  ctx.restore();
}

// ── Movement ───────────────────────────────────────────────────────────────

function moveTo(x, y) {
  send({ action: 'move', x, y });
}

// transitionRoom navigates to the grid adjacent in the direction the player walked off.
// Left/right change the column; up/down change the row. Each direction is independent.
function transitionRoom(nx, ny, curX, curY) {
  let col = gridCol, row = gridRow, ex, ey;
  if (nx < 0)          { col--;  ex = GRID_W - 1; ey = curY;       }
  else if (nx >= GRID_W) { col++;  ex = 0;          ey = curY;       }
  else if (ny < 0)     { row--;  ex = curX;        ey = GRID_H - 1; }
  else                 { row++;  ex = curX;        ey = 0;          }
  window.location.href = `/farm/game?room=${col},${row}&ex=${ex}&ey=${ey}`;
}

document.addEventListener('keydown', (e) => {
  // Don't capture keys while typing in any text input or modal
  if (document.activeElement === document.getElementById('chat-input')) return;
  if (document.activeElement === document.getElementById('dm-modal-input')) return;
  if (document.activeElement === document.getElementById('account-username-input')) return;
  if (document.activeElement === document.getElementById('account-email-input')) return;

  const dirs = {
    w: [0, -1], ArrowUp: [0, -1],
    s: [0,  1], ArrowDown: [0, 1],
    a: [-1, 0], ArrowLeft: [-1, 0],
    d: [1,  0], ArrowRight: [1, 0],
  };
  const dir = dirs[e.key];
  if (!dir) return;
  e.preventDefault();

  const me = players[myID];
  if (!me) return;
  const nx = me.x + dir[0];
  const ny = me.y + dir[1];
  if (nx < 0 || nx >= GRID_W || ny < 0 || ny >= GRID_H) {
    transitionRoom(nx, ny, me.x, me.y);
    return;
  }
  if (isBlockCell(nx, ny)) return;
  moveTo(nx, ny);
});

canvas.addEventListener('click', (e) => {
  const rect = canvas.getBoundingClientRect();
  const scaleX = canvas.width / rect.width;
  const scaleY = canvas.height / rect.height;
  const cx = Math.floor((e.clientX - rect.left) * scaleX / CELL_W);
  const cy = Math.floor((e.clientY - rect.top)  * scaleY / CELL_H);

  // Check for another player first — this must not depend on knowing our own
  // position, because players[myID] may be undefined on the first few ticks
  // after joining while state is still propagating.
  const target = Object.values(players).find(p => p.id !== myID && p.x === cx && p.y === cy);
  if (target) {
    openDMModal(target.username);
    return;
  }

  // Own position is required only for click-to-move.
  const me = players[myID];
  if (!me) return;

  const dx = Math.abs(cx - me.x);
  const dy = Math.abs(cy - me.y);
  // Allow click-to-move on adjacent cells (distance 1, no same-cell)
  if (dx <= 1 && dy <= 1 && !(dx === 0 && dy === 0) && !isBlockCell(cx, cy)) {
    moveTo(cx, cy);
  }
});

// ── Chat ───────────────────────────────────────────────────────────────────

const chatBox  = document.getElementById('chat-box');
const chatForm = document.getElementById('chat-form');
const chatInput = document.getElementById('chat-input');

chatForm.addEventListener('submit', (e) => {
  e.preventDefault();
  const text = chatInput.value.trim();
  if (!text) return;
  send({ action: 'chat', text });
  chatInput.value = '';
});

function appendChat(from, text) {
  const line = document.createElement('div');
  line.className = 'farm-chat-line farm-chat-line--chat';
  const color = (players[myID]?.username === from)
    ? myColor
    : (Object.values(players).find(p => p.username === from)?.color || '#a0c4ff');
  line.innerHTML = `<span class="farm-chat-from" style="color:${color}">${esc(from)}</span>: ${esc(text)}`;
  appendLine(line);
}

function appendSystem(text) {
  const line = document.createElement('div');
  line.className = 'farm-chat-line farm-chat-line--system';
  line.textContent = text;
  appendLine(line);
}

function appendLine(el) {
  chatBox.appendChild(el);
  chatBox.scrollTop = chatBox.scrollHeight;
}

function esc(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// ── Online list ────────────────────────────────────────────────────────────

const onlineList = document.getElementById('online-list');

function updateOnlineList() {
  onlineList.innerHTML = '';
  for (const p of Object.values(players)) {
    const li = document.createElement('li');
    const isSelf = p.id === myID;
    li.className = 'farm-online-item' + (isSelf ? '' : ' farm-online-item--clickable');
    li.innerHTML = `<span class="farm-online-dot" style="background:${p.color}"></span>${esc(p.username)}`;
    if (!isSelf) {
      li.title = `Message ${p.username}`;
      li.addEventListener('click', () => openDMModal(p.username));
    }
    onlineList.appendChild(li);
  }
}

// ── Wallet modal ───────────────────────────────────────────────────────────

const walletModal   = document.getElementById('wallet-modal');
const walletBtn     = document.getElementById('wallet-btn');
const walletClose   = document.getElementById('wallet-close');
const walletBalance = document.getElementById('wallet-balance');
const walletAddress = document.getElementById('wallet-address');
const copyBtn       = document.getElementById('copy-address-btn');
const copyConfirm   = document.getElementById('copy-confirm');
const withdrawAddr  = document.getElementById('withdraw-address');
const withdrawAmt   = document.getElementById('withdraw-amount');
const withdrawBtn   = document.getElementById('withdraw-btn');
const withdrawMsg   = document.getElementById('withdraw-msg');

walletBtn.addEventListener('click', () => {
  walletModal.classList.remove('hidden');
  fetchBalance();
});
walletClose.addEventListener('click', () => walletModal.classList.add('hidden'));
walletModal.addEventListener('click', (e) => {
  if (e.target === walletModal) walletModal.classList.add('hidden');
});

copyBtn.addEventListener('click', () => {
  navigator.clipboard.writeText(walletAddress.textContent).then(() => {
    copyConfirm.classList.remove('hidden');
    setTimeout(() => copyConfirm.classList.add('hidden'), 2000);
  });
});

function fetchBalance() {
  walletBalance.textContent = '…';
  fetch('/farm/api/balance')
    .then(r => r.json())
    .then(d => { walletBalance.textContent = d.xno; })
    .catch(() => { walletBalance.textContent = 'error'; });
}

withdrawBtn.addEventListener('click', () => {
  const toAddress = withdrawAddr.value.trim();
  const amountRaw = withdrawAmt.value.trim();
  if (!toAddress) {
    withdrawMsg.style.color = '#e74c3c';
    withdrawMsg.textContent = 'Please enter a destination address.';
    return;
  }
  withdrawMsg.style.color = '#a0c4ff';
  withdrawMsg.textContent = 'Sending…';
  withdrawBtn.disabled = true;

  fetch('/farm/withdraw', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ toAddress, amountRaw: amountRaw || '' }),
  })
    .then(async r => {
      if (!r.ok) throw new Error(await r.text());
      return r.json();
    })
    .then(d => {
      withdrawMsg.style.color = '#50c878';
      withdrawMsg.textContent = `Sent ${d.xno} XNO! Block: ${d.hash.slice(0, 12)}…`;
      fetchBalance();
    })
    .catch(err => {
      withdrawMsg.style.color = '#e74c3c';
      withdrawMsg.textContent = err.message || 'Send failed.';
    })
    .finally(() => { withdrawBtn.disabled = false; });
});

// ── Cell info modal ────────────────────────────────────────────────────────
// Built entirely in JS — no HTML element dependency that could null-crash on load.

const cellModal = document.createElement('div');
cellModal.style.cssText = [
  'position:fixed', 'bottom:32px', 'left:50%', 'transform:translateX(-50%)',
  'z-index:9999', 'display:none',
  'background:#12122a', 'border:1px solid #4a4a90', 'border-radius:8px',
  'padding:10px 18px', 'color:#a0c4ff', 'font:0.95rem monospace',
  'white-space:nowrap', 'box-shadow:0 4px 20px rgba(0,0,0,0.7)',
  'display:none', 'align-items:center', 'gap:14px',
].join(';');

const cellModalText  = document.createElement('span');
const cellModalClose = document.createElement('button');
cellModalClose.textContent = '✕';
cellModalClose.style.cssText = 'background:none;border:none;color:#606090;cursor:pointer;font-size:0.9rem;padding:0;line-height:1;';
cellModalClose.addEventListener('click', () => { cellModal.style.display = 'none'; });

cellModal.appendChild(cellModalText);
cellModal.appendChild(cellModalClose);
document.body.appendChild(cellModal);

// Show cell info on any canvas click — but not when another player occupies the
// cell, because the DM modal takes priority there and the cell info (z-index 9999)
// would otherwise render on top of it and block interaction.
canvas.addEventListener('click', (e) => {
  const rect = canvas.getBoundingClientRect();
  const scaleX = canvas.width / rect.width;
  const scaleY = canvas.height / rect.height;
  const cx = Math.floor((e.clientX - rect.left) * scaleX / CELL_W);
  const cy = Math.floor((e.clientY - rect.top)  * scaleY / CELL_H);
  if (cx < 0 || cx >= GRID_W || cy < 0 || cy >= GRID_H) return;
  // Defer to the DM flow when another player is on this cell.
  if (Object.values(players).some(p => p.id !== myID && p.x === cx && p.y === cy)) return;
  const cellNum = (gridRow * 1000 + gridCol) * GRID_W * GRID_H + cy * GRID_W + cx + 1;
  cellModalText.textContent = `Cell ${cellNum}  ·  col ${cx + 1}, row ${cy + 1}`;
  cellModal.style.display = 'flex';
});

// ── Account modal ──────────────────────────────────────────────────────────

const accountModal           = document.getElementById('account-modal');
const accountBtn             = document.getElementById('account-btn');
const accountUsernameInput   = document.getElementById('account-username-input');
const accountEmailInput      = document.getElementById('account-email-input');
const accountColorInput      = document.getElementById('account-color-input');
const accountColorPreview    = document.getElementById('account-color-preview');
const accountModalMsg        = document.getElementById('account-modal-msg');
const accountModalCancel     = document.getElementById('account-modal-cancel');
const accountModalSave       = document.getElementById('account-modal-save');

// Sync the preview circle whenever the picker value changes.
accountColorInput.addEventListener('input', () => {
  accountColorPreview.style.background = accountColorInput.value;
});

accountBtn.addEventListener('click', () => {
  accountUsernameInput.value = username;
  accountEmailInput.value    = canvas.dataset.email || '';
  // Fall back to the player's current color from the game state, or a default.
  const savedColor = canvas.dataset.color ||
    (myID && players[myID] ? players[myID].color : '#4a90d9') ||
    '#4a90d9';
  accountColorInput.value  = savedColor;
  accountColorPreview.style.background = savedColor;
  accountModalMsg.textContent = '';
  accountModal.classList.remove('hidden');
  accountUsernameInput.focus();
});

accountModalCancel.addEventListener('click', () => accountModal.classList.add('hidden'));
accountModal.addEventListener('click', (e) => {
  if (e.target === accountModal) accountModal.classList.add('hidden');
});

accountModalSave.addEventListener('click', () => {
  const newUsername = accountUsernameInput.value.trim();
  if (!newUsername) {
    accountModalMsg.style.color = '#e74c3c';
    accountModalMsg.textContent = 'Username cannot be empty.';
    return;
  }
  const email = accountEmailInput.value.trim();
  if (!email || !email.includes('@')) {
    accountModalMsg.style.color = '#e74c3c';
    accountModalMsg.textContent = 'Please enter a valid email address.';
    return;
  }
  const color = accountColorInput.value; // always a valid #RRGGBB from <input type="color">
  accountModalMsg.style.color = '#a0c4ff';
  accountModalMsg.textContent = 'Saving…';
  accountModalSave.disabled = true;

  fetch('/farm/account', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: newUsername, email, color }),
  })
    .then(async r => {
      if (!r.ok) throw new Error(await r.text());
      return r.json();
    })
    .then(d => {
      // Persist new values in data attributes.
      canvas.dataset.username = d.username;
      canvas.dataset.email    = d.email;
      canvas.dataset.color    = d.color;

      // Update module-level username so DM sent/received logic stays correct.
      username = d.username;

      // Update the header player name display.
      const nameEl = document.querySelector('.farm-player-name strong');
      if (nameEl) nameEl.textContent = d.username;

      // Update local player state and re-render immediately.
      myColor = d.color;
      if (myID && players[myID]) {
        players[myID].color    = d.color;
        players[myID].username = d.username;
        render();
        updateOnlineList();
      }

      // Push both changes to the room so all other players see them.
      send({ action: 'color',    text: d.color });
      send({ action: 'username', text: d.username });

      accountModal.classList.add('hidden');
    })
    .catch(err => {
      accountModalMsg.style.color = '#e74c3c';
      accountModalMsg.textContent = err.message || 'Save failed.';
    })
    .finally(() => { accountModalSave.disabled = false; });
});

// ── DM panel ───────────────────────────────────────────────────────────────

const dmList  = document.getElementById('dm-list');
const dmEmpty = document.getElementById('dm-empty');

/**
 * appendDM renders a DM card in the left panel.
 * Sent cards (from === username) are styled differently from received ones.
 * Clicking a received card opens the DM modal pre-filled with the sender;
 * clicking a sent card pre-fills the original recipient.
 */
function appendDM(from, to, text) {
  // Remove the empty-state placeholder on first message.
  if (dmEmpty) dmEmpty.remove();

  const isSent = from === username;
  const peer   = isSent ? to : from;
  const color  = Object.values(players).find(p => p.username === peer)?.color || '#a0c4ff';
  const now    = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

  const card = document.createElement('div');
  card.className = 'farm-dm-card' + (isSent ? ' farm-dm-card--sent' : '');
  card.title = `Reply to ${peer}`;
  card.innerHTML =
    `<div class="farm-dm-card-meta">` +
      `<span class="farm-dm-card-from" style="color:${color}">${esc(isSent ? '→ ' + to : from)}</span>` +
      `<span class="farm-dm-card-time">${now}</span>` +
    `</div>` +
    `<div class="farm-dm-card-text">${esc(text)}</div>`;

  card.addEventListener('click', () => openDMModal(peer));
  dmList.appendChild(card);
  dmList.scrollTop = dmList.scrollHeight;
}

// ── DM modal ───────────────────────────────────────────────────────────────

const dmModal       = document.getElementById('dm-modal');
const dmModalToName = document.getElementById('dm-modal-to-name');
const dmModalInput  = document.getElementById('dm-modal-input');
const dmModalMsg    = document.getElementById('dm-modal-msg');
const dmModalCancel = document.getElementById('dm-modal-cancel');
const dmModalSend   = document.getElementById('dm-modal-send');

/** Opens the DM compose modal directed at toUsername. */
function openDMModal(toUsername) {
  dmModalToName.textContent = toUsername;
  dmModalToName.style.color =
    Object.values(players).find(p => p.username === toUsername)?.color || '#a0c4ff';
  dmModalInput.value = '';
  dmModalMsg.textContent = '';
  dmModal.classList.remove('hidden');
  dmModalInput.focus();
}

function closeDMModal() {
  dmModal.classList.add('hidden');
}

dmModalCancel.addEventListener('click', closeDMModal);
dmModal.addEventListener('click', (e) => {
  if (e.target === dmModal) closeDMModal();
});

dmModalSend.addEventListener('click', () => {
  const text = dmModalInput.value.trim();
  const to   = dmModalToName.textContent;
  if (!text) {
    dmModalMsg.style.color = '#e74c3c';
    dmModalMsg.textContent = 'Message cannot be empty.';
    return;
  }
  send({ action: 'dm', to, text });
  closeDMModal();
});

// Send on Enter (Shift+Enter inserts newline).
dmModalInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    dmModalSend.click();
  }
});

// ── Boot ───────────────────────────────────────────────────────────────────

connect();
render(); // draw empty grid immediately
