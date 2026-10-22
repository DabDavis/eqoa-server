// A model on a turntable: the mesh the disc worker built, its textures and
// tints, lit from the front; drag to turn it, the wheel to come closer,
// double-click to start again.

const VS = `#version 300 es
in vec3 aPos; in vec3 aNrm; in vec2 aUV; in vec3 aCol;
uniform mat4 uMVP; uniform vec3 uU; uniform vec3 uV;
out vec2 vUV; out vec3 vN; out vec3 vC;
void main() {
  gl_Position = uMVP * vec4(aPos, 1.0);
  vUV = vec2(aUV.x * uU.x + aUV.y * uU.y + uU.z, aUV.x * uV.x + aUV.y * uV.y + uV.z);
  vN = aNrm; vC = aCol;
}`;
const FS = `#version 300 es
precision mediump float;
in vec2 vUV; in vec3 vN; in vec3 vC;
uniform sampler2D uTex; uniform vec4 uTint; uniform float uHasTex; uniform vec3 uLight;
out vec4 o;
void main() {
  vec4 t = uHasTex > 0.5 ? texture(uTex, vUV) : vec4(0.7, 0.7, 0.7, 1.0);
  if (t.a < 0.35) discard;
  vec3 n = normalize(vN);
  float lit = 0.42 + 0.58 * abs(dot(n, uLight));
  o = vec4(t.rgb * uTint.rgb * vC * lit, 1.0);
}`;

function mat4mul(a, b) {
  const o = new Float32Array(16);
  for (let c = 0; c < 4; c++) for (let r = 0; r < 4; r++) {
    let s = 0;
    for (let k = 0; k < 4; k++) s += a[k * 4 + r] * b[c * 4 + k];
    o[c * 4 + r] = s;
  }
  return o;
}

function perspective(fovy, aspect, near, far) {
  const f = 1 / Math.tan(fovy / 2), nf = 1 / (near - far);
  return new Float32Array([f / aspect, 0, 0, 0, 0, f, 0, 0, 0, 0, (far + near) * nf, -1, 0, 0, 2 * far * near * nf, 0]);
}

function lookAt(eye, at, up) {
  const z = norm([eye[0] - at[0], eye[1] - at[1], eye[2] - at[2]]);
  const x = norm(cross(up, z));
  const y = cross(z, x);
  return new Float32Array([x[0], y[0], z[0], 0, x[1], y[1], z[1], 0, x[2], y[2], z[2], 0,
    -dot(x, eye), -dot(y, eye), -dot(z, eye), 1]);
}
const cross = (a, b) => [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]];
const dot = (a, b) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
const norm = (a) => { const l = Math.hypot(a[0], a[1], a[2]) || 1; return [a[0] / l, a[1] / l, a[2] / l]; };

