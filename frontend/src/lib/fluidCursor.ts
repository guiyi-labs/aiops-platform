/* eslint-disable @typescript-eslint/no-explicit-any -- Ported WebGL code; extension
   methods (WebGL2, OES_texture_half_float, etc.) lack TypeScript definitions. */
/**
 * fluidCursor.ts — WebGL Fluid Simulation cursor effect
 *
 * Ported from the internal LimX Dynamics FluidCursor implementation.
 * Framework-agnostic: accepts an HTMLCanvasElement and manages the full
 * WebGL Navier-Stokes pipeline (advection, divergence, pressure, curl,
 * vorticity confinement, splat rendering) with pointer tracking.
 *
 * Based on Pavel Dobryakov's WebGL Fluid Simulation.
 */

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

export interface FluidCursorConfig {
  simResolution: number
  dyeResolution: number
  captureResolution: number
  densityDissipation: number
  velocityDissipation: number
  pressure: number
  pressureIterations: number
  curl: number
  splatRadius: number
  splatForce: number
  shading: boolean
  colorUpdateSpeed: number
  backColor: { r: number; g: number; b: number }
  transparent: boolean
  paused?: boolean
}

interface PointPointer {
  id: number
  texcoordX: number
  texcoordY: number
  prevTexcoordX: number
  prevTexcoordY: number
  deltaX: number
  deltaY: number
  down: boolean
  moved: boolean
  color: { r: number; g: number; b: number }
}

interface DoubleFBO {
  width: number
  height: number
  texelSizeX: number
  texelSizeY: number
  read: FBO
  write: FBO
  swap(): void
}

interface FBO {
  texture: WebGLTexture
  fbo: WebGLFramebuffer
  width: number
  height: number
  texelSizeX: number
  texelSizeY: number
  attach(unit: number): number
}

interface Material {
  program: WebGLProgram | null
  uniforms: Record<string, WebGLUniformLocation | null>
  bind(gl: WebGLRenderingContext): void
  setKeywords?(gl: WebGLRenderingContext, keywords: string[]): void
}

/* ------------------------------------------------------------------ */
/*  Defaults                                                           */
/* ------------------------------------------------------------------ */

const DEFAULTS: FluidCursorConfig = {
  simResolution: 128,
  dyeResolution: 1024,
  captureResolution: 512,
  densityDissipation: 3.5,
  velocityDissipation: 2,
  pressure: 0.1,
  pressureIterations: 20,
  curl: 3,
  splatRadius: 0.4,
  splatForce: 6000,
  shading: true,
  colorUpdateSpeed: 10,
  backColor: { r: 0.5, g: 0, b: 0 },
  transparent: true,
}

/* ------------------------------------------------------------------ */
/*  WebGL helpers                                                      */
/* ------------------------------------------------------------------ */

function compileShader(
  gl: WebGLRenderingContext,
  type: number,
  source: string,
  keywords?: string[] | null,
): WebGLShader | null {
  let src = source
  if (keywords?.length) {
    src = keywords.map((k) => `#define ${k}\n`).join('') + src
  }
  const shader = gl.createShader(type)
  if (!shader) return null
  gl.shaderSource(shader, src)
  gl.compileShader(shader)
  return shader
}

function linkProgram(
  gl: WebGLRenderingContext,
  vs: WebGLShader,
  fs: WebGLShader,
): WebGLProgram | null {
  const prog = gl.createProgram()
  if (!prog) return null
  gl.attachShader(prog, vs)
  gl.attachShader(prog, fs)
  gl.linkProgram(prog)
  return prog
}

function createProgram(
  gl: WebGLRenderingContext,
  vertexSrc: string,
  fragmentSrc: string,
  keywords?: string[] | null,
) {
  const vs = compileShader(gl, gl.VERTEX_SHADER, vertexSrc)!
  const fs = compileShader(gl, gl.FRAGMENT_SHADER, fragmentSrc, keywords)!
  const prog = linkProgram(gl, vs, fs)!
  const uniforms: Record<string, WebGLUniformLocation | null> = {}
  const count = gl.getProgramParameter(prog, gl.ACTIVE_UNIFORMS) as number
  for (let i = 0; i < count; i++) {
    const info = gl.getActiveUniform(prog, i)
    if (info) uniforms[info.name] = gl.getUniformLocation(prog, info.name)
  }
  return { program: prog, uniforms }
}

/* ------------------------------------------------------------------ */
/*  Framebuffer management                                             */
/* ------------------------------------------------------------------ */

function createFBO(
  gl: WebGLRenderingContext,
  w: number,
  h: number,
  internalFormat: number,
  format: number,
  type: number,
  filter: number,
): FBO {
  const tex = gl.createTexture()!
  gl.bindTexture(gl.TEXTURE_2D, tex)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, filter)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, filter)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  gl.texImage2D(gl.TEXTURE_2D, 0, internalFormat, w, h, 0, format, type, null)

  const fbo = gl.createFramebuffer()!
  gl.bindFramebuffer(gl.FRAMEBUFFER, fbo)
  gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex, 0)
  gl.viewport(0, 0, w, h)
  gl.clear(gl.COLOR_BUFFER_BIT)

  const texelSizeX = 1 / w
  const texelSizeY = 1 / h

  return {
    texture: tex,
    fbo,
    width: w,
    height: h,
    texelSizeX,
    texelSizeY,
    attach(unit: number) {
      gl.activeTexture(gl.TEXTURE0 + unit)
      gl.bindTexture(gl.TEXTURE_2D, tex)
      return unit
    },
  }
}

