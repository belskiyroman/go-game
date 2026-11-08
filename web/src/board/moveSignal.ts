// Move signal (prototypes/move-signal.html, variant 7, the wave part): a glow spreads along the grid
// lines about 2 cells from a new stone and fades within 1 s. The pulsing beams the prototype keeps on
// the last stone were removed on the user's request: they pulled the eye away from the game.
// Shift replays the wave on the last stone.

export const SIGNAL_TIME = 1.0;   // the spreading glow, s
const RADIUS = 2.4;               // glow radius, cells

const VS = `#version 300 es
  in vec2 aPos;
  uniform vec2 uCenter; uniform float uR; uniform vec2 uRes;
  out vec2 vLocal;
  void main() {
    vLocal = aPos;
    vec2 px = uCenter + aPos * uR;
    vec2 clip = px / uRes * 2.0 - 1.0; clip.y = -clip.y;
    gl_Position = vec4(clip, 0.0, 1.0);
  }`;

const FS = `#version 300 es
  precision highp float;
  in vec2 vLocal; out vec4 outColor;
  uniform vec2 uCenter; uniform float uR; uniform float uProg; uniform int uColor; uniform vec4 uGrid;
  void main() {
    vec3 col = uColor == 0 ? vec3(1.0, 0.48, 0.12) : vec3(0.45, 0.78, 1.0);
    float d = length(vLocal);
    vec2 px = uCenter + vLocal * uR;
    vec2 g = (px - uGrid.xy) / uGrid.z;
    float m = 0.45;
    if (g.x < -m || g.y < -m || g.x > uGrid.w - 1.0 + m || g.y > uGrid.w - 1.0 + m) discard;
    // spreading glow along the grid lines
    float dpx = d * uR;
    float reach = uR * (1.0 - pow(1.0 - min(1.0, uProg / 0.55), 2.0));
    float lit = 1.0 - smoothstep(reach - uGrid.z * 0.45, reach, dpx);
    float front = exp(-pow((dpx - reach) / (uGrid.z * 0.3), 2.0)) * (1.0 - smoothstep(0.45, 0.6, uProg));
    float falloff = 1.0 - smoothstep(uR * 0.35, uR, dpx);
    float lfade = 1.0 - smoothstep(0.6, 1.0, uProg);
    float wave = (lit * (0.35 + 0.65 * falloff) + front * 0.6) * lfade;
    vec2 f = abs(fract(g + 0.5) - 0.5) * uGrid.z;
    float line = 1.0 - smoothstep(0.7, 2.4, min(f.x, f.y));
    float inBoard = step(-0.05, g.x) * step(-0.05, g.y) * step(g.x, uGrid.w - 0.95) * step(g.y, uGrid.w - 0.95);
    float waveA = clamp(wave * line * inBoard, 0.0, 1.0);
    outColor = vec4(col * waveA, waveA);
  }`;

export type Signal = { idx: number; color: 0 | 1; start: number };

export function createMoveSignal(gl: WebGL2RenderingContext) {
  const sh = (t: number, s: string) => {
    const x = gl.createShader(t)!; gl.shaderSource(x, s); gl.compileShader(x);
    if (!gl.getShaderParameter(x, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(x) ?? 'shader');
    return x;
  };
  const p = gl.createProgram()!;
  gl.attachShader(p, sh(gl.VERTEX_SHADER, VS)); gl.attachShader(p, sh(gl.FRAGMENT_SHADER, FS));
  gl.bindAttribLocation(p, 0, 'aPos');
  gl.linkProgram(p);
  const u = (n: string) => gl.getUniformLocation(p, n);
  const U = { uCenter: u('uCenter'), uR: u('uR'), uRes: u('uRes'), uProg: u('uProg'), uColor: u('uColor'), uGrid: u('uGrid') };
  const vao = gl.createVertexArray()!;
  gl.bindVertexArray(vao);
  const buf = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, buf);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW);
  gl.enableVertexAttribArray(0);
  gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
  gl.bindVertexArray(null);

  /** Draws the signals; geometry in CSS pixels. Returns the signals still alive. */
  function draw(list: Signal[], t: number, geo: { w: number; h: number; size: number; margin: number; cell: number }) {
    const alive = list.filter((s) => t - s.start < SIGNAL_TIME);
    if (!alive.length) return alive;
    gl.useProgram(p);
    gl.bindVertexArray(vao);
    gl.uniform2f(U.uRes, geo.w, geo.h);
    gl.uniform4f(U.uGrid, geo.margin, geo.margin, geo.cell, geo.size);
    gl.uniform1f(U.uR, geo.cell * RADIUS);
    for (const s of alive) {
      const x = s.idx % geo.size, y = Math.floor(s.idx / geo.size);
      gl.uniform2f(U.uCenter, geo.margin + x * geo.cell, geo.margin + y * geo.cell);
      gl.uniform1f(U.uProg, Math.min(1, Math.max(0, (t - s.start) / SIGNAL_TIME)));
      gl.uniform1i(U.uColor, s.color);
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
    }
    gl.bindVertexArray(null);
    return alive;
  }
  return { draw };
}
