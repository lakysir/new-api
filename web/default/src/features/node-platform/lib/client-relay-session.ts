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

// Client side of the E2EE data plane for the purchase page: connect the relay
// for (task_id, attempt) as "client", handshake with the provider (in-band
// ephemeral X25519 keys), send the encrypted config, and await the encrypted
// result. Mirrors the plugin's provider-side RelaySession; the relay only
// forwards opaque frames.

import {
  DIR_CLIENT_TO_PROVIDER,
  DIR_PROVIDER_TO_CLIENT,
  Opener,
  Sealer,
  deriveAesKey,
  generateKeyPair,
  sharedSecret,
} from './dataplane-browser'

const TAG_HANDSHAKE = 0x01
const TAG_DATA = 0x02

function tagFrame(tag: number, body: Uint8Array): Uint8Array {
  const out = new Uint8Array(1 + body.length)
  out[0] = tag
  out.set(body, 1)
  return out
}

export interface ClientRelayOptions {
  relayUrl: string // wss://…/api/relay
  deviceToken?: string // optional for dashboard sessions; API clients pass a key
  taskId: string
  attempt: number
  clientDeviceId: string
}

export class ClientRelaySession {
  #opts: ClientRelayOptions
  #socket: WebSocket | null = null
  #kp: { privateKey: CryptoKey; publicKeyB64: string } | null = null
  #sealer: Sealer | null = null
  #opener: Opener | null = null
  #established: (() => void) | null = null
  #establishedPromise: Promise<void>
  #resultResolve: ((r: unknown) => void) | null = null
  #resultPromise: Promise<unknown>
  #pendingDataFrames: Uint8Array[] = []

  constructor(opts: ClientRelayOptions) {
    this.#opts = opts
    this.#establishedPromise = new Promise((r) => (this.#established = r))
    this.#resultPromise = new Promise((r) => (this.#resultResolve = r))
  }

  async connect(): Promise<void> {
    const url =
      `${this.#opts.relayUrl}?task_id=${encodeURIComponent(this.#opts.taskId)}` +
      `&attempt=${this.#opts.attempt}&role=client`
    this.#kp = await generateKeyPair()
    const protocols = this.#opts.deviceToken
      ? ['aitoken', this.#opts.deviceToken]
      : ['aitoken']
    const socket = new WebSocket(url, protocols)
    socket.binaryType = 'arraybuffer'
    this.#socket = socket
    await new Promise<void>((resolve, reject) => {
      socket.onopen = () => {
        this.#sendHandshake()
        resolve()
      }
      socket.onerror = () => reject(new Error('relay connect failed'))
      socket.onmessage = (ev) =>
        void this.#onFrame(new Uint8Array(ev.data as ArrayBuffer))
    })
  }

  #sendHandshake() {
    const hello = JSON.stringify({
      role: 'client',
      pub: this.#kp!.publicKeyB64,
      device_id: this.#opts.clientDeviceId,
    })
    this.#socket!.send(
      tagFrame(
        TAG_HANDSHAKE,
        new TextEncoder().encode(hello)
      ) as unknown as ArrayBuffer
    )
  }

  async #onFrame(frame: Uint8Array) {
    if (frame.length < 1) return
    const tag = frame[0]
    const body = frame.slice(1)
    if (tag === TAG_HANDSHAKE) await this.#onHandshake(body)
    else if (tag === TAG_DATA) {
      if (this.#opener) await this.#onData(body)
      else this.#pendingDataFrames.push(body)
    }
  }

  async #onHandshake(body: Uint8Array) {
    let msg: { pub?: string; device_id?: string }
    try {
      msg = JSON.parse(new TextDecoder().decode(body))
    } catch {
      return
    }
    if (!msg.pub || !msg.device_id) return
    // Already established: ignore duplicate/late handshakes so we never rebuild
    // the Opener (which would reset its sequence and reject the live stream).
    if (this.#sealer) return
    // Re-send our handshake in reply. If the client connected before the
    // provider, our initial handshake was dropped (no peer yet); the provider's
    // handshake proves it has now joined, so replying delivers our pubkey to it.
    this.#sendHandshake()
    const secret = await sharedSecret(this.#kp!.privateKey, msg.pub)
    const ctx = {
      taskId: this.#opts.taskId,
      attempt: this.#opts.attempt,
      clientDeviceId: this.#opts.clientDeviceId,
      providerDeviceId: msg.device_id,
    }
    // Client writes c2p, reads p2c.
    this.#sealer = new Sealer(
      await deriveAesKey(secret, ctx, DIR_CLIENT_TO_PROVIDER)
    )
    this.#opener = new Opener(
      await deriveAesKey(secret, ctx, DIR_PROVIDER_TO_CLIENT)
    )
    const pending = this.#pendingDataFrames.splice(0)
    for (const frame of pending) await this.#onData(frame)
    this.#established?.()
  }

  async #onData(body: Uint8Array) {
    try {
      const pt = await this.#opener!.open(body)
      this.#resultResolve?.(JSON.parse(new TextDecoder().decode(pt)))
      this.#resultResolve = null
    } catch {
      /* ignore malformed/duplicate */
    }
  }

  /** Wait until the provider handshake completes (keys derived). */
  waitEstablished(timeoutMs = 30000): Promise<void> {
    return Promise.race([
      this.#establishedPromise,
      new Promise<void>((_, rej) =>
        setTimeout(() => rej(new Error('handshake timeout')), timeoutMs)
      ),
    ])
  }

  /** Encrypt and send the config to the provider. */
  async sendConfig(config: unknown): Promise<void> {
    if (!this.#sealer) throw new Error('session not established')
    const frame = await this.#sealer.seal(
      new TextEncoder().encode(JSON.stringify(config))
    )
    this.#socket!.send(tagFrame(TAG_DATA, frame) as unknown as ArrayBuffer)
  }

  /** Await the provider's encrypted result. */
  waitForResult(timeoutMs = 120000): Promise<unknown> {
    return Promise.race([
      this.#resultPromise,
      new Promise((_, rej) =>
        setTimeout(() => rej(new Error('result timeout')), timeoutMs)
      ),
    ])
  }

  close() {
    this.#pendingDataFrames = []
    try {
      this.#socket?.close()
    } catch {
      /* noop */
    }
  }
}
