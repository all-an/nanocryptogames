// rpg.js — Nano Faucet Multiplayer RPG client
'use strict';

const GRID_W   = 20;
const GRID_H   = 15;
const CELL_W   = 40;
const CELL_H   = 40;

const canvas   = document.getElementById('rpg-canvas');
const ctx      = canvas.getContext('2d');
const username = canvas.dataset.username;
const room     = canvas.dataset.room;

// State
let myID      = null;
let myColor   = '#4A90D9';
let players   = {};   // id → {id, username, x, y, color}
let connected = false;

// ── WebSocket ──────────────────────────────────────────────────────────────

function wsURL() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  return `${proto}://${location.host}/rpg/ws?room=${encodeURIComponent(room)}`;
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
  }
}

// ── Rendering ──────────────────────────────────────────────────────────────

function render() {
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  drawGrid();
  drawPlayers();
}

function drawGrid() {
  for (let y = 0; y < GRID_H; y++) {
    for (let x = 0; x < GRID_W; x++) {
      const px = x * CELL_W;
      const py = y * CELL_H;
      const cellNum = y * GRID_W + x + 1;

      // Cell background
      ctx.fillStyle = '#0f0f22';
      ctx.fillRect(px, py, CELL_W - 1, CELL_H - 1);

      // Cell number
      ctx.fillStyle = '#2a2a50';
      ctx.font = '9px monospace';
      ctx.textAlign = 'left';
      ctx.fillText(cellNum, px + 2, py + 10);
    }
  }
}

function drawPlayers() {
  for (const p of Object.values(players)) {
    const cx = p.x * CELL_W + CELL_W / 2;
    const cy = p.y * CELL_H + CELL_H / 2;
    const r  = 13;

    // Shadow
    ctx.beginPath();
    ctx.arc(cx, cy + 2, r, 0, Math.PI * 2);
    ctx.fillStyle = 'rgba(0,0,0,0.4)';
    ctx.fill();

    // Player circle
    ctx.beginPath();
    ctx.arc(cx, cy, r, 0, Math.PI * 2);
    ctx.fillStyle = p.color;
    ctx.fill();

    // White ring for self
    if (p.id === myID) {
      ctx.strokeStyle = '#ffffff';
      ctx.lineWidth = 2;
      ctx.stroke();
    }

    // Nano ticker inside circle
    ctx.fillStyle = 'rgba(255,255,255,0.85)';
    ctx.font = 'bold 11px sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText('Ӿ', cx, cy);
    ctx.textBaseline = 'alphabetic';

    // Username label
    ctx.fillStyle = '#ffffff';
    ctx.font = 'bold 9px sans-serif';
    ctx.textAlign = 'center';
    ctx.fillText(p.username.slice(0, 10), cx, cy + r + 11);
  }
}

// ── Movement ───────────────────────────────────────────────────────────────

function moveTo(x, y) {
  send({ action: 'move', x, y });
}

document.addEventListener('keydown', (e) => {
  // Don't capture keys while typing in chat
  if (document.activeElement === document.getElementById('chat-input')) return;

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
  if (nx < 0 || nx >= GRID_W || ny < 0 || ny >= GRID_H) return;
  moveTo(nx, ny);
});

canvas.addEventListener('click', (e) => {
  const rect = canvas.getBoundingClientRect();
  const scaleX = canvas.width / rect.width;
  const scaleY = canvas.height / rect.height;
  const cx = Math.floor((e.clientX - rect.left) * scaleX / CELL_W);
  const cy = Math.floor((e.clientY - rect.top)  * scaleY / CELL_H);

  const me = players[myID];
  if (!me) return;
  const dx = Math.abs(cx - me.x);
  const dy = Math.abs(cy - me.y);
  // Allow click-to-move on adjacent cells (distance 1, no same-cell)
  if (dx <= 1 && dy <= 1 && !(dx === 0 && dy === 0)) {
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
  line.className = 'rpg-chat-line rpg-chat-line--chat';
  const color = (players[myID]?.username === from)
    ? myColor
    : (Object.values(players).find(p => p.username === from)?.color || '#a0c4ff');
  line.innerHTML = `<span class="rpg-chat-from" style="color:${color}">${esc(from)}</span>: ${esc(text)}`;
  appendLine(line);
}

function appendSystem(text) {
  const line = document.createElement('div');
  line.className = 'rpg-chat-line rpg-chat-line--system';
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
    li.className = 'rpg-online-item';
    li.innerHTML = `<span class="rpg-online-dot" style="background:${p.color}"></span>${esc(p.username)}`;
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
  fetch('/rpg/api/balance')
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

  fetch('/rpg/withdraw', {
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

// ── Boot ───────────────────────────────────────────────────────────────────

connect();
render(); // draw empty grid immediately