function createDoubleFBO(
  gl: WebGLRenderingContext,
  w: number,
  h: number,
  internalFormat: number,
  format: number,
  type: number,
  filter: number,
): DoubleFBO {
  let fbo1 = createFBO(gl, w, h, internalFormat, format, type, filter)
  let fbo2 = createFBO(gl, w, h, internalFormat, format, type, filter)
  return {
    width: w,
    height: h,
    texelSizeX: fbo1.texelSizeX,
    texelSizeY: fbo1.texelSizeY,
    get read() {
      return fbo1
    },
    get write() {
      return fbo2
    },
    swap() {
      const tmp = fbo1
      fbo1 = fbo2
      fbo2 = tmp
    },
  }
}

function getResolution(
  gl: WebGLRenderingContext,
  resolution: number,
) {
  const w = gl.drawingBufferWidth
  const h = gl.drawingBufferHeight
  const aspect = w / h
  const scale = aspect < 1 ? 1 / aspect : aspect
  const r = Math.round(resolution)
  const c = Math.round(resolution * scale)
  return w > h ? { width: c, height: r } : { width: r, height: c }
}

function getSupportedFormat(
  gl: WebGLRenderingContext,
  type: number,
  internalFormat: number,
  format: number,
): { internalFormat: number; format: number } | null {
  if (supportRenderTextureFormat(gl, internalFormat, format, type)) {
    return { internalFormat, format }
  }
  if ('drawBuffers' in gl) {
    const g = gl as any
    if (internalFormat === g.R16F) {
      return getSupportedFormat(gl, type, g.RG16F, g.RG)
    }
    if (internalFormat === g.RG16F) {
      return getSupportedFormat(gl, type, g.RGBA16F, g.RGBA)
    }
  }
  return null
}

function supportRenderTextureFormat(
  gl: WebGLRenderingContext,
  internalFormat: number,
  format: number,
  type: number,
): boolean {
  const tex = gl.createTexture()!
  gl.bindTexture(gl.TEXTURE_2D, tex)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  gl.texImage2D(gl.TEXTURE_2D, 0, internalFormat, 4, 4, 0, format, type, null)

  const fbo = gl.createFramebuffer()!
  gl.bindFramebuffer(gl.FRAMEBUFFER, fbo)
  gl.framebufferTexture2D(
    gl.FRAMEBUFFER,
    gl.COLOR_ATTACHMENT0,
    gl.TEXTURE_2D,
    tex,
    0,
  )
  const status = gl.checkFramebufferStatus(gl.FRAMEBUFFER)
  return status === gl.FRAMEBUFFER_COMPLETE
}

/* ------------------------------------------------------------------ */
/*  Quad setup                                                         */
/* ------------------------------------------------------------------ */

function initBlit(gl: WebGLRenderingContext) {
  const buf = gl.createBuffer()
  gl.bindBuffer(gl.ARRAY_BUFFER, buf)
  gl.bufferData(
    gl.ARRAY_BUFFER,
    new Float32Array([-1, -1, -1, 1, 1, 1, 1, -1]),
    gl.STATIC_DRAW,
  )
  const idx = gl.createBuffer()
  gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, idx)
  gl.bufferData(
    gl.ELEMENT_ARRAY_BUFFER,
    new Uint16Array([0, 1, 2, 0, 2, 3]),
    gl.STATIC_DRAW,
  )
  gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0)
  gl.enableVertexAttribArray(0)
}

function blit(
  gl: WebGLRenderingContext,
  target: FBO | null,
  clear = false,
) {
  if (target) {
    gl.viewport(0, 0, target.width, target.height)
    gl.bindFramebuffer(gl.FRAMEBUFFER, target.fbo)
  } else {
    gl.viewport(0, 0, gl.drawingBufferWidth, gl.drawingBufferHeight)
    gl.bindFramebuffer(gl.FRAMEBUFFER, null)
  }
  if (clear) {
    gl.clearColor(0, 0, 0, 1)
    gl.clear(gl.COLOR_BUFFER_BIT)
  }
  gl.drawElements(gl.TRIANGLES, 6, gl.UNSIGNED_SHORT, 0)
}

/* ------------------------------------------------------------------ */
/*  HSL → RGB for pointer colours                                      */
/* ------------------------------------------------------------------ */

function HSVtoRGB(hue: number, saturation: number, value: number) {
  const i = Math.floor(hue * 6)
  const f = hue * 6 - i
  const p = value * (1 - saturation)
  const q = value * (1 - f * saturation)
  const t = value * (1 - (1 - f) * saturation)
  let r = 0
  let g = 0
  let b = 0
  switch (i % 6) {
    case 0:
      r = value
      g = t
      b = p
      break
    case 1:
      r = q
      g = value
      b = p
      break
    case 2:
      r = p
      g = value
      b = t
      break
    case 3:
      r = p
      g = q
      b = value
      break
    case 4:
      r = t
      g = p
      b = value
      break
    case 5:
      r = value
      g = p
      b = q
      break
  }
  return { r, g, b }
}

