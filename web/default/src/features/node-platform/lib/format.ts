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

// Money on the wire is integer micro-USD (1 USD = 1,000,000 micros). Format for
// display and parse user input back to micros.

const MICROS_PER_UNIT = 1_000_000

export function microsToDisplay(micros?: number): string {
  if (!micros && micros !== 0) return '-'
  return (micros / MICROS_PER_UNIT).toFixed(6).replace(/\.?0+$/, '')
}

export function displayToMicros(value: string): number {
  const n = Number(value)
  if (!Number.isFinite(n) || n < 0) return 0
  return Math.round(n * MICROS_PER_UNIT)
}

export function formatUnix(seconds?: number): string {
  if (!seconds) return '-'
  return new Date(seconds * 1000).toLocaleString()
}
