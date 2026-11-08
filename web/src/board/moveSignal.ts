// Move signal (approved in prototypes/move-signal.html, variant 7): a glow spreads along the grid
// lines about 2 cells from a new stone and fades within 1 s; the last stone keeps four pulsing beams of
// fire (black) / frost (white) along its own lines, out to its neighbouring points, until the next move.
// The shader is copied from the prototype; keep the two in sync.

export const SIGNAL_TIME = 1.0;   // the spreading glow, s
export const SIGNAL_OUT = 0.4;    // the beams go out after the next move, s
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
  uniform vec2 uCenter; uniform float uR; uniform float uProg; uniform int uColor; uniform vec4 uGrid; uniform float uTime; uniform float uKeep;
  float h(vec2 p) { p = fract(p * vec2(123.34, 456.21)); p += dot(p, p + 45.32); return fract(p.x * p.y); }
  float vn(vec2 p) { vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f);
    return mix(mix(h(i), h(i + vec2(1, 0)), u.x), mix(h(i + vec2(0, 1)), h(i + vec2(1, 1)), u.x), u.y); }
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
    // last-move mark: four pulsing beams along the stone's own lines
    vec2 v = vLocal * uR;
    float cell = uGrid.z, beam = 0.0, hot = 0.0;
    for (int k = 0; k < 2; k++) {
      float along = k == 0 ? abs(v.x) : abs(v.y);
      float across = k == 0 ? abs(v.y) : abs(v.x);
      float side = k == 0 ? sign(v.x) : sign(v.y);
      float t = (along - cell * 0.42) / (cell * 0.62);
      float taper = smoothstep(-0.15, 0.05, t) * (1.0 - smoothstep(0.8, 1.08, t)) * (1.0 - 0.35 * clamp(t, 0.0, 1.0));
      float n = vn(vec2(along / cell * 5.0 - uTime * 3.2, side * 7.0 + float(k) * 13.0));
      float n2 = vn(vec2(along / cell * 11.0 - uTime * 5.0, side * 3.0 + float(k) * 5.0));
      float w = cell * (0.09 + 0.08 * n) * (1.0 - 0.4 * clamp(t, 0.0, 1.0));
      float pulse = 0.72 + 0.28 * sin(uTime * 3.4 - along / cell * 5.0);
      float core = exp(-pow(across / max(w * 0.45, 0.6), 2.0));
      float glow = exp(-pow(across / (w * 2.6 + 1.5), 2.0));
      float b = (core * 1.2 + glow * 0.7) * taper * pulse * (0.75 + 0.5 * n2);
      beam = max(beam, b);
      hot = max(hot, core * taper * pulse);
    }
    float keep = clamp(beam, 0.0, 1.0) * smoothstep(0.45, 0.85, uProg) * uKeep * inBoard;
    vec3 beamCol = uColor == 0
      ? mix(vec3(0.95, 0.25, 0.04), vec3(1.0, 0.86, 0.45), clamp(hot, 0.0, 1.0))
      : mix(vec3(0.3, 0.62, 1.0), vec3(0.92, 0.98, 1.0), clamp(hot, 0.0, 1.0));
    vec3 rgb = col * waveA * (1.0 - keep) + beamCol * keep;
    float a = clamp(waveA + keep - waveA * keep, 0.0, 1.0);
    outColor = vec4(rgb, a);
  }`;

export type Signal = { idx: number; color: 0 | 1; start: number; endAt?: number };

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
  const U = { uCenter: u('uCenter'), uR: u('uR'), uRes: u('uRes'), uProg: u('uProg'), uColor: u('uColor'), uGrid: u('uGrid'), uTime: u('uTime'), uKeep: u('uKeep') };
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
    const alive = list.filter((s) => s.endAt === undefined || t - s.endAt < SIGNAL_OUT);
    if (!alive.length) return alive;
    gl.useProgram(p);
    gl.bindVertexArray(vao);
    gl.uniform2f(U.uRes, geo.w, geo.h);
    gl.uniform4f(U.uGrid, geo.margin, geo.margin, geo.cell, geo.size);
    gl.uniform1f(U.uR, geo.cell * RADIUS);
    gl.uniform1f(U.uTime, t);
    for (const s of alive) {
      const x = s.idx % geo.size, y = Math.floor(s.idx / geo.size);
      gl.uniform2f(U.uCenter, geo.margin + x * geo.cell, geo.margin + y * geo.cell);
      gl.uniform1f(U.uProg, Math.min(1, Math.max(0, (t - s.start) / SIGNAL_TIME)));
      gl.uniform1i(U.uColor, s.color);
      gl.uniform1f(U.uKeep, s.endAt === undefined ? 1 : Math.max(0, 1 - (t - s.endAt) / SIGNAL_OUT));
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
    }
    gl.bindVertexArray(null);
    return alive;
  }
  return { draw };
}
