import { STATUS } from './geo'
import type { HiringStatus } from './types'

const order: HiringStatus[] = ['hiring', 'open_application_only', 'no_openings', 'unknown']

type Props = {
  hiringOnly: boolean
  onHiringOnlyChange: (value: boolean) => void
}

export default function Controls({ hiringOnly, onHiringOnlyChange }: Props) {
  return (
    <div className="controls">
      <label>
        <input
          type="checkbox"
          checked={hiringOnly}
          onChange={(event) => onHiringOnlyChange(event.target.checked)}
        />
        Hiring only
      </label>
      <ul className="legend">
        {order.map((status) => (
          <li key={status}>
            <span
              className="swatch"
              style={{
                background: STATUS[status].color,
                borderColor: STATUS[status].stroke ?? STATUS[status].color,
              }}
            />
            {STATUS[status].label}
          </li>
        ))}
      </ul>
    </div>
  )
}