export class Viewer {
  constructor(canvas) {
    this.canvas = canvas;
    const gl = canvas.getContext('webgl2', { antialias: true, alpha: true });
    if (!gl) throw new Error('this browser has no WebGL2');
    this.gl = gl;
    const sh = (type, src) => {
      const s = gl.createShader(type);
      gl.shaderSource(s, src);
      gl.compileShader(s);
      if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(s));
      return s;
    };
    this.prog = gl.createProgram();
    gl.attachShader(this.prog, sh(gl.VERTEX_SHADER, VS));
    gl.attachShader(this.prog, sh(gl.FRAGMENT_SHADER, FS));
    gl.linkProgram(this.prog);
    this.yaw = 0.6;
    this.pitch = 0.18;
    this.dist = 1;
    this.model = null;
    let drag = null;
    canvas.addEventListener('pointerdown', (e) => { canvas.setPointerCapture(e.pointerId); drag = { x: e.clientX, y: e.clientY }; });
    canvas.addEventListener('pointermove', (e) => {
      if (!drag) return;
      this.yaw -= (e.clientX - drag.x) * 0.01;
      this.pitch = Math.max(-1.4, Math.min(1.4, this.pitch + (e.clientY - drag.y) * 0.01));
      drag = { x: e.clientX, y: e.clientY };
      this.draw();
    });
    canvas.addEventListener('pointerup', () => { drag = null; });
    canvas.addEventListener('wheel', (e) => { e.preventDefault(); this.dist *= e.deltaY < 0 ? 0.88 : 1.14; this.draw(); }, { passive: false });
    canvas.addEventListener('dblclick', () => { this.frame(); this.draw(); });
    this.ro = new ResizeObserver(() => this.draw());
    this.ro.observe(canvas);
  }

  setModel({ mesh, textures }) {
    const gl = this.gl;
    this.dispose(false);
    const vao = gl.createVertexArray();
    gl.bindVertexArray(vao);
    const bufs = [];
    const attr = (name, data, n) => {
      const b = gl.createBuffer();
      bufs.push(b);
      gl.bindBuffer(gl.ARRAY_BUFFER, b);
      gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW);
      const loc = gl.getAttribLocation(this.prog, name);
      gl.enableVertexAttribArray(loc);
      gl.vertexAttribPointer(loc, n, gl.FLOAT, false, 0, 0);
    };
    attr('aPos', mesh.pos, 3);
    attr('aNrm', mesh.nrm, 3);
    attr('aUV', mesh.uv, 2);
    attr('aCol', mesh.col, 3);
    const texs = new Map();
    for (const [id, img] of Object.entries(textures || {})) {
      const t = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, t);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, img.w, img.h, 0, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array(img.data.buffer));
      gl.generateMipmap(gl.TEXTURE_2D);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT);
      texs.set(Number(id), t);
    }
    const batches = mesh.batches.map((b) => {
      const ib = gl.createBuffer();
      bufs.push(ib);
      gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, ib);
      gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, b.idx, gl.STATIC_DRAW);
      return { ib, count: b.idx.length, tex: texs.get(b.tex) || null, col: b.col, uv: b.uv };
    });
    this.model = { vao, bufs, texs, batches, bbox: mesh.bbox };
    this.frame();
    this.draw();
  }

  frame() {
    if (!this.model) return;
    const b = this.model.bbox;
    this.center = [(b[0] + b[3]) / 2, (b[1] + b[4]) / 2, (b[2] + b[5]) / 2];
    this.radius = Math.max(0.5, Math.hypot(b[3] - b[0], b[4] - b[1], b[5] - b[2]) / 2);
    this.dist = this.radius * 2.6;
    this.yaw = 0.6;
    this.pitch = 0.18;
  }

  draw() {
    const gl = this.gl, c = this.canvas;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const w = Math.max(1, Math.round(c.clientWidth * dpr)), h = Math.max(1, Math.round(c.clientHeight * dpr));
    if (c.width !== w || c.height !== h) { c.width = w; c.height = h; }
    gl.viewport(0, 0, w, h);
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
    if (!this.model) return;
    gl.enable(gl.DEPTH_TEST);
    gl.disable(gl.CULL_FACE);
    gl.useProgram(this.prog);
    const eye = [
      this.center[0] + this.dist * Math.cos(this.pitch) * Math.sin(this.yaw),
      this.center[1] + this.dist * Math.sin(this.pitch),
      this.center[2] + this.dist * Math.cos(this.pitch) * Math.cos(this.yaw),
    ];
    const proj = perspective(0.75, w / h, this.radius * 0.02, this.radius * 20 + this.dist);
    const mvp = mat4mul(proj, lookAt(eye, this.center, [0, 1, 0]));
    gl.uniformMatrix4fv(gl.getUniformLocation(this.prog, 'uMVP'), false, mvp);
    gl.uniform3fv(gl.getUniformLocation(this.prog, 'uLight'), norm([eye[0] - this.center[0], eye[1] - this.center[1] + this.radius, eye[2] - this.center[2]]));
    gl.uniform1i(gl.getUniformLocation(this.prog, 'uTex'), 0);
    const uTint = gl.getUniformLocation(this.prog, 'uTint'), uHas = gl.getUniformLocation(this.prog, 'uHasTex');
    const uU = gl.getUniformLocation(this.prog, 'uU'), uV = gl.getUniformLocation(this.prog, 'uV');
    gl.bindVertexArray(this.model.vao);
    for (const b of this.model.batches) {
      gl.activeTexture(gl.TEXTURE0);
      gl.bindTexture(gl.TEXTURE_2D, b.tex);
      gl.uniform1f(uHas, b.tex ? 1 : 0);
      gl.uniform4f(uTint, b.col[0] / 255, b.col[1] / 255, b.col[2] / 255, 1);
      gl.uniform3f(uU, b.uv[0], b.uv[2], b.uv[4]);
      gl.uniform3f(uV, b.uv[1], b.uv[3], b.uv[5]);
      gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, b.ib);
      gl.drawElements(gl.TRIANGLES, b.count, gl.UNSIGNED_INT, 0);
    }
  }

  dispose(all = true) {
    const gl = this.gl;
    if (this.model) {
      for (const b of this.model.bufs) gl.deleteBuffer(b);
      for (const t of this.model.texs.values()) gl.deleteTexture(t);
      gl.deleteVertexArray(this.model.vao);
      this.model = null;
    }
    if (all) this.ro.disconnect();
  }
}