function generateColor() {
  const c = HSVtoRGB(Math.random(), 1, 1)
  c.r *= 0.15
  c.g *= 0.15
  c.b *= 0.15
  return c
}

/* ------------------------------------------------------------------ */
/*  Pointer helpers                                                    */
/* ------------------------------------------------------------------ */

function createPointer(id = -1): PointPointer {
  return {
    id,
    texcoordX: 0,
    texcoordY: 0,
    prevTexcoordX: 0,
    prevTexcoordY: 0,
    deltaX: 0,
    deltaY: 0,
    down: false,
    moved: false,
    color: { r: 0, g: 0, b: 0 },
  }
}

function devicePixelRatio() {
  return Math.floor(window.devicePixelRatio || 1)
}


function scalePointerMovement(v: number, aspectRatio: number) {
  return aspectRatio < 1 ? v * aspectRatio : v
}

function downPointer(
  ptr: PointPointer,
  id: number,
  posX: number,
  posY: number,
  canvasW: number,
  canvasH: number,
) {
  ptr.id = id
  ptr.down = true
  ptr.moved = false
  ptr.texcoordX = posX / canvasW
  ptr.texcoordY = 1 - posY / canvasH
  ptr.prevTexcoordX = ptr.texcoordX
  ptr.prevTexcoordY = ptr.texcoordY
  ptr.deltaX = 0
  ptr.deltaY = 0
  ptr.color = generateColor()
}

function movePointer(
  ptr: PointPointer,
  posX: number,
  posY: number,
  color: { r: number; g: number; b: number },
  canvasW: number,
  canvasH: number,
) {
  ptr.prevTexcoordX = ptr.texcoordX
  ptr.prevTexcoordY = ptr.texcoordY
  ptr.texcoordX = posX / canvasW
  ptr.texcoordY = 1 - posY / canvasH
  const aspectRatio = canvasW / canvasH
  ptr.deltaX = scalePointerMovement(ptr.texcoordX - ptr.prevTexcoordX, aspectRatio)
  ptr.deltaY = scalePointerMovement(ptr.texcoordY - ptr.prevTexcoordY, 1 / aspectRatio)
  ptr.moved = Math.abs(ptr.deltaX) > 0 || Math.abs(ptr.deltaY) > 0
  ptr.color = color
}

function upPointer(ptr: PointPointer) {
  ptr.down = false
}

/* ------------------------------------------------------------------ */
/*  GLSL Shaders                                                       */
/* ------------------------------------------------------------------ */

const baseVertexShader = `
  precision highp float;
  attribute vec2 aPosition;
  varying vec2 vUv;
  varying vec2 vL;
  varying vec2 vR;
  varying vec2 vT;
  varying vec2 vB;
  uniform vec2 texelSize;
  void main () {
    vUv = aPosition * 0.5 + 0.5;
    vL = vUv - vec2(texelSize.x, 0.0);
    vR = vUv + vec2(texelSize.x, 0.0);
    vT = vUv + vec2(0.0, texelSize.y);
    vB = vUv - vec2(0.0, texelSize.y);
    gl_Position = vec4(aPosition, 0.0, 1.0);
  }
`

const copyFrag = `
  precision mediump float;
  precision mediump sampler2D;
  varying highp vec2 vUv;
  uniform sampler2D uTexture;
  void main () {
    gl_FragColor = texture2D(uTexture, vUv);
  }
`

const clearFrag = `
  precision mediump float;
  precision mediump sampler2D;
  varying highp vec2 vUv;
  uniform sampler2D uTexture;
  uniform float value;
  void main () {
    gl_FragColor = value * texture2D(uTexture, vUv);
  }
`

const displayFrag = `
  precision highp float;
  precision highp sampler2D;
  varying vec2 vUv;
  varying vec2 vL;
  varying vec2 vR;
  varying vec2 vT;
  varying vec2 vB;
  uniform sampler2D uTexture;
  uniform sampler2D uDithering;
  uniform vec2 ditherScale;
  uniform vec2 texelSize;
  vec3 linearToGamma (vec3 color) {
    color = max(color, vec3(0));
    return max(1.055 * pow(color, vec3(0.416666667)) - 0.055, vec3(0));
  }
  void main () {
    vec3 c = texture2D(uTexture, vUv).rgb;
    #ifdef SHADING
      vec3 lc = texture2D(uTexture, vL).rgb;
      vec3 rc = texture2D(uTexture, vR).rgb;
      vec3 tc = texture2D(uTexture, vT).rgb;
      vec3 bc = texture2D(uTexture, vB).rgb;
      float dx = length(rc) - length(lc);
      float dy = length(tc) - length(bc);
      vec3 n = normalize(vec3(dx, dy, length(texelSize)));
      vec3 l = vec3(0.0, 0.0, 1.0);
      float diffuse = clamp(dot(n, l) + 0.7, 0.7, 1.0);
      c *= diffuse;
    #endif
    float a = max(c.r, max(c.g, c.b));
    gl_FragColor = vec4(c, a);
  }
`

