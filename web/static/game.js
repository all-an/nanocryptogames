// game.js — Canvas renderer and WebSocket client for Nano Multiplayer.
// The server is authoritative; this file is display + input only.

const canvas = document.getElementById("game");
const ctx = canvas.getContext("2d");

// Room ID is embedded in the canvas element by the server-rendered template.
const roomID = canvas.dataset.room;

// myID is set once the server sends the "init" message after WS connect.
let myID = null;

// state holds the latest world snapshot received from the server.
let state = { players: [] };

// keys tracks which movement keys are currently held down.
const keys = {};

// ── WebSocket ────────────────────────────────────────────────────────────────

const wsProto = location.protocol === "https:" ? "wss:" : "ws:";
const ws = new WebSocket(`${wsProto}//${location.host}/ws/${roomID}`);

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);

  if (msg.type === "init") {
    // Server assigns our ID and colour on connect.
    myID = msg.id;
  } else if (msg.type === "state") {
    state = msg;
  }
};

ws.onclose = () => {
  // Dim the canvas to signal disconnection.
  ctx.fillStyle = "rgba(0,0,0,0.6)";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = "#fff";
  ctx.font = "24px system-ui";
  ctx.textAlign = "center";
  ctx.fillText("Disconnected", canvas.width / 2, canvas.height / 2);
};

// ── Input ────────────────────────────────────────────────────────────────────

document.addEventListener("keydown", (e) => {
  if (!keys[e.key]) {
    keys[e.key] = true;
    sendMove();
  }
});

document.addEventListener("keyup", (e) => {
  keys[e.key] = false;
  sendMove();
});

// sendMove computes the current direction vector from held keys and sends it to
// the server. The server stores the velocity and applies it every tick.
function sendMove() {
  if (ws.readyState !== WebSocket.OPEN) return;

  let dx = 0;
  let dy = 0;

  if (keys["ArrowLeft"]  || keys["a"]) dx -= 1;
  if (keys["ArrowRight"] || keys["d"]) dx += 1;
  if (keys["ArrowUp"]    || keys["w"]) dy -= 1;
  if (keys["ArrowDown"]  || keys["s"]) dy += 1;

  ws.send(JSON.stringify({ dx, dy }));
}

// ── Renderer ─────────────────────────────────────────────────────────────────

function draw() {
  // Background
  ctx.fillStyle = "#1a1a2e";
  ctx.fillRect(0, 0, canvas.width, canvas.height);

  // Subtle arena border
  ctx.strokeStyle = "#2a2a4a";
  ctx.lineWidth = 2;
  ctx.strokeRect(1, 1, canvas.width - 2, canvas.height - 2);

  for (const p of state.players || []) {
    drawPlayer(p);
  }

  requestAnimationFrame(draw);
}

// drawPlayer renders a filled circle with the Ӿ Nano symbol centred inside.
// The local player gets a white ring to distinguish it from others.
function drawPlayer(player) {
  const r = 20;
  const isMe = player.id === myID;

  // ── Circle body ──
  ctx.beginPath();
  ctx.arc(player.x, player.y, r, 0, Math.PI * 2);
  ctx.fillStyle = player.color;
  ctx.fill();

  // ── White outline ring for the local player ──
  if (isMe) {
    ctx.strokeStyle = "#ffffff";
    ctx.lineWidth = 2.5;
    ctx.stroke();
  }

  // ── Ӿ glyph centred in the circle ──
  ctx.fillStyle = "#ffffff";
  ctx.font = "bold 16px system-ui";
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillText("Ӿ", player.x, player.y);

  // ── Health bar above the circle ──
  drawHealthBar(player.x, player.y - r - 10, player.health);
}

// drawHealthBar renders a small coloured bar above the player circle.
function drawHealthBar(cx, cy, health) {
  const w = 40;
  const h = 4;
  const x = cx - w / 2;

  // Background track
  ctx.fillStyle = "#333";
  ctx.fillRect(x, cy, w, h);

  // Filled portion — green → red as health drops
  const pct = Math.max(0, health / 100);
  ctx.fillStyle = pct > 0.5 ? "#52C07A" : pct > 0.25 ? "#F5A623" : "#E05252";
  ctx.fillRect(x, cy, w * pct, h);
}

draw();
