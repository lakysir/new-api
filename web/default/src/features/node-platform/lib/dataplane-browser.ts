/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

// Browser (Web Crypto) E2EE data-plane, matching the plugin's
// src/data-plane/dataplane.js and new-api/service/dataplane byte-for-byte
// (X25519 + HKDF-SHA256 + AES-256-GCM, seq nonce, seq header as AAD). Used by
// the purchase page (client side) so config ciphertext interops with the
// provider plugin. Crypto correctness is covered by the plugin's Node vector
// tests against the shared dataplane_vector.json.

export const PROTOCOL_VERSION = '1.0'
export const DIR_CLIENT_TO_PROVIDER = 'c2p'
export const DIR_PROVIDER_TO_CLIENT = 'p2c'

// MAX_PLAINTEXT_BYTES caps the config plaintext a single frame may carry, and
// must match relayhub.MaxPlaintextBytes on the server. E2EE ciphertext still
// crosses the operator's relay, so oversized params (e.g. an inline base64
// image) are rejected before sealing — the relay would drop the frame anyway.
export const MAX_PLAINTEXT_BYTES = 2 * 1024 * 1024

export interface SessionContext {
  taskId: string
  attempt: number
  clientDeviceId: string
  providerDeviceId: string
}

function bytesToB64(bytes: ArrayBuffer | Uint8Array): string {
  const a = new Uint8Array(bytes as ArrayBuffer)
  let bin = ''
  for (let i = 0; i < a.length; i++) bin += String.fromCharCode(a[i]!)
  return btoa(bin)
}
function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

export async function generateKeyPair(): Promise<{
  privateKey: CryptoKey
  publicKeyB64: string
}> {
  const kp = (await crypto.subtle.generateKey({ name: 'X25519' }, true, [
    'deriveBits',
  ])) as CryptoKeyPair
  const raw = await crypto.subtle.exportKey('raw', kp.publicKey)
  return { privateKey: kp.privateKey, publicKeyB64: bytesToB64(raw) }
}

// bs casts a Uint8Array to BufferSource — TS 5.7 typed-array generics reject
// Uint8Array<ArrayBufferLike> at Web Crypto/WebSocket call sites even though it
// is a valid BufferSource at runtime.
function bs(u: Uint8Array): BufferSource {
  return u as unknown as BufferSource
}

export async function sharedSecret(
  privateKey: CryptoKey,
  peerPubB64: string
): Promise<Uint8Array> {
  const peer = await crypto.subtle.importKey(
    'raw',
    bs(b64ToBytes(peerPubB64)),
    { name: 'X25519' },
    false,
    []
  )
  const bits = await crypto.subtle.deriveBits(
    { name: 'X25519', public: peer },
    privateKey,
    256
  )
  return new Uint8Array(bits)
}

function hkdfInfo(ctx: SessionContext, direction: string): Uint8Array {
  const s =
    'ai-token-p2p:dataplane:v' +
    PROTOCOL_VERSION +
    '|task=' +
    ctx.taskId +
    '|attempt=' +
    String(ctx.attempt) +
    '|client=' +
    ctx.clientDeviceId +
    '|provider=' +
    ctx.providerDeviceId +
    '|dir=' +
    direction
  return new TextEncoder().encode(s)
}

export async function deriveAesKey(
  secret: Uint8Array,
  ctx: SessionContext,
  direction: string
): Promise<CryptoKey> {
  const base = await crypto.subtle.importKey('raw', bs(secret), 'HKDF', false, [
    'deriveKey',
  ])
  return crypto.subtle.deriveKey(
    {
      name: 'HKDF',
      hash: 'SHA-256',
      salt: bs(new Uint8Array(0)),
      info: bs(hkdfInfo(ctx, direction)),
    },
    base,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt']
  )
}

function nonceFromSeq(seq: bigint): Uint8Array {
  const n = new Uint8Array(12)
  new DataView(n.buffer).setBigUint64(4, seq, false)
  return n
}
function headerFromSeq(seq: bigint): Uint8Array {
  const h = new Uint8Array(8)
  new DataView(h.buffer).setBigUint64(0, seq, false)
  return h
}

export class Sealer {
  #key: CryptoKey
  #seq = 0n
  constructor(key: CryptoKey) {
    this.#key = key
  }
  async seal(plaintext: Uint8Array): Promise<Uint8Array> {
    const seq = this.#seq
    this.#seq += 1n
    const header = headerFromSeq(seq)
    const ct = new Uint8Array(
      await crypto.subtle.encrypt(
        {
          name: 'AES-GCM',
          iv: bs(nonceFromSeq(seq)),
          additionalData: bs(header),
        },
        this.#key,
        bs(plaintext)
      )
    )
    const frame = new Uint8Array(8 + ct.length)
    frame.set(header, 0)
    frame.set(ct, 8)
    return frame
  }
}

export class Opener {
  #key: CryptoKey
  #nextSeq = 0n
  constructor(key: CryptoKey) {
    this.#key = key
  }
  async open(frame: Uint8Array): Promise<Uint8Array> {
    if (frame.length < 8 + 16) throw new Error('frame too short')
    const header = frame.slice(0, 8)
    const seq = new DataView(header.buffer, header.byteOffset, 8).getBigUint64(
      0,
      false
    )
    if (seq !== this.#nextSeq) throw new Error('aead open failed')
    const pt = new Uint8Array(
      await crypto.subtle.decrypt(
        {
          name: 'AES-GCM',
          iv: bs(nonceFromSeq(seq)),
          additionalData: bs(header),
        },
        this.#key,
        bs(frame.slice(8))
      )
    )
    this.#nextSeq += 1n
    return pt
  }
}