const splatFrag = `
  precision highp float;
  precision highp sampler2D;
  varying vec2 vUv;
  uniform sampler2D uTarget;
  uniform float aspectRatio;
  uniform vec3 color;
  uniform vec2 point;
  uniform float radius;
  void main () {
    vec2 p = vUv - point.xy;
    p.x *= aspectRatio;
    vec3 splat = exp(-dot(p, p) / radius) * color;
    vec3 base = texture2D(uTarget, vUv).xyz;
    gl_FragColor = vec4(base + splat, 1.0);
  }
`

const advectionFrag = `
  precision highp float;
  precision highp sampler2D;
  varying vec2 vUv;
  uniform sampler2D uVelocity;
  uniform sampler2D uSource;
  uniform vec2 texelSize;
  uniform vec2 dyeTexelSize;
  uniform float dt;
  uniform float dissipation;
  vec4 bilerp (sampler2D sam, vec2 uv, vec2 tsize) {
    vec2 st = uv / tsize - 0.5;
    vec2 iuv = floor(st);
    vec2 fuv = fract(st);
    vec4 a = texture2D(sam, (iuv + vec2(0.5, 0.5)) * tsize);
    vec4 b = texture2D(sam, (iuv + vec2(1.5, 0.5)) * tsize);
    vec4 c = texture2D(sam, (iuv + vec2(0.5, 1.5)) * tsize);
    vec4 d = texture2D(sam, (iuv + vec2(1.5, 1.5)) * tsize);
    return mix(mix(a, b, fuv.x), mix(c, d, fuv.x), fuv.y);
  }
  void main () {
    #ifdef MANUAL_FILTERING
      vec2 coord = vUv - dt * bilerp(uVelocity, vUv, texelSize).xy * texelSize;
      vec4 result = bilerp(uSource, coord, dyeTexelSize);
    #else
      vec2 coord = vUv - dt * texture2D(uVelocity, vUv).xy * texelSize;
      vec4 result = texture2D(uSource, coord);
    #endif
    float decay = 1.0 + dissipation * dt;
    gl_FragColor = result / decay;
  }
`

const divergenceFrag = `
  precision mediump float;
  precision mediump sampler2D;
  varying highp vec2 vUv;
  varying highp vec2 vL;
  varying highp vec2 vR;
  varying highp vec2 vT;
  varying highp vec2 vB;
  uniform sampler2D uVelocity;
  void main () {
    float L = texture2D(uVelocity, vL).x;
    float R = texture2D(uVelocity, vR).x;
    float T = texture2D(uVelocity, vT).y;
    float B = texture2D(uVelocity, vB).y;
    vec2 C = texture2D(uVelocity, vUv).xy;
    if (vL.x < 0.0) { L = -C.x; }
    if (vR.x > 1.0) { R = -C.x; }
    if (vT.y > 1.0) { T = -C.y; }
    if (vB.y < 0.0) { B = -C.y; }
    float div = 0.5 * (R - L + T - B);
    gl_FragColor = vec4(div, 0.0, 0.0, 1.0);
  }
`

const curlFrag = `
  precision mediump float;
  precision mediump sampler2D;
  varying highp vec2 vUv;
  varying highp vec2 vL;
  varying highp vec2 vR;
  varying highp vec2 vT;
  varying highp vec2 vB;
  uniform sampler2D uVelocity;
  void main () {
    float L = texture2D(uVelocity, vL).y;
    float R = texture2D(uVelocity, vR).y;
    float T = texture2D(uVelocity, vT).x;
    float B = texture2D(uVelocity, vB).x;
    float vorticity = R - L - T + B;
    gl_FragColor = vec4(0.5 * vorticity, 0.0, 0.0, 1.0);
  }
`

const vorticityFrag = `
  precision highp float;
  precision highp sampler2D;
  varying vec2 vUv;
  varying vec2 vL;
  varying vec2 vR;
  varying vec2 vT;
  varying vec2 vB;
  uniform sampler2D uVelocity;
  uniform sampler2D uCurl;
  uniform float curl;
  uniform float dt;
  void main () {
    float L = texture2D(uCurl, vL).x;
    float R = texture2D(uCurl, vR).x;
    float T = texture2D(uCurl, vT).x;
    float B = texture2D(uCurl, vB).x;
    float C = texture2D(uCurl, vUv).x;
    vec2 force = 0.5 * vec2(abs(T) - abs(B), abs(R) - abs(L));
    force /= length(force) + 0.0001;
    force *= curl * C;
    force.y *= -1.0;
    vec2 velocity = texture2D(uVelocity, vUv).xy;
    velocity += force * dt;
    velocity = min(max(velocity, -1000.0), 1000.0);
    gl_FragColor = vec4(velocity, 0.0, 1.0);
  }
`

const pressureFrag = `
  precision mediump float;
  precision mediump sampler2D;
  varying highp vec2 vUv;
  varying highp vec2 vL;
  varying highp vec2 vR;
  varying highp vec2 vT;
  varying highp vec2 vB;
  uniform sampler2D uPressure;
  uniform sampler2D uDivergence;
  void main () {
    float L = texture2D(uPressure, vL).x;
    float R = texture2D(uPressure, vR).x;
    float T = texture2D(uPressure, vT).x;
    float B = texture2D(uPressure, vB).x;
    float C = texture2D(uPressure, vUv).x;
    float divergence = texture2D(uDivergence, vUv).x;
    float pressure = (L + R + B + T - divergence) * 0.25;
    gl_FragColor = vec4(pressure, 0.0, 0.0, 1.0);
  }
`

