import type { LoreRailGroups, LoreRecord } from './graph/aggregate'
import { NODE_COLORS } from './graph/style'

export type RailGroupKey = 'decisions' | 'items' | 'notes' | 'sessions'

const GROUP_META: Array<{ key: RailGroupKey; title: string; color: string }> = [
  { key: 'decisions', title: 'Decisions', color: NODE_COLORS.decision },
  { key: 'items', title: 'Work items', color: NODE_COLORS.item },
  { key: 'notes', title: 'Notes', color: NODE_COLORS.note },
  { key: 'sessions', title: 'Sessions', color: NODE_COLORS.note },
]

interface Props {
  rail: LoreRailGroups
  visible: Set<RailGroupKey>
  onToggleGroup: (k: RailGroupKey) => void
  hotIds: Set<string>
  onHover: (rec: LoreRecord | null) => void
  onOpen: (id: string) => void
  focusPkg: string | null
  showAll: boolean
  onToggleShowAll: () => void
}

export function LoreRail({ rail, visible, onToggleGroup, hotIds, onHover, onOpen, focusPkg, showAll, onToggleShowAll }: Props) {
  const filterPkg = focusPkg && !showAll ? focusPkg : null
  const rows = (recs: LoreRecord[]) =>
    filterPkg ? recs.filter((r) => r.pkgs.includes(filterPkg)) : recs

  return (
    <div className="lore-rail" data-testid="lore-rail" onMouseLeave={() => onHover(null)}>
      <div className="rail-head">
        <span className="section-label">Lore</span>
        {focusPkg && (
          <button className="chip rail-showall" data-testid="rail-showall" onClick={onToggleShowAll}>
            {showAll ? 'this package' : 'show all'}
          </button>
        )}
      </div>
      {GROUP_META.map(({ key, title, color }) => {
        if (!visible.has(key)) return null
        const recs = rows(rail[key])
        if (recs.length === 0) return null
        return (
          <div className="rail-group" key={key}>
            <div className="rail-group-title">{title}</div>
            {recs.map((r) => (
              <button
                key={r.id}
                className={`rail-item${hotIds.has(r.id) ? ' hot' : ''}`}
                data-testid="rail-item"
                data-kind={key}
                title={r.label}
                onMouseEnter={() => onHover(r)}
                onClick={() => onOpen(r.id)}
              >
                <span className="rail-dot" style={{ background: color }} />
                <span className="rail-label">{r.label}</span>
                {r.blockedBy.length > 0 && <span className="rail-badge">⛓{r.blockedBy.length}</span>}
              </button>
            ))}
          </div>
        )
      })}
      <div className="rail-chips">
        {GROUP_META.map(({ key, title }) => (
          <button
            key={key}
            className={`chip rail-chip${visible.has(key) ? ' on' : ' off'}`}
            data-testid={`rail-chip-${key}`}
            onClick={() => onToggleGroup(key)}
          >
            {title.toLowerCase()} {rail[key].length}
          </button>
        ))}
      </div>
    </div>
  )
}
