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
import { Fragment } from 'react'

export function ModelDescription(props: { text: string }) {
  return props.text.split(/(\[\[[^\]]+\]\])/g).map((part, index) => {
    const highlighted = part.startsWith('[[') && part.endsWith(']]')
    const content = highlighted ? part.slice(2, -2) : part

    return highlighted ? (
      <strong
        key={`${index}-${content}`}
        className='font-bold text-red-600 dark:text-red-400'
      >
        {content}
      </strong>
    ) : (
      <Fragment key={`${index}-${content}`}>{content}</Fragment>
    )
  })
}