const gradientSubtractFrag = `
  precision mediump float;
  precision mediump sampler2D;
  varying highp vec2 vUv;
  varying highp vec2 vL;
  varying highp vec2 vR;
  varying highp vec2 vT;
  varying highp vec2 vB;
  uniform sampler2D uPressure;
  uniform sampler2D uVelocity;
  void main () {
    float L = texture2D(uPressure, vL).x;
    float R = texture2D(uPressure, vR).x;
    float T = texture2D(uPressure, vT).x;
    float B = texture2D(uPressure, vB).x;
    vec2 velocity = texture2D(uVelocity, vUv).xy;
    velocity.xy -= vec2(R - L, T - B);
    gl_FragColor = vec4(velocity, 0.0, 1.0);
  }
`

/* ------------------------------------------------------------------ */
/*  Simulation class                                                   */
/* ------------------------------------------------------------------ */

class FluidSimulation {
  private gl: WebGLRenderingContext
  private config: FluidCursorConfig
  private blitFn: (target: FBO | null, clear?: boolean) => void

  copyProgram: Material
  clearProgram: Material
  splatProgram: Material
  advectionProgram: Material
  divergenceProgram: Material
  curlProgram: Material
  vorticityProgram: Material
  pressureProgram: Material
  gradienSubtractProgram: Material
  displayMaterial: Material

  dye!: DoubleFBO
  velocity!: DoubleFBO
  divergence!: FBO
  curl!: FBO
  pressure!: DoubleFBO

  constructor(
    gl: WebGLRenderingContext,
    ext: any,
    programs: Material[],
    config: FluidCursorConfig,
  ) {
    this.gl = gl
    this.config = config
    this.blitFn = (target, clear = false) => blit(gl, target, clear)

    initBlit(gl)

    const [
      copy,
      clear,
      splat,
      advection,
      divergence,
      curl,
      vorticity,
      pressure,
      gradSub,
      display,
    ] = programs
    this.copyProgram = copy
    this.clearProgram = clear
    this.splatProgram = splat
    this.advectionProgram = advection
    this.divergenceProgram = divergence
    this.curlProgram = curl
    this.vorticityProgram = vorticity
    this.pressureProgram = pressure
    this.gradienSubtractProgram = gradSub
    this.displayMaterial = display

    this.initFramebuffers(ext)
    this.updateKeywords()
  }

  initFramebuffers(ext: any) {
    const gl = this.gl
    const simRes = getResolution(gl, this.config.simResolution)
    const dyeRes = getResolution(gl, this.config.dyeResolution)
    const rgba = ext.formatRGBA
    const rg = ext.formatRG
    const rFmt = ext.formatR
    const filtering = ext.supportLinearFiltering ? gl.LINEAR : gl.NEAREST

    gl.disable(gl.BLEND)

    this.dye = createDoubleFBO(
      gl,
      dyeRes.width,
      dyeRes.height,
      rgba.internalFormat,
      rgba.format,
      rgba.type,
      filtering,
    )
    this.velocity = createDoubleFBO(
      gl,
      simRes.width,
      simRes.height,
      rg.internalFormat,
      rg.format,
      rg.type,
      filtering,
    )
    this.divergence = createFBO(
      gl,
      simRes.width,
      simRes.height,
      rFmt.internalFormat,
      rFmt.format,
      rFmt.type,
      gl.NEAREST,
    )
    this.curl = createFBO(
      gl,
      simRes.width,
      simRes.height,
      rFmt.internalFormat,
      rFmt.format,
      rFmt.type,
      gl.NEAREST,
    )
    this.pressure = createDoubleFBO(
      gl,
      simRes.width,
      simRes.height,
      rFmt.internalFormat,
      rFmt.format,
      rFmt.type,
      gl.NEAREST,
    )
  }

  updateKeywords() {
    const keywords: string[] = []
    if (this.config.shading) keywords.push('SHADING')
    this.displayMaterial.setKeywords!(this.gl, keywords)
  }

  step(dt: number) {
    const gl = this.gl
    gl.disable(gl.BLEND)

    // Curl
    this.curlProgram.bind(gl)
    gl.uniform2f(
      this.curlProgram.uniforms.texelSize!,
      this.velocity.texelSizeX,
      this.velocity.texelSizeY,
    )
    gl.uniform1i(
      this.curlProgram.uniforms.uVelocity!,
      this.velocity.read.attach(0),
    )
    this.blitFn(this.curl)

    // Vorticity
    this.vorticityProgram.bind(gl)
    gl.uniform2f(
      this.vorticityProgram.uniforms.texelSize!,
      this.velocity.texelSizeX,
      this.velocity.texelSizeY,
    )
    gl.uniform1i(
      this.vorticityProgram.uniforms.uVelocity!,
      this.velocity.read.attach(0),
    )
    gl.uniform1i(this.vorticityProgram.uniforms.uCurl!, this.curl.attach(1))
    gl.uniform1f(this.vorticityProgram.uniforms.curl!, this.config.curl)
    gl.uniform1f(this.vorticityProgram.uniforms.dt!, dt)
    this.blitFn(this.velocity.write)
    this.velocity.swap()

    // Divergence
    this.divergenceProgram.bind(gl)
    gl.uniform2f(
      this.divergenceProgram.uniforms.texelSize!,
      this.velocity.texelSizeX,
      this.velocity.texelSizeY,
    )
    gl.uniform1i(
      this.divergenceProgram.uniforms.uVelocity!,
      this.velocity.read.attach(0),
    )
    this.blitFn(this.divergence)

    // Clear pressure
    this.clearProgram.bind(gl)
    gl.uniform1i(
      this.clearProgram.uniforms.uTexture!,
      this.pressure.read.attach(0),
    )
    gl.uniform1f(this.clearProgram.uniforms.value!, this.config.pressure)
    this.blitFn(this.pressure.write)
    this.pressure.swap()

    // Pressure (Jacobi iterations)
    this.pressureProgram.bind(gl)
    gl.uniform2f(
      this.pressureProgram.uniforms.texelSize!,
      this.velocity.texelSizeX,
      this.velocity.texelSizeY,
    )
    gl.uniform1i(
      this.pressureProgram.uniforms.uDivergence!,
      this.divergence.attach(0),
    )
    for (let i = 0; i < this.config.pressureIterations; i++) {
      gl.uniform1i(
        this.pressureProgram.uniforms.uPressure!,
        this.pressure.read.attach(1),
      )
      this.blitFn(this.pressure.write)
      this.pressure.swap()
    }

    // Gradient subtract
    this.gradienSubtractProgram.bind(gl)
    gl.uniform2f(
      this.gradienSubtractProgram.uniforms.texelSize!,
      this.velocity.texelSizeX,
      this.velocity.texelSizeY,
    )
    gl.uniform1i(
      this.gradienSubtractProgram.uniforms.uPressure!,
      this.pressure.read.attach(0),
    )
    gl.uniform1i(
      this.gradienSubtractProgram.uniforms.uVelocity!,
      this.velocity.read.attach(1),
    )
    this.blitFn(this.velocity.write)
    this.velocity.swap()

    // Advection — velocity
    this.advectionProgram.bind(gl)
    gl.uniform2f(
      this.advectionProgram.uniforms.texelSize!,
      this.velocity.texelSizeX,
      this.velocity.texelSizeY,
    )
    if (this.advectionProgram.uniforms.dyeTexelSize) {
      gl.uniform2f(
        this.advectionProgram.uniforms.dyeTexelSize,
        this.velocity.texelSizeX,
        this.velocity.texelSizeY,
      )
    }
    const velSrc = this.velocity.read.attach(0)
    gl.uniform1i(this.advectionProgram.uniforms.uVelocity!, velSrc)
    gl.uniform1i(this.advectionProgram.uniforms.uSource!, velSrc)
    gl.uniform1f(this.advectionProgram.uniforms.dt!, dt)
    gl.uniform1f(
      this.advectionProgram.uniforms.dissipation!,
      this.config.velocityDissipation,
    )
    this.blitFn(this.velocity.write)
    this.velocity.swap()

    // Advection — dye
    if (this.advectionProgram.uniforms.dyeTexelSize) {
      gl.uniform2f(
        this.advectionProgram.uniforms.dyeTexelSize,
        this.dye.texelSizeX,
        this.dye.texelSizeY,
      )
    }
    gl.uniform1i(
      this.advectionProgram.uniforms.uVelocity!,
      this.velocity.read.attach(0),
    )
    gl.uniform1i(
      this.advectionProgram.uniforms.uSource!,
      this.dye.read.attach(1),
    )
    gl.uniform1f(
      this.advectionProgram.uniforms.dissipation!,
      this.config.densityDissipation,
    )
    this.blitFn(this.dye.write)
    this.dye.swap()
  }

  render(target: FBO | null) {
    const gl = this.gl
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
    gl.enable(gl.BLEND)

    const w = target ? target.width : gl.drawingBufferWidth
    const h = target ? target.height : gl.drawingBufferHeight

    this.displayMaterial.bind(gl)
    if (this.config.shading) {
      gl.uniform2f(
        this.displayMaterial.uniforms.texelSize!,
        1 / w,
        1 / h,
      )
    }
    gl.uniform1i(
      this.displayMaterial.uniforms.uTexture!,
      this.dye.read.attach(0),
    )
    this.blitFn(target, false)
  }

  splat(
    x: number,
    y: number,
    dx: number,
    dy: number,
    color: { r: number; g: number; b: number },
  ) {
    const gl = this.gl
    // Velocity splat
    this.splatProgram.bind(gl)
    gl.uniform1i(this.splatProgram.uniforms.uTarget!, this.velocity.read.attach(0))
    gl.uniform1f(
      this.splatProgram.uniforms.aspectRatio!,
      gl.drawingBufferWidth / gl.drawingBufferHeight,
    )
    gl.uniform2f(this.splatProgram.uniforms.point!, x, y)
    gl.uniform3f(this.splatProgram.uniforms.color!, dx, dy, 0)
    gl.uniform1f(
      this.splatProgram.uniforms.radius!,
      this.config.splatRadius / 100,
    )
    this.blitFn(this.velocity.write)
    this.velocity.swap()
    // Dye splat
    gl.uniform1i(this.splatProgram.uniforms.uTarget!, this.dye.read.attach(0))
    gl.uniform3f(
      this.splatProgram.uniforms.color!,
      color.r,
      color.g,
      color.b,
    )
    this.blitFn(this.dye.write)
    this.dye.swap()
  }
}

/* ------------------------------------------------------------------ */
/*  Public API                                                         */
/* ------------------------------------------------------------------ */

function initWebGL(canvas: HTMLCanvasElement) {
  const opts: WebGLContextAttributes = {
    alpha: true,
    depth: false,
    stencil: false,
    antialias: false,
    preserveDrawingBuffer: false,
  }
  let gl: WebGLRenderingContext | null =
    canvas.getContext('webgl2', opts) as any
  const isWebGL2 = !!gl
  if (!gl) {
    gl =
      (canvas.getContext('webgl', opts) as any) ||
      canvas.getContext('experimental-webgl', opts)
  }
  if (!gl) throw new Error('WebGL not supported')

  const rgba8 = isWebGL2 ? (gl as any).RGBA8 as number : gl.RGBA
  const ext: {
    formatRGBA: { internalFormat: number; format: number; type: number }
    formatRG: { internalFormat: number; format: number; type: number }
    formatR: { internalFormat: number; format: number; type: number }
    halfFloatTexType: number
    supportLinearFiltering: boolean
  } = {
    formatRGBA: { internalFormat: rgba8, format: gl.RGBA, type: gl.UNSIGNED_BYTE },
    formatRG: { internalFormat: rgba8, format: gl.RGBA, type: gl.UNSIGNED_BYTE },
    formatR: { internalFormat: rgba8, format: gl.RGBA, type: gl.UNSIGNED_BYTE },
    halfFloatTexType: gl.UNSIGNED_BYTE,
    supportLinearFiltering: false,
  }

  const tryHalfFloat = (): boolean => {
    if (isWebGL2) {
      gl.getExtension('EXT_color_buffer_float')
      ext.supportLinearFiltering = !!gl.getExtension('OES_texture_float_linear')
      const hfType = (gl as any).HALF_FLOAT as number
      const a = getSupportedFormat(gl, hfType, (gl as any).RGBA16F, gl.RGBA)
      const b = getSupportedFormat(gl, hfType, (gl as any).RG16F, (gl as any).RG)
      const c = getSupportedFormat(gl, hfType, (gl as any).R16F, (gl as any).RED)
      if (a && b && c) {
        ext.formatRGBA = { ...a, type: hfType }
        ext.formatRG = { ...b, type: hfType }
        ext.formatR = { ...c, type: hfType }
        ext.halfFloatTexType = hfType
        return true
      }
    } else {
      const hfExt = gl.getExtension('OES_texture_half_float') as any
      if (hfExt) {
        ext.supportLinearFiltering = !!gl.getExtension('OES_texture_half_float_linear')
        const hfType = hfExt.HALF_FLOAT_OES as number
        const a = getSupportedFormat(gl, hfType, gl.RGBA, gl.RGBA)
        if (a) {
          ext.formatRGBA = { ...a, type: hfType }
          ext.formatRG = { ...a, type: hfType }
          ext.formatR = { ...a, type: hfType }
          ext.halfFloatTexType = hfType
          return true
        }
      }
    }
    return false
  }

  tryHalfFloat()
  gl.clearColor(0, 0, 0, 1)
  return { gl, ext }
}


/** Build programs array once (reuse across renders). */
function buildPrograms(gl: WebGLRenderingContext, supportLinearFiltering: boolean): Material[] {
  const vs = baseVertexShader
  const mk = (src: string, kw?: string[] | null): Material => {
    const p = createProgram(gl, vs, src, kw)
    return {
      ...p,
      bind(g: WebGLRenderingContext) {
        g.useProgram(p.program)
      },
    } as Material
  }
  return [
    mk(copyFrag),
    mk(clearFrag),
    mk(splatFrag),
    mk(advectionFrag, supportLinearFiltering ? null : ['MANUAL_FILTERING']),
    mk(divergenceFrag),
    mk(curlFrag),
    mk(vorticityFrag),
    mk(pressureFrag),
    mk(gradientSubtractFrag),
    (() => {
      const p = createProgram(gl, vs, displayFrag)
      return {
        ...p,
        bind(g: WebGLRenderingContext) {
          g.useProgram(p.program)
        },
        setKeywords(g: WebGLRenderingContext, keywords: string[]) {
          const kw = keywords.length ? keywords : null
          const newP = createProgram(g, vs, displayFrag, kw)
          if (newP.program) g.useProgram(newP.program)
          p.program = newP.program
          Object.assign(p.uniforms, newP.uniforms)
        },
      } as Material
    })(),
  ]
}

export interface FluidCursorControls {
  destroy(): void
}

/**
 * Initialise the fluid cursor effect on a canvas element.
 * Returns a controls object with a `destroy()` cleanup method.
 */
export function initFluidCursor(
  canvas: HTMLCanvasElement,
  userConfig: Partial<FluidCursorConfig> = {},
): FluidCursorControls {
  const config: FluidCursorConfig = { ...DEFAULTS, ...userConfig }
  const reducedMotion =
    typeof matchMedia === 'function' &&
    matchMedia('(prefers-reduced-motion: reduce)').matches

  if (reducedMotion) return { destroy() {} }

  if (!canvas.clientWidth || !canvas.clientHeight) return { destroy() {} }

  const dpr = window.devicePixelRatio || 1
  canvas.width = Math.floor(canvas.clientWidth * dpr)
  canvas.height = Math.floor(canvas.clientHeight * dpr)

  const { gl, ext } = initWebGL(canvas)
  if (!ext.supportLinearFiltering) {
    config.dyeResolution = 256
    config.shading = false
  }

  const programs = buildPrograms(gl, ext.supportLinearFiltering)
  const sim = new FluidSimulation(gl, ext, programs, config)

  const pointers: PointPointer[] = [createPointer()]

  // --- pointer events ---
  const splatOnMove = (ptr: PointPointer) => {
    const dx = ptr.deltaX * config.splatForce
    const dy = ptr.deltaY * config.splatForce
    sim.splat(ptr.texcoordX, ptr.texcoordY, dx, dy, ptr.color)
  }

  const onMouseMove = (e: MouseEvent) => {
    const ptr = pointers[0]
    const px = devicePixelRatio() * e.clientX
    const py = devicePixelRatio() * e.clientY
    if (ptr.down) {
      movePointer(ptr, px, py, ptr.color, canvas.width, canvas.height)
    } else {
      downPointer(ptr, -1, px, py, canvas.width, canvas.height)
      const col = generateColor()
      col.r *= 10
      col.g *= 10
      col.b *= 10
      const randDx = 10 * (Math.random() - 0.5)
      const randDy = 30 * (Math.random() - 0.5)
      sim.splat(ptr.texcoordX, ptr.texcoordY, randDx, randDy, col)
    }
  }
  const onMouseUp = () => upPointer(pointers[0])

  const onTouchStart = (e: TouchEvent) => {
    e.preventDefault()
    const touches = e.targetTouches
    for (let i = 0; i < touches.length && i < pointers.length; i++) {
      const p = pointers[i]
      const px = devicePixelRatio() * touches[i].clientX
      const py = devicePixelRatio() * touches[i].clientY
      downPointer(p, touches[i].identifier, px, py, canvas.width, canvas.height)
    }
  }
  const onTouchMove = (e: TouchEvent) => {
    e.preventDefault()
    const touches = e.targetTouches
    for (let i = 0; i < touches.length && i < pointers.length; i++) {
      const p = pointers[i]
      if (!p.down) continue
      const px = devicePixelRatio() * touches[i].clientX
      const py = devicePixelRatio() * touches[i].clientY
      movePointer(p, px, py, p.color, canvas.width, canvas.height)
    }
  }
  const onTouchEnd = (e: TouchEvent) => {
    const touches = e.changedTouches
    for (let i = 0; i < touches.length; i++) {
      const p = pointers.find((pp) => pp.id === touches[i].identifier)
      if (p) upPointer(p)
    }
  }

  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
  canvas.addEventListener('touchstart', onTouchStart, { passive: false })
  window.addEventListener('touchmove', onTouchMove, { passive: false })
  window.addEventListener('touchend', onTouchEnd)

  // --- animation loop ---
  let animFrame = 0
  let lastTime = Date.now()
  let colorUpdateAccum = 0

  const resizeCanvas = (): boolean => {
    const dpr = window.devicePixelRatio || 1
    const w = Math.floor(canvas.clientWidth * dpr)
    const h = Math.floor(canvas.clientHeight * dpr)
    if (canvas.width !== w || canvas.height !== h) {
      canvas.width = w
      canvas.height = h
      return true
    }
    return false
  }

  const updateColors = (dt: number) => {
    colorUpdateAccum += dt * config.colorUpdateSpeed
    if (colorUpdateAccum >= 1) {
      colorUpdateAccum = (colorUpdateAccum - 1) % 1
      for (const ptr of pointers) {
        ptr.color = generateColor()
      }
    }
  }

  const tick = () => {
    const now = Date.now()
    let dt = (now - lastTime) / 1000
    dt = Math.min(dt, 0.016666)
    lastTime = now

    if (resizeCanvas()) {
      sim.initFramebuffers(ext)
    }

    updateColors(dt)

    for (const ptr of pointers) {
      if (ptr.moved) {
        ptr.moved = false
        splatOnMove(ptr)
      }
    }

    sim.step(dt)
    sim.render(null)

    animFrame = requestAnimationFrame(tick)
  }

  lastTime = Date.now()
  animFrame = requestAnimationFrame(tick)

  return {
    destroy() {
      cancelAnimationFrame(animFrame)
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', onMouseUp)
      canvas.removeEventListener('touchstart', onTouchStart)
      window.removeEventListener('touchmove', onTouchMove)
      window.removeEventListener('touchend', onTouchEnd)
    },
  }
}
